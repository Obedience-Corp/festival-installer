//go:build container_fs && !windows

package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Obedience-Corp/festival-installer/internal/artifacts"
	"github.com/Obedience-Corp/festival-installer/internal/metadata"
	"github.com/Obedience-Corp/festival-installer/internal/state/lock"
	"github.com/Obedience-Corp/festival-installer/internal/verify"
	"github.com/Obedience-Corp/festival-installer/internal/verify/minisigntest"
)

const appDownloadComment = "timestamp:1\tfile:Festival.App_0.1.8_amd64.AppImage"

type appDownloadCase struct {
	home     string
	payload  []byte
	signer   *minisigntest.Signer
	key      verify.MinisignPublicKey
	requests atomic.Int32
}

func newAppDownloadCase(t *testing.T) *appDownloadCase {
	t.Helper()
	home := filepath.Join(t.TempDir(), "installer")
	t.Setenv("FESTIVAL_HOME", home)
	signer, err := minisigntest.NewSigner()
	if err != nil {
		t.Fatal(err)
	}
	key, err := verify.ParseTauriPublicKey(signer.TauriPublicKey())
	if err != nil {
		t.Fatal(err)
	}
	return &appDownloadCase{home: home, payload: bytes.Repeat([]byte("festival"), 32*1024), signer: signer, key: key}
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (c *appDownloadCase) release(url string, served []byte, sig string) appRelease {
	art := metadata.Artifact{
		Kind: metadata.ArtifactKindAppImage, OS: "linux", Arch: "amd64",
		URL: url + "/Festival.App_0.1.8_amd64.AppImage", Sha256: sha256Hex(served),
		Filename: "Festival.App_0.1.8_amd64.AppImage", UpdaterSignature: sig,
	}
	return appRelease{PackageID: AppPackageID, Version: "0.1.8", Channel: "stable", Format: metadata.ArtifactKindAppImage, Artifact: art}
}

func (c *appDownloadCase) serve(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.requests.Add(1)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (c *appDownloadCase) goodRelease(t *testing.T) (appRelease, *httptest.Server) {
	srv := c.serve(t, c.payload)
	return c.release(srv.URL, c.payload, c.signer.SignTauri(c.payload, appDownloadComment)), srv
}

func runDirs(t *testing.T, home string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(home, appDownloadsDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "run-") {
			names = append(names, e.Name())
		}
	}
	return names
}

func TestAppDownloadVerifiesAndReportsProgress(t *testing.T) {
	c := newAppDownloadCase(t)
	rel, _ := c.goodRelease(t)
	var events []ProgressEvent
	got, err := downloadAppArtifact(context.Background(), c.home, rel, appDownloadOptions{UpdaterKey: &c.key, Progress: func(e ProgressEvent) { events = append(events, e) }})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got.Path, filepath.Join(c.home, appDownloadsDir, "run-")) {
		t.Fatalf("path %s not under app-downloads/run-*", got.Path)
	}
	if st, _ := os.Stat(got.Dir); st == nil || st.Mode().Perm() != 0o700 {
		t.Fatalf("run dir mode: %v", st)
	}
	data, err := os.ReadFile(got.Path)
	if err != nil || !bytes.Equal(data, c.payload) {
		t.Fatalf("payload mismatch: %v", err)
	}
	var lastDownload ProgressEvent
	sawVerify := false
	for _, e := range events {
		if e.Stage == "download" {
			lastDownload = e
		}
		if e.Stage == "verify" {
			sawVerify = true
		}
	}
	if lastDownload.Bytes != int64(len(c.payload)) || !sawVerify {
		t.Fatalf("progress ended at %+v (verify=%v)", lastDownload, sawVerify)
	}
	got.Cleanup()
	if len(runDirs(t, c.home)) != 0 {
		t.Fatal("Cleanup left the run directory")
	}
}

func TestAppDownloadStalledIsAppError(t *testing.T) {
	c := newAppDownloadCase(t)
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100000")
		_, _ = w.Write(c.payload[:1024])
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	t.Cleanup(func() { close(release); srv.Close() })
	rel := c.release(srv.URL, c.payload, c.signer.SignTauri(c.payload, appDownloadComment))
	_, err := downloadAppArtifact(context.Background(), c.home, rel, appDownloadOptions{UpdaterKey: &c.key, StallTimeout: 300 * time.Millisecond})
	wantCode(t, err, "E_APP_DOWNLOAD_STALLED")
	if !errors.Is(err, artifacts.ErrDownloadStalled) {
		t.Fatalf("does not wrap ErrDownloadStalled: %v", err)
	}
	if len(runDirs(t, c.home)) != 0 {
		t.Fatal("run directory left after a stall")
	}
}

func TestAppDownloadCancelRemovesFiles(t *testing.T) {
	c := newAppDownloadCase(t)
	rel, _ := c.goodRelease(t)
	ctx, cancel := context.WithCancel(context.Background())
	_, err := downloadAppArtifact(ctx, c.home, rel, appDownloadOptions{UpdaterKey: &c.key, Progress: func(e ProgressEvent) {
		if e.Stage == "download" {
			cancel()
		}
	}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if len(runDirs(t, c.home)) != 0 {
		t.Fatal("run directory left after cancel")
	}
}

func TestAppDownloadOversize(t *testing.T) {
	c := newAppDownloadCase(t)
	rel, _ := c.goodRelease(t)
	_, err := downloadAppArtifact(context.Background(), c.home, rel, appDownloadOptions{UpdaterKey: &c.key, MaxBytes: 1024})
	wantCode(t, err, "E_ARTIFACT_DOWNLOAD_TOO_LARGE")
	if len(runDirs(t, c.home)) != 0 {
		t.Fatal("run directory left after oversize")
	}
}

func TestAppDownloadWrongSHA256(t *testing.T) {
	c := newAppDownloadCase(t)
	rel, _ := c.goodRelease(t)
	rel.Artifact.Sha256 = sha256Hex([]byte("something else"))
	_, err := downloadAppArtifact(context.Background(), c.home, rel, appDownloadOptions{UpdaterKey: &c.key})
	wantCode(t, err, "E_APP_SIGNATURE")
	if !errors.Is(err, artifacts.ErrChecksumMismatch) {
		t.Fatalf("does not wrap ErrChecksumMismatch: %v", err)
	}
	if len(runDirs(t, c.home)) != 0 {
		t.Fatal("run directory left after checksum mismatch")
	}
}

func TestAppDownloadWrongUpdaterSignature(t *testing.T) {
	tampered := func(p []byte) []byte { b := append([]byte{}, p...); b[0] ^= 1; return b }
	cases := []struct {
		name  string
		build func(c *appDownloadCase, t *testing.T) (rel appRelease, key *verify.MinisignPublicKey)
		want  error
		calls int32
	}{
		{"other key same id", func(c *appDownloadCase, t *testing.T) (appRelease, *verify.MinisignPublicKey) {
			other, _ := minisigntest.NewSignerWithKeyID(c.signer.KeyID)
			srv := c.serve(t, c.payload)
			return c.release(srv.URL, c.payload, other.SignTauri(c.payload, appDownloadComment)), &c.key
		}, verify.ErrMinisignGlobal, 1},
		{"pinned production key", func(c *appDownloadCase, t *testing.T) (appRelease, *verify.MinisignPublicKey) {
			rel, _ := c.goodRelease(t)
			return rel, nil
		}, verify.ErrMinisignKeyID, 1},
		{"served bytes differ from signed bytes", func(c *appDownloadCase, t *testing.T) (appRelease, *verify.MinisignPublicKey) {
			served := tampered(c.payload)
			srv := c.serve(t, served)
			return c.release(srv.URL, served, c.signer.SignTauri(c.payload, appDownloadComment)), &c.key
		}, verify.ErrMinisignInvalid, 1},
		{"malformed signature", func(c *appDownloadCase, t *testing.T) (appRelease, *verify.MinisignPublicKey) {
			rel, _ := c.goodRelease(t)
			rel.Artifact.UpdaterSignature = "not-a-signature"
			return rel, &c.key
		}, verify.ErrMinisignMalformed, 1},
		{"empty signature", func(c *appDownloadCase, t *testing.T) (appRelease, *verify.MinisignPublicKey) {
			rel, _ := c.goodRelease(t)
			rel.Artifact.UpdaterSignature = ""
			return rel, &c.key
		}, nil, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newAppDownloadCase(t)
			rel, key := tc.build(c, t)
			_, err := downloadAppArtifact(context.Background(), c.home, rel, appDownloadOptions{UpdaterKey: key})
			wantCode(t, err, "E_APP_SIGNATURE")
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("error does not wrap %v: %v", tc.want, err)
			}
			if got := c.requests.Load(); got != tc.calls {
				t.Fatalf("server saw %d requests, want %d", got, tc.calls)
			}
			if len(runDirs(t, c.home)) != 0 {
				t.Fatal("run directory left after a signature failure")
			}
		})
	}
}

func TestAppDownloadDoesNotHoldInstallerLock(t *testing.T) {
	c := newAppDownloadCase(t)
	started := make(chan struct{})
	proceed := make(chan struct{})
	half := len(c.payload) / 2
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "262144")
		_, _ = w.Write(c.payload[:half])
		w.(http.Flusher).Flush()
		close(started)
		select {
		case <-proceed:
		case <-r.Context().Done():
			return
		}
		_, _ = w.Write(c.payload[half:])
	}))
	t.Cleanup(srv.Close)
	rel := c.release(srv.URL, c.payload, c.signer.SignTauri(c.payload, appDownloadComment))
	type result struct {
		art verifiedAppArtifact
		err error
	}
	done := make(chan result, 1)
	go func() {
		art, err := downloadAppArtifact(context.Background(), c.home, rel, appDownloadOptions{UpdaterKey: &c.key})
		done <- result{art, err}
	}()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("download never started")
	}
	if code := runAppLockProbe(t, c.home); code != 0 {
		t.Fatalf("lock probe exit %d mid-download, want 0 (lock free)", code)
	}
	t.Log("lock probe mid-download: exit 0 (lock free)")

	fl, err := lock.NewFileLock(c.home)
	if err != nil {
		t.Fatal(err)
	}
	release, err := fl.Acquire(context.Background(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if code := runAppLockProbe(t, c.home); code != 3 {
		t.Fatalf("positive control: probe exit %d while the test holds the lock, want 3", code)
	}
	t.Log("lock probe while held (positive control): exit 3 (timeout)")
	if err := release(); err != nil {
		t.Fatal(err)
	}

	close(proceed)
	r := <-done
	if r.err != nil {
		t.Fatalf("download: %v", r.err)
	}
	r.art.Cleanup()
}

func TestAppDownloadPrunesStaleRuns(t *testing.T) {
	c := newAppDownloadCase(t)
	root := filepath.Join(c.home, appDownloadsDir)
	for _, n := range []string{"run-old", "run-recent"} {
		if err := os.MkdirAll(filepath.Join(root, n), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(filepath.Join(root, "run-old"), old, old); err != nil {
		t.Fatal(err)
	}
	rel, _ := c.goodRelease(t)
	got, err := downloadAppArtifact(context.Background(), c.home, rel, appDownloadOptions{UpdaterKey: &c.key})
	if err != nil {
		t.Fatal(err)
	}
	defer got.Cleanup()
	if _, err := os.Stat(filepath.Join(root, "run-old")); !os.IsNotExist(err) {
		t.Fatalf("run-old not pruned: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "run-recent")); err != nil {
		t.Fatalf("run-recent removed: %v", err)
	}
}
