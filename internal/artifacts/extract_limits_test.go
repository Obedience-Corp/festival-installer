package artifacts

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeTarGz(t *testing.T, files map[string]int) string {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, size := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(size), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(make([]byte, size)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "a.tar.gz")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExtractTarGz_TotalBytesCap(t *testing.T) {
	src := writeTarGz(t, map[string]int{"big.bin": 500})
	lim := ExtractLimits{MaxEntries: 100, MaxFileBytes: 1 << 20, MaxTotalBytes: 100}
	err := ExtractTarGzWithLimits(context.Background(), src, t.TempDir(), lim)
	if !errors.Is(err, ErrArchiveTooLarge) {
		t.Fatalf("expected ErrArchiveTooLarge, got %v", err)
	}
}

func TestExtractTarGz_EntryCountCap(t *testing.T) {
	src := writeTarGz(t, map[string]int{"a": 1, "b": 1, "c": 1})
	lim := ExtractLimits{MaxEntries: 1, MaxFileBytes: 1 << 20, MaxTotalBytes: 1 << 20}
	err := ExtractTarGzWithLimits(context.Background(), src, t.TempDir(), lim)
	if !errors.Is(err, ErrArchiveTooLarge) {
		t.Fatalf("expected ErrArchiveTooLarge, got %v", err)
	}
}

func TestExtractTarGz_WithinLimits(t *testing.T) {
	src := writeTarGz(t, map[string]int{"ok.bin": 50})
	lim := ExtractLimits{MaxEntries: 100, MaxFileBytes: 1 << 20, MaxTotalBytes: 1 << 20}
	if err := ExtractTarGzWithLimits(context.Background(), src, t.TempDir(), lim); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExtractTarGz_TotalBytesCapLeavesNoPartialFiles(t *testing.T) {
	src := writeTarGz(t, map[string]int{"first.bin": 10, "second.bin": 500})
	dest := filepath.Join(t.TempDir(), "extracted")
	lim := ExtractLimits{MaxEntries: 100, MaxFileBytes: 1 << 20, MaxTotalBytes: 100}
	err := ExtractTarGzWithLimits(context.Background(), src, dest, lim)
	if !errors.Is(err, ErrArchiveTooLarge) {
		t.Fatalf("expected ErrArchiveTooLarge, got %v", err)
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Fatalf("expected dest removed after rejected extraction, stat err=%v", statErr)
	}
}
