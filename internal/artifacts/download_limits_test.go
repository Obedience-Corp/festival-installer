package artifacts_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/artifacts"
)

func TestDownload_SizeCap(t *testing.T) {
	tests := []struct {
		name        string
		maxBytes    int64
		advertiseCL int64
		bodyLen     int
		wantErr     bool
	}{
		{name: "over limit via content-length", maxBytes: 1024, advertiseCL: 1 << 20, bodyLen: 200, wantErr: true},
		{name: "over limit via chunked stream", maxBytes: 16, advertiseCL: -1, bodyLen: 200, wantErr: true},
		{name: "under limit succeeds", maxBytes: 1 << 20, advertiseCL: 0, bodyLen: 512, wantErr: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case tc.advertiseCL < 0:
					w.WriteHeader(http.StatusOK)
					flushChunked(t, w)
					_, _ = w.Write(make([]byte, tc.bodyLen))
				case tc.advertiseCL > int64(tc.bodyLen):
					w.Header().Set("Content-Length", strconv.FormatInt(tc.advertiseCL, 10))
					w.WriteHeader(http.StatusOK)
					flushChunked(t, w)
					_, _ = w.Write(make([]byte, tc.bodyLen))
				default:
					_, _ = w.Write(make([]byte, tc.bodyLen))
				}
			}))
			t.Cleanup(srv.Close)

			d := &artifacts.Downloader{Client: srv.Client(), MaxBytes: tc.maxBytes}
			dest := t.TempDir()
			path, err := d.Download(context.Background(), srv.URL+"/artifact.tar.gz", dest)

			if tc.wantErr {
				if !errors.Is(err, artifacts.ErrDownloadTooLarge) {
					t.Fatalf("expected ErrDownloadTooLarge, got %v", err)
				}
				entries, _ := os.ReadDir(dest)
				if len(entries) != 0 {
					t.Fatalf("expected empty dest after over-limit download, got %d entries", len(entries))
				}
				return
			}

			if err != nil {
				t.Fatalf("Download: %v", err)
			}
			info, statErr := os.Stat(path)
			if statErr != nil {
				t.Fatalf("stat downloaded file: %v", statErr)
			}
			if info.Size() != int64(tc.bodyLen) {
				t.Fatalf("expected %d bytes on disk, got %d", tc.bodyLen, info.Size())
			}
		})
	}
}

func TestDownload_DefaultCapApplied(t *testing.T) {
	d := artifacts.NewDownloader()
	if d.MaxBytes != artifacts.DefaultExtractLimits.MaxTotalBytes {
		t.Fatalf("expected default MaxBytes %d, got %d", artifacts.DefaultExtractLimits.MaxTotalBytes, d.MaxBytes)
	}
}

func flushChunked(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	fl, ok := w.(http.Flusher)
	if !ok {
		t.Fatal("response writer does not support flushing")
	}
	fl.Flush()
}
