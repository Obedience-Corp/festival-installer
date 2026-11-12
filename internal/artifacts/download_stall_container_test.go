//go:build container_fs && !windows

package artifacts_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/Obedience-Corp/festival-installer/internal/artifacts"
	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

func assertEmptyDir(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("download dir not empty: %v", entries)
	}
}

func blockingServer(t *testing.T, fn func(w http.ResponseWriter, r *http.Request, release <-chan struct{})) *httptest.Server {
	t.Helper()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fn(w, r, release) }))
	t.Cleanup(func() { close(release); srv.Close() })
	return srv
}

func waitOrDone(r *http.Request, release <-chan struct{}, d time.Duration) {
	select {
	case <-time.After(d):
	case <-release:
	case <-r.Context().Done():
	}
}

func TestAppDownloaderStallBeforeHeaders(t *testing.T) {
	srv := blockingServer(t, func(w http.ResponseWriter, r *http.Request, release <-chan struct{}) {
		waitOrDone(r, release, 5*time.Second)
	})
	dir := t.TempDir()
	_, err := artifacts.NewStallDownloader(300*time.Millisecond, nil).Download(context.Background(), srv.URL+"/a.bin", dir)
	if !errors.Is(err, artifacts.ErrDownloadStalled) || errpkg.Code(err) != "E_ARTIFACT_STALLED" {
		t.Fatalf("error = %v (code %s)", err, errpkg.Code(err))
	}
	assertEmptyDir(t, dir)
}

func TestAppDownloaderStallMidBody(t *testing.T) {
	srv := blockingServer(t, func(w http.ResponseWriter, r *http.Request, release <-chan struct{}) {
		w.Header().Set("Content-Length", "100000")
		_, _ = w.Write(bytes.Repeat([]byte{1}, 1024))
		w.(http.Flusher).Flush()
		waitOrDone(r, release, 5*time.Second)
	})
	dir := t.TempDir()
	_, err := artifacts.NewStallDownloader(300*time.Millisecond, nil).Download(context.Background(), srv.URL+"/a.bin", dir)
	if !errors.Is(err, artifacts.ErrDownloadStalled) || errpkg.Code(err) != "E_ARTIFACT_STALLED" {
		t.Fatalf("error = %v (code %s)", err, errpkg.Code(err))
	}
	assertEmptyDir(t, dir)
}

func TestAppDownloaderSlowSteadyBodyNotStalled(t *testing.T) {
	const chunks = 15
	srv := blockingServer(t, func(w http.ResponseWriter, r *http.Request, release <-chan struct{}) {
		w.Header().Set("Content-Length", "15360")
		for i := 0; i < chunks; i++ {
			_, _ = w.Write(bytes.Repeat([]byte{2}, 1024))
			w.(http.Flusher).Flush()
			waitOrDone(r, release, 100*time.Millisecond)
		}
	})
	dir := t.TempDir()
	path, err := artifacts.NewStallDownloader(300*time.Millisecond, nil).Download(context.Background(), srv.URL+"/a.bin", dir)
	if err != nil {
		t.Fatalf("steady slow body failed: %v", err)
	}
	if st, err := os.Stat(path); err != nil || st.Size() != chunks*1024 {
		t.Fatalf("file: %v %v", st, err)
	}
}

func TestAppDownloaderProgress(t *testing.T) {
	body := bytes.Repeat([]byte{3}, 300_000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	var seen []int64
	var lastTotal int64
	d := artifacts.NewStallDownloader(5*time.Second, func(written, total int64) {
		seen = append(seen, written)
		lastTotal = total
	})
	if _, err := d.Download(context.Background(), srv.URL+"/a.bin", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if len(seen) == 0 {
		t.Fatal("no progress reported")
	}
	for i := 1; i < len(seen); i++ {
		if seen[i] < seen[i-1] {
			t.Fatalf("progress went backwards: %v", seen)
		}
	}
	if seen[len(seen)-1] != int64(len(body)) || lastTotal != int64(len(body)) {
		t.Fatalf("final written %d total %d, want %d", seen[len(seen)-1], lastTotal, len(body))
	}
}

func TestAppDownloaderCancelMidBody(t *testing.T) {
	srv := blockingServer(t, func(w http.ResponseWriter, r *http.Request, release <-chan struct{}) {
		w.Header().Set("Content-Length", "100000")
		_, _ = w.Write(bytes.Repeat([]byte{4}, 1024))
		w.(http.Flusher).Flush()
		waitOrDone(r, release, 5*time.Second)
	})
	ctx, cancel := context.WithCancel(context.Background())
	dir := t.TempDir()
	d := artifacts.NewStallDownloader(30*time.Second, func(written, total int64) { cancel() })
	_, err := d.Download(ctx, srv.URL+"/a.bin", dir)
	if !errors.Is(err, context.Canceled) || errors.Is(err, artifacts.ErrDownloadStalled) {
		t.Fatalf("error = %v", err)
	}
	assertEmptyDir(t, dir)
}
