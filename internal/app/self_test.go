package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSelf_ManagedViaSymlink(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", dir)
	t.Setenv("FESTIVAL_HOME", "")

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	managed := filepath.Join(binDir, selfBinaryName)
	if err := os.Symlink(exe, managed); err != nil {
		t.Fatalf("symlink self as managed: %v", err)
	}

	placement, path, err := ResolveSelf(context.Background())
	if err != nil {
		t.Fatalf("ResolveSelf: %v", err)
	}
	if placement != SelfManaged {
		t.Fatalf("placement = %q, want %q", placement, SelfManaged)
	}
	if path != managed {
		t.Fatalf("path = %q, want %q", path, managed)
	}
}

func TestResolveSelf_External(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", dir)
	t.Setenv("FESTIVAL_HOME", "")

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	// No symlink at binDir/festival, so the running test binary is external.
	placement, path, err := ResolveSelf(context.Background())
	if err != nil {
		t.Fatalf("ResolveSelf: %v", err)
	}
	if placement != SelfExternal {
		t.Fatalf("placement = %q, want %q", placement, SelfExternal)
	}
	if path != exe {
		t.Fatalf("path = %q, want %q", path, exe)
	}
}

func TestResolveSelf_RelativePathInvocation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", dir)
	t.Setenv("FESTIVAL_HOME", "")

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	managed := filepath.Join(binDir, selfBinaryName)
	// Relative symlink target, mirroring a relative argv[0] invocation.
	rel, err := filepath.Rel(binDir, exe)
	if err != nil {
		t.Fatalf("filepath.Rel: %v", err)
	}
	if err := os.Symlink(rel, managed); err != nil {
		t.Fatalf("symlink self as managed (relative): %v", err)
	}

	placement, path, err := ResolveSelf(context.Background())
	if err != nil {
		t.Fatalf("ResolveSelf: %v", err)
	}
	if placement != SelfManaged {
		t.Fatalf("placement = %q, want %q", placement, SelfManaged)
	}
	if path != managed {
		t.Fatalf("path = %q, want %q", path, managed)
	}
}

func TestResolveSelf_PathLookupInvocation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", dir)
	t.Setenv("FESTIVAL_HOME", "")

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	managed := filepath.Join(binDir, selfBinaryName)
	if err := os.Symlink(exe, managed); err != nil {
		t.Fatalf("symlink self as managed: %v", err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	placement, path, err := ResolveSelf(context.Background())
	if err != nil {
		t.Fatalf("ResolveSelf: %v", err)
	}
	if placement != SelfManaged {
		t.Fatalf("placement = %q, want %q", placement, SelfManaged)
	}
	if path != managed {
		t.Fatalf("path = %q, want %q", path, managed)
	}
}

func TestResolveSelf_ContextCancelledSkipsFilesystem(t *testing.T) {
	// If ResolveSelf reached state.BinDir before checking ctx, this would
	// fail with E_HOME_NOT_ABS instead of the context error.
	t.Setenv("OBEY_INSTALLER_HOME", "relative/not/absolute")
	t.Setenv("FESTIVAL_HOME", "")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	placement, path, err := ResolveSelf(ctx)
	if placement != SelfUnknown {
		t.Fatalf("placement = %q, want %q", placement, SelfUnknown)
	}
	if path != "" {
		t.Fatalf("path = %q, want empty", path)
	}
	if err != context.Canceled {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
