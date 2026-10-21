package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestManagedSelectionPreservesExternalPackage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	pkg := writeVersionedPackagePrefix(t, t.TempDir(), "0.1.0")
	t.Setenv("PATH", pkg)
	managed := filepath.Join(home, "bin")
	if err := os.MkdirAll(managed, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(managed, "camp")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho 9.0.0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(pkg, "camp"))
	ctx := WithManagedToolchain(context.Background())
	if err := refusePackageChannel(context.Background(), false); err == nil {
		t.Fatal("ordinary install should retain ownership refusal")
	}
	if err := refusePackageChannel(ctx, false); err != nil {
		t.Fatal(err)
	}
	selected, err := ResolveTool(ctx, "camp")
	if err != nil || selected != path {
		t.Fatalf("managed selection: %s %v", selected, err)
	}
	selected, err = ResolveTool(context.Background(), "camp")
	if err != nil || selected != filepath.Join(pkg, "camp") {
		t.Fatalf("ordinary selection: %s %v", selected, err)
	}
	after, _ := os.ReadFile(filepath.Join(pkg, "camp"))
	if string(before) != string(after) {
		t.Fatal("package modified")
	}
}
