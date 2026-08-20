package launch

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestResolve_ManagedBinPreferred(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	t.Setenv("OBEY_INSTALLER_HOME", "")
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	tool := filepath.Join(bin, "camp")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\necho managed\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Also put a decoy earlier on PATH. Managed must win.
	decoyDir := t.TempDir()
	decoy := filepath.Join(decoyDir, "camp")
	if err := os.WriteFile(decoy, []byte("#!/bin/sh\necho path\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", decoyDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	got, err := Resolve(context.Background(), "camp")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != tool {
		t.Fatalf("want managed %q, got %q", tool, got)
	}
}

func TestResolve_FallsBackToPATH(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	t.Setenv("OBEY_INSTALLER_HOME", "")
	// empty managed bin
	_ = os.MkdirAll(filepath.Join(home, "bin"), 0o755)

	decoyDir := t.TempDir()
	decoy := filepath.Join(decoyDir, "fest")
	if err := os.WriteFile(decoy, []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", decoyDir)

	got, err := Resolve(context.Background(), "fest")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != decoy {
		t.Fatalf("want PATH %q, got %q", decoy, got)
	}
}

func TestResolve_Missing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	t.Setenv("PATH", t.TempDir())
	_, err := Resolve(context.Background(), "definitely-not-a-real-tool-xyz")
	if err == nil {
		t.Fatal("expected not found")
	}
}

func TestResolve_RejectsPath(t *testing.T) {
	_, err := Resolve(context.Background(), "../camp")
	if err == nil {
		t.Fatal("expected reject")
	}
}

func TestCatalog_NonEmpty(t *testing.T) {
	c := Catalog()
	if len(c) == 0 {
		t.Fatal("empty catalog")
	}
	for _, e := range c {
		if e.Label == "" || e.Spec.Tool == "" {
			t.Fatalf("invalid entry: %+v", e)
		}
	}
}
