package source_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Obedience-Corp/obey-installer/internal/source"
)

func gitEnv() []string {
	return append(os.Environ(),
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@example.com",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
	)
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func newFixtureRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitRun(t, dir, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("v1\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "init")
	return dir
}

func TestClone_ReturnsCommitAndPopulatesDest(t *testing.T) {
	fixture := newFixtureRepo(t)
	t.Setenv("OBEY_INSTALLER_HOME", t.TempDir())
	ctx := context.Background()

	dest, err := source.CloneDir(ctx, "fix")
	if err != nil {
		t.Fatalf("CloneDir: %v", err)
	}
	commit, err := source.Clone(ctx, fixture, dest)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if commit == "" {
		t.Fatal("expected non-empty commit")
	}
	if _, err := os.Stat(filepath.Join(dest, "README.md")); err != nil {
		t.Fatalf("dest not populated: %v", err)
	}
}

func TestPull_AdvancesCommit(t *testing.T) {
	fixture := newFixtureRepo(t)
	t.Setenv("OBEY_INSTALLER_HOME", t.TempDir())
	ctx := context.Background()

	dest, err := source.CloneDir(ctx, "fix")
	if err != nil {
		t.Fatalf("CloneDir: %v", err)
	}
	first, err := source.Clone(ctx, fixture, dest)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}

	if err := os.WriteFile(filepath.Join(fixture, "README.md"), []byte("v2\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	gitRun(t, fixture, "add", ".")
	gitRun(t, fixture, "commit", "-m", "second")

	second, err := source.Pull(ctx, dest)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if second == first {
		t.Fatalf("expected commit to advance, still %s", second)
	}
}

func TestRemoveClone_DeletesInsideCache(t *testing.T) {
	fixture := newFixtureRepo(t)
	t.Setenv("OBEY_INSTALLER_HOME", t.TempDir())
	ctx := context.Background()

	dest, err := source.CloneDir(ctx, "fix")
	if err != nil {
		t.Fatalf("CloneDir: %v", err)
	}
	if _, err := source.Clone(ctx, fixture, dest); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if err := source.RemoveClone(ctx, dest); err != nil {
		t.Fatalf("RemoveClone: %v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("expected dest removed, stat err=%v", err)
	}
}

func TestRemoveClone_OutsideCacheRefused(t *testing.T) {
	t.Setenv("OBEY_INSTALLER_HOME", t.TempDir())
	ctx := context.Background()

	outside := filepath.Join(t.TempDir(), "keepme")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := source.RemoveClone(ctx, outside); !errors.Is(err, source.ErrNotInCache) {
		t.Fatalf("expected ErrNotInCache, got %v", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside path should still exist: %v", err)
	}
}

func TestClone_RespectsContextCancellation(t *testing.T) {
	fixture := newFixtureRepo(t)
	t.Setenv("OBEY_INSTALLER_HOME", t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()

	dest := filepath.Join(t.TempDir(), "cancelled")
	if _, err := source.Clone(ctx, fixture, dest); err == nil {
		t.Fatal("expected clone to fail under cancelled context")
	}
}
