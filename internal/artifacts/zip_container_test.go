//go:build container_fs

package artifacts_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/artifacts"
)

func writeZip(t *testing.T, entries map[string]string, symlink string) string {
	t.Helper()
	var body bytes.Buffer
	w := zip.NewWriter(&body)
	for name, contents := range entries {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(contents)); err != nil {
			t.Fatal(err)
		}
	}
	if symlink != "" {
		h := &zip.FileHeader{Name: symlink}
		h.SetMode(os.ModeSymlink | 0o777)
		f, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte("camp.exe")); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "festival-windows-amd64.zip")
	if err := os.WriteFile(archive, body.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return archive
}

func TestExtractZipWindowsSuite(t *testing.T) {
	archive := writeZip(t, map[string]string{
		"camp.exe": "camp-v1", "fest.exe": "fest-v1", "festival.exe": "festival-v1",
		"README.md": "suite",
	}, "")
	dest := filepath.Join(t.TempDir(), "extract")
	if err := artifacts.ExtractZip(context.Background(), archive, dest); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"camp.exe": "camp-v1", "fest.exe": "fest-v1", "festival.exe": "festival-v1"} {
		got, err := os.ReadFile(filepath.Join(dest, name))
		if err != nil || string(got) != want {
			t.Fatalf("%s = %q, %v; want %q", name, got, err, want)
		}
	}
}

func TestExtractZipRejectsUnsafeMembersAndCleansStaging(t *testing.T) {
	for _, tc := range []struct {
		name string
		file string
		link string
	}{
		{"traversal", "../escape.exe", ""},
		{"windows separator", `..\\escape.exe`, ""},
		{"alternate stream", `camp.exe:evil`, ""},
		{"symlink", "camp.exe", "alias.exe"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			archive := writeZip(t, map[string]string{tc.file: "bytes"}, tc.link)
			root := t.TempDir()
			dest := filepath.Join(root, "extract")
			err := artifacts.ExtractZip(context.Background(), archive, dest)
			if !errors.Is(err, artifacts.ErrUnsafePath) {
				t.Fatalf("want unsafe path, got %v", err)
			}
			if _, err := os.Stat(dest); !os.IsNotExist(err) {
				t.Fatalf("failed extraction left staging: %v", err)
			}
			if _, err := os.Stat(filepath.Join(root, "escape.exe")); !os.IsNotExist(err) {
				t.Fatalf("member escaped staging: %v", err)
			}
		})
	}
}

func TestExtractZipRejectsSizeAndCancellation(t *testing.T) {
	archive := writeZip(t, map[string]string{"camp.exe": "oversized"}, "")
	dest := filepath.Join(t.TempDir(), "extract")
	limits := artifacts.ExtractLimits{MaxEntries: 10, MaxFileBytes: 4, MaxTotalBytes: 4}
	if err := artifacts.ExtractZipWithLimits(context.Background(), archive, dest, limits); !errors.Is(err, artifacts.ErrArchiveTooLarge) {
		t.Fatalf("want size error, got %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := artifacts.ExtractZip(ctx, archive, dest); !errors.Is(err, context.Canceled) {
		t.Fatalf("want cancellation, got %v", err)
	}
}
