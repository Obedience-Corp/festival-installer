package launch

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRun_Success(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "tool")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FESTIVAL_HOME", t.TempDir())
	t.Setenv("PATH", dir)

	res := Run(context.Background(), Spec{Tool: "tool", Args: nil, Title: "tool"})
	if res.Err != nil {
		t.Fatalf("Run: %v", res.Err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit %d", res.ExitCode)
	}
	if res.Path != script {
		// LookPath may return absolute; compare base
		if filepath.Base(res.Path) != "tool" {
			t.Fatalf("path %q", res.Path)
		}
	}
}

func TestRun_NonZeroExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "tool")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FESTIVAL_HOME", t.TempDir())
	t.Setenv("PATH", dir)

	res := Run(context.Background(), Spec{Tool: "tool"})
	if res.ExitCode != 3 {
		t.Fatalf("want exit 3, got %d err=%v", res.ExitCode, res.Err)
	}
}
