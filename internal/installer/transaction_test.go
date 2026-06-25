package installer_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Obedience-Corp/obey-installer/internal/installer"
	"github.com/Obedience-Corp/obey-installer/internal/state"
	"github.com/Obedience-Corp/obey-installer/internal/state/lock"
	"github.com/Obedience-Corp/obey-installer/internal/state/receipts"
)

const envLockHelperHome = "OBEY_INSTALLER_LOCK_HELPER_HOME"

func TestMain(m *testing.M) {
	if home := os.Getenv(envLockHelperHome); home != "" {
		runLockHelper(home)
		return
	}
	os.Exit(m.Run())
}

func runLockHelper(home string) {
	l, err := lock.NewFileLock(home)
	if err != nil {
		fmt.Fprintln(os.Stderr, "helper NewFileLock:", err)
		os.Exit(2)
	}
	rel, err := l.Acquire(context.Background(), 5*time.Second)
	if err != nil {
		fmt.Fprintln(os.Stderr, "helper Acquire:", err)
		os.Exit(3)
	}
	fmt.Println("acquired")
	time.Sleep(2 * time.Second)
	_ = rel()
	os.Exit(0)
}

func spawnLockHelper(t *testing.T, home string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestMain$")
	cmd.Env = append(os.Environ(), envLockHelperHome+"="+home)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	buf := make([]byte, 64)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		n, _ := stdout.Read(buf)
		if n > 0 && strings.Contains(string(buf[:n]), "acquired") {
			return cmd
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	t.Fatal("helper never printed 'acquired'")
	return nil
}

func newHomeDB(t *testing.T) (string, *state.DB) {
	t.Helper()
	home := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	db, err := state.OpenDB(ctx, home)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close(context.Background()) })
	return home, db
}

func stageBlob(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("stage blob: %v", err)
	}
	return p
}

func TestCommit_PlacesFilesAndWritesReceipt(t *testing.T) {
	home, db := newHomeDB(t)
	ctx := context.Background()

	tx, err := installer.Begin(ctx, db.Raw(), home)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	src := stageBlob(t, tx.StagingDir(), "camp", "camp-binary")
	dst := filepath.Join(home, "bin", "camp")

	if err := tx.Stage(ctx, installer.StagedFile{StagedPath: src, DestPath: dst, Mode: 0o755}); err != nil {
		t.Fatalf("Stage: %v", err)
	}
	r, err := tx.Commit(ctx, installer.ReceiptInfo{
		PackageID: "obedience-corp/festival", Version: "0.2.10", Channel: "stable",
		Source: "official-obey", ManifestURL: "https://example/manifest.json",
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	info, statErr := os.Stat(dst)
	if statErr != nil {
		t.Fatalf("dst not placed: %v", statErr)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("dst mode = %o, want 0755", info.Mode().Perm())
	}
	got, err := receipts.Get(ctx, db.Raw(), "obedience-corp/festival")
	if err != nil {
		t.Fatalf("Get receipt: %v", err)
	}
	if got.Version != "0.2.10" || len(got.OwnedFiles) != 1 || got.OwnedFiles[0].Path != dst {
		t.Fatalf("receipt wrong: %+v", got)
	}
	if got.Metadata["manifest_url"] != "https://example/manifest.json" {
		t.Fatalf("manifest_url not recorded: %+v", got.Metadata)
	}
	if r.Version != got.Version {
		t.Fatalf("returned receipt %+v disagrees with stored %+v", r, got)
	}
}

func TestCommit_FailureRollsBackEverything(t *testing.T) {
	home, db := newHomeDB(t)
	ctx := context.Background()

	tx, err := installer.Begin(ctx, db.Raw(), home)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	good := stageBlob(t, tx.StagingDir(), "camp", "camp-binary")
	goodDst := filepath.Join(home, "bin", "camp")
	badSrc := filepath.Join(tx.StagingDir(), "missing")
	badDst := filepath.Join(home, "bin", "fest")

	if err := tx.Stage(ctx, installer.StagedFile{StagedPath: good, DestPath: goodDst, Mode: 0o755}); err != nil {
		t.Fatalf("Stage good: %v", err)
	}
	if err := tx.Stage(ctx, installer.StagedFile{StagedPath: badSrc, DestPath: badDst, Mode: 0o755}); err != nil {
		t.Fatalf("Stage bad: %v", err)
	}

	if _, err := tx.Commit(ctx, installer.ReceiptInfo{PackageID: "obedience-corp/festival", Version: "0.2.10", Channel: "stable", Source: "official-obey"}); err == nil {
		t.Fatal("expected commit to fail on missing staged file")
	}
	if _, statErr := os.Stat(goodDst); !os.IsNotExist(statErr) {
		t.Fatalf("good file should have been rolled back, stat err=%v", statErr)
	}
	if _, err := receipts.Get(ctx, db.Raw(), "obedience-corp/festival"); !errors.Is(err, receipts.ErrNotFound) {
		t.Fatalf("expected no receipt after failed commit, got %v", err)
	}
}

func TestSecondBeginBlockedWhileLockHeld(t *testing.T) {
	home := t.TempDir()
	cmd := spawnLockHelper(t, home)
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()

	ctx := context.Background()
	db, err := state.OpenDB(ctx, home)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close(context.Background()) })

	_, err = installer.BeginWithTimeout(ctx, db.Raw(), home, 200*time.Millisecond)
	if !errors.Is(err, lock.ErrLockTimeout) {
		t.Fatalf("expected ErrLockTimeout while another process holds the lock, got %v", err)
	}
}
