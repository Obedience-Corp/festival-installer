package artifacts_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/artifacts"
)

func redirectingClient(t *testing.T, tlsSrv *httptest.Server, fakeHost string, plainSrv *httptest.Server) *http.Client {
	t.Helper()
	client := tlsSrv.Client()
	transport := client.Transport.(*http.Transport).Clone()
	realAddr := plainSrv.Listener.Addr().String()
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if addr == fakeHost {
			addr = realAddr
		}
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	client.Transport = transport
	client.CheckRedirect = artifacts.CheckRedirect
	return client
}

func makeTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write header %s: %v", name, err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatalf("write body %s: %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gz close: %v", err)
	}
	return buf.Bytes()
}

func makeMaliciousTarGz(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := "pwned"
	hdr := &tar.Header{Name: "../escape.txt", Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if _, err := tw.Write([]byte(body)); err != nil {
		t.Fatalf("write body: %v", err)
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func makeSymlinkTarGz(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{Name: "camplink", Linkname: "camp", Mode: 0o777, Typeflag: tar.TypeSymlink}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write header: %v", err)
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func serve(t *testing.T, body []byte, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestVerifySHA256_Mismatch(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "blob")
	if err := os.WriteFile(p, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	err := artifacts.VerifySHA256(context.Background(), p, sha256Hex([]byte("goodbye")))
	if !errors.Is(err, artifacts.ErrChecksumMismatch) {
		t.Fatalf("expected ErrChecksumMismatch, got %v", err)
	}
}

func TestExtractTarGz_RejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	arc := filepath.Join(dir, "evil.tar.gz")
	if err := os.WriteFile(arc, makeMaliciousTarGz(t), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	dest := filepath.Join(dir, "out")
	err := artifacts.ExtractTarGz(context.Background(), arc, dest)
	if !errors.Is(err, artifacts.ErrUnsafePath) {
		t.Fatalf("expected ErrUnsafePath, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "escape.txt")); statErr == nil {
		t.Fatal("traversal file escaped to parent dir")
	}
}

func TestExtractTarGz_RejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	arc := filepath.Join(dir, "link.tar.gz")
	if err := os.WriteFile(arc, makeSymlinkTarGz(t), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	dest := filepath.Join(dir, "out")
	err := artifacts.ExtractTarGz(context.Background(), arc, dest)
	if !errors.Is(err, artifacts.ErrUnsafePath) {
		t.Fatalf("expected ErrUnsafePath for symlink member, got %v", err)
	}
	if _, statErr := os.Lstat(filepath.Join(dest, "camplink")); statErr == nil {
		t.Fatal("symlink member should not have been written")
	}
}

func TestExtractTarGz_Success(t *testing.T) {
	dir := t.TempDir()
	arc := filepath.Join(dir, "ok.tar.gz")
	if err := os.WriteFile(arc, makeTarGz(t, map[string]string{"bin/camp": "camp-bytes", "bin/fest": "fest-bytes"}), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	dest := filepath.Join(dir, "out")
	if err := artifacts.ExtractTarGz(context.Background(), arc, dest); err != nil {
		t.Fatalf("ExtractTarGz: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "bin", "camp"))
	if err != nil || string(got) != "camp-bytes" {
		t.Fatalf("extracted camp wrong: %q err=%v", got, err)
	}
}

func TestDownload_Success(t *testing.T) {
	body := makeTarGz(t, map[string]string{"camp": "binary-bytes"})
	srv := serve(t, body, http.StatusOK)
	d := &artifacts.Downloader{Client: srv.Client()}

	dest := t.TempDir()
	path, err := d.Download(context.Background(), srv.URL+"/festival-0.2.10-macOS-all.tar.gz", dest)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if err := artifacts.VerifySHA256(context.Background(), path, sha256Hex(body)); err != nil {
		t.Fatalf("verify downloaded artifact: %v", err)
	}
}

func TestDownload_BadStatusLeavesNoFile(t *testing.T) {
	srv := serve(t, []byte("nope"), http.StatusNotFound)
	d := &artifacts.Downloader{Client: srv.Client()}
	dest := t.TempDir()

	_, err := d.Download(context.Background(), srv.URL+"/missing.tar.gz", dest)
	if !errors.Is(err, artifacts.ErrHTTPStatus) {
		t.Fatalf("expected ErrHTTPStatus, got %v", err)
	}
	entries, _ := os.ReadDir(dest)
	if len(entries) != 0 {
		t.Fatalf("expected empty dest after failed download, got %d entries", len(entries))
	}
}

func TestDownload_CancelLeavesNoPartial(t *testing.T) {
	body := makeTarGz(t, map[string]string{"camp": "binary-bytes"})
	srv := serve(t, body, http.StatusOK)
	d := &artifacts.Downloader{Client: srv.Client()}
	dest := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := d.Download(ctx, srv.URL+"/x.tar.gz", dest)
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
	entries, _ := os.ReadDir(dest)
	if len(entries) != 0 {
		t.Fatalf("cancelled download left %d entries", len(entries))
	}
}

func TestDownload_RejectsRedirectDowngrade(t *testing.T) {
	body := makeTarGz(t, map[string]string{"camp": "binary-bytes"})
	plainSrv := serve(t, body, http.StatusOK)
	plainURL, err := url.Parse(plainSrv.URL)
	if err != nil {
		t.Fatalf("parse plain url: %v", err)
	}
	fakeHost := "insecure.example.com:" + plainURL.Port()

	tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://"+fakeHost+"/download", http.StatusFound)
	}))
	t.Cleanup(tlsSrv.Close)

	client := redirectingClient(t, tlsSrv, fakeHost, plainSrv)
	d := &artifacts.Downloader{Client: client}
	dest := t.TempDir()

	_, err = d.Download(context.Background(), tlsSrv.URL+"/a.tar.gz", dest)
	if !errors.Is(err, artifacts.ErrInsecureURL) {
		t.Fatalf("expected ErrInsecureURL for downgraded redirect, got %v", err)
	}
	entries, _ := os.ReadDir(dest)
	if len(entries) != 0 {
		t.Fatalf("expected empty dest after rejected redirect, got %d entries", len(entries))
	}
}

func TestAtomicMove_SameDir(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "sub", "dst")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := artifacts.AtomicMove(context.Background(), src, dst); err != nil {
		t.Fatalf("AtomicMove: %v", err)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Fatalf("dst missing: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("src should be gone, stat err=%v", err)
	}
}
