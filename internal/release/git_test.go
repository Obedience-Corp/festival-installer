package release_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/artifacts"
	"github.com/Obedience-Corp/festival-installer/internal/release"
)

func gitEnv() []string {
	return append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
	)
}

func gitRepoWithTags(t *testing.T, tags ...string) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = gitEnv()
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	run("add", ".")
	run("commit", "-m", "init")
	for _, tag := range tags {
		run("tag", tag)
	}
	return dir
}

func defaultToken(goos, goarch string) (string, string) {
	osMap := map[string]string{"darwin": "macOS", "linux": "linux"}
	archMap := map[string]string{"amd64": "x86_64", "arm64": "arm64"}
	return osMap[goos], archMap[goarch]
}

func TestResolve_GitLatestStable(t *testing.T) {
	repo := gitRepoWithTags(t, "v0.1.1", "v0.1.2", "v0.1.3")
	osTok, archTok := defaultToken(runtime.GOOS, runtime.GOARCH)
	assetName := fmt.Sprintf("camp-graph-0.1.3-%s-%s.tar.gz", osTok, archTok)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "deadbeef  %s\nother  unrelated.tar.gz\n", assetName)
	}))
	t.Cleanup(srv.Close)

	src := release.Source{
		Type:         "git",
		Repo:         repo,
		AssetURL:     srv.URL + "/dl/camp-graph-{version}-{os}-{arch}.tar.gz",
		ChecksumsURL: srv.URL + "/dl/checksums",
	}
	got, err := release.NewResolver().Resolve(context.Background(), src, "stable", runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Version != "0.1.3" {
		t.Fatalf("version = %q, want 0.1.3 (highest tag)", got.Version)
	}
	if got.URL != srv.URL+"/dl/"+assetName {
		t.Fatalf("url = %q, want .../%s", got.URL, assetName)
	}
	if got.Sha256 != "deadbeef" || !got.IsArchive {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestResolve_GitChannelSkipsPrerelease(t *testing.T) {
	repo := gitRepoWithTags(t, "v0.1.3", "v0.2.0-rc.1")
	osTok, archTok := defaultToken(runtime.GOOS, runtime.GOARCH)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "s130  camp-graph-0.1.3-%[1]s-%[2]s.tar.gz\nrc  camp-graph-0.2.0-rc.1-%[1]s-%[2]s.tar.gz\n", osTok, archTok)
	}))
	t.Cleanup(srv.Close)

	src := release.Source{
		Type:         "git",
		Repo:         repo,
		AssetURL:     srv.URL + "/dl/camp-graph-{version}-{os}-{arch}.tar.gz",
		ChecksumsURL: srv.URL + "/dl/checksums",
	}
	r := release.NewResolver()

	stable, err := r.Resolve(context.Background(), src, "stable", runtime.GOOS, runtime.GOARCH)
	if err != nil || stable.Version != "0.1.3" {
		t.Fatalf("stable should pick 0.1.3 (skip rc), got %+v err=%v", stable, err)
	}
	rc, err := r.Resolve(context.Background(), src, "rc", runtime.GOOS, runtime.GOARCH)
	if err != nil || rc.Version != "0.2.0-rc.1" {
		t.Fatalf("rc should pick 0.2.0-rc.1, got %+v err=%v", rc, err)
	}
}

func TestResolve_RejectsChecksumRedirectDowngrade(t *testing.T) {
	repo := gitRepoWithTags(t, "v0.1.0")

	plainSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "deadbeef  asset.tar.gz")
	}))
	t.Cleanup(plainSrv.Close)
	plainURL, err := url.Parse(plainSrv.URL)
	if err != nil {
		t.Fatalf("parse plain url: %v", err)
	}
	fakeHost := "insecure.example.com:" + plainURL.Port()

	tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://"+fakeHost+"/checksums", http.StatusFound)
	}))
	t.Cleanup(tlsSrv.Close)

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

	r := release.NewResolver()
	r.Client = client

	src := release.Source{
		Type:         "git",
		Repo:         repo,
		AssetURL:     tlsSrv.URL + "/dl/camp-graph-{version}-{os}-{arch}.tar.gz",
		ChecksumsURL: tlsSrv.URL + "/dl/checksums",
	}
	_, err = r.Resolve(context.Background(), src, "stable", runtime.GOOS, runtime.GOARCH)
	if !errors.Is(err, artifacts.ErrInsecureURL) {
		t.Fatalf("expected ErrInsecureURL for downgraded checksum redirect, got %v", err)
	}
}
