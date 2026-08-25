package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectSuite_EmptyHomeDoesNotCreateState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	t.Setenv("PATH", t.TempDir()) // no suite tools

	got, err := DetectSuite(context.Background())
	if err != nil {
		t.Fatalf("DetectSuite: %v", err)
	}
	if got.Kind != OriginAbsent {
		t.Fatalf("Kind=%q, want absent", got.Kind)
	}
	if got.Dual {
		t.Fatal("Dual should be false with no BinDir files")
	}
	if _, err := os.Stat(filepath.Join(home, "state.db")); !os.IsNotExist(err) {
		t.Fatalf("state.db must not exist, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "locks")); !os.IsNotExist(err) {
		t.Fatalf("locks/ must not exist, stat err=%v", err)
	}
}

func TestDetectSuite_LeftoverOnPATH(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)

	bin := t.TempDir()
	writeStub(t, filepath.Join(bin, "camp"))
	t.Setenv("PATH", bin)

	got, err := DetectSuite(context.Background())
	if err != nil {
		t.Fatalf("DetectSuite: %v", err)
	}
	if got.Kind != OriginLeftover {
		t.Fatalf("Kind=%q, want leftover", got.Kind)
	}
	if got.Prefix != bin {
		t.Fatalf("Prefix=%q, want %q", got.Prefix, bin)
	}
	if got.Dual {
		t.Fatal("Dual should be false: no BinDir files")
	}
	if _, err := os.Stat(filepath.Join(home, "state.db")); !os.IsNotExist(err) {
		t.Fatalf("state.db must not exist, stat err=%v", err)
	}
}

func TestDetectSuite_ManagedAndDual(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	binDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeStub(t, filepath.Join(binDir, "camp"))

	other := t.TempDir()
	writeStub(t, filepath.Join(other, "fest"))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+other)

	got, err := DetectSuite(context.Background())
	if err != nil {
		t.Fatalf("DetectSuite: %v", err)
	}
	if got.Kind != OriginManaged {
		t.Fatalf("Kind=%q, want managed", got.Kind)
	}
	if !got.Dual {
		t.Fatal("want Dual: managed BinDir plus another PATH copy")
	}
}

func TestDetectSuite_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := DetectSuite(ctx)
	if err == nil {
		t.Fatal("want cancelled error")
	}
	if got.Kind != OriginAbsent {
		t.Fatalf("Kind=%q, want absent on cancel", got.Kind)
	}
}

func writeStub(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}
