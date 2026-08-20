package installer_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Obedience-Corp/festival-installer/internal/installer"
	"github.com/Obedience-Corp/festival-installer/internal/state"
	"github.com/Obedience-Corp/festival-installer/internal/state/lock"
	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
)

const envLockHelperHome = "FESTIVAL_INSTALLER_LOCK_HELPER_HOME"

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
	if got.Metadata["install_journal_id"] == "" {
		t.Fatalf("install journal id not recorded: %+v", got.Metadata)
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

func TestCommit_UpgradeReplacesPriorBinary(t *testing.T) {
	home, db := newHomeDB(t)
	ctx := context.Background()

	binDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	dst := filepath.Join(binDir, "camp")
	if err := os.WriteFile(dst, []byte("old-camp"), 0o755); err != nil {
		t.Fatalf("seed camp: %v", err)
	}

	tx, err := installer.Begin(ctx, db.Raw(), home)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	src := stageBlob(t, tx.StagingDir(), "camp", "new-camp")
	if err := tx.Stage(ctx, installer.StagedFile{StagedPath: src, DestPath: dst, Mode: 0o755}); err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if _, err := tx.Commit(ctx, installer.ReceiptInfo{PackageID: "obedience-corp/festival", Version: "0.3.0", Channel: "stable", Source: "official-obey"}); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read upgraded camp: %v", err)
	}
	if string(got) != "new-camp" {
		t.Fatalf("camp = %q, want new-camp after upgrade", got)
	}
	entries, err := os.ReadDir(binDir)
	if err != nil {
		t.Fatalf("read bin: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "backup-") {
			t.Fatalf("backup leaked into bin dir: %s", e.Name())
		}
	}
}

func TestCommit_FailureRestoresPriorBinaries(t *testing.T) {
	home, db := newHomeDB(t)
	ctx := context.Background()

	binDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	campDst := filepath.Join(binDir, "camp")
	festDst := filepath.Join(binDir, "fest")
	if err := os.WriteFile(campDst, []byte("old-camp"), 0o755); err != nil {
		t.Fatalf("seed camp: %v", err)
	}
	if err := os.WriteFile(festDst, []byte("old-fest"), 0o755); err != nil {
		t.Fatalf("seed fest: %v", err)
	}

	tx, err := installer.Begin(ctx, db.Raw(), home)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	good := stageBlob(t, tx.StagingDir(), "camp", "new-camp")
	badSrc := filepath.Join(tx.StagingDir(), "missing")

	if err := tx.Stage(ctx, installer.StagedFile{StagedPath: good, DestPath: campDst, Mode: 0o755}); err != nil {
		t.Fatalf("Stage good: %v", err)
	}
	if err := tx.Stage(ctx, installer.StagedFile{StagedPath: badSrc, DestPath: festDst, Mode: 0o755}); err != nil {
		t.Fatalf("Stage bad: %v", err)
	}

	if _, err := tx.Commit(ctx, installer.ReceiptInfo{PackageID: "obedience-corp/festival", Version: "0.3.0", Channel: "stable", Source: "official-obey"}); err == nil {
		t.Fatal("expected commit to fail on missing staged file")
	}

	gotCamp, err := os.ReadFile(campDst)
	if err != nil {
		t.Fatalf("prior camp should be restored: %v", err)
	}
	if string(gotCamp) != "old-camp" {
		t.Fatalf("camp = %q, want prior binary restored", gotCamp)
	}
	gotFest, err := os.ReadFile(festDst)
	if err != nil {
		t.Fatalf("prior fest should be restored: %v", err)
	}
	if string(gotFest) != "old-fest" {
		t.Fatalf("fest = %q, want prior binary restored", gotFest)
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

func TestBeginWaitsForLockBeforeReconciling(t *testing.T) {
	home, db := newHomeDB(t)
	ctx := context.Background()
	dest := filepath.Join(home, "bin", "camp")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("live"), 0o755); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(home, "backup-camp")
	if err := os.WriteFile(backup, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	intent, err := json.Marshal(map[string]any{
		"id":         "active-transaction",
		"package_id": "obedience-corp/festival",
		"placed":     []map[string]string{{"dest": dest, "backup": backup}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, installer.JournalName), intent, 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := spawnLockHelper(t, home)
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	if _, err := installer.BeginWithTimeout(ctx, db.Raw(), home, 200*time.Millisecond); !errors.Is(err, lock.ErrLockTimeout) {
		t.Fatalf("expected lock timeout, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, installer.JournalName)); err != nil {
		t.Fatalf("journal should remain while another transaction owns the lock: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "live" {
		t.Fatalf("live transaction was reconciled while lock was held: %q", got)
	}
}

func TestCommit_UpgradeBackupsOutsideStaging(t *testing.T) {
	home, db := newHomeDB(t)
	ctx := context.Background()

	binDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	dst := filepath.Join(binDir, "camp")
	if err := os.WriteFile(dst, []byte("old-camp"), 0o755); err != nil {
		t.Fatalf("seed camp: %v", err)
	}

	tx, err := installer.Begin(ctx, db.Raw(), home)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	// Hold the staging dir path, then observe that durable backups are not under it.
	staging := tx.StagingDir()
	src := stageBlob(t, staging, "camp", "new-camp")
	if err := tx.Stage(ctx, installer.StagedFile{StagedPath: src, DestPath: dst, Mode: 0o755}); err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if _, err := tx.Commit(ctx, installer.ReceiptInfo{
		PackageID: "obedience-corp/festival", Version: "0.3.0", Channel: "stable", Source: "official-obey",
	}); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Staging is cleaned on success; durable intent-backups dir must also be gone.
	if _, err := os.Stat(staging); !os.IsNotExist(err) {
		t.Fatalf("staging should be removed after successful commit, err=%v", err)
	}
	if entries, err := os.ReadDir(filepath.Join(home, installer.IntentBackupsDir)); err == nil && len(entries) > 0 {
		t.Fatalf("intent backups should be cleared after success: %v", entries)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new-camp" {
		t.Fatalf("camp = %q, want new-camp", got)
	}
}

func TestCommit_FailedUpgradeRestoresViaJournal(t *testing.T) {
	home, db := newHomeDB(t)
	ctx := context.Background()

	binDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	campDst := filepath.Join(binDir, "camp")
	festDst := filepath.Join(binDir, "fest")
	if err := os.WriteFile(campDst, []byte("old-camp"), 0o755); err != nil {
		t.Fatalf("seed camp: %v", err)
	}
	if err := os.WriteFile(festDst, []byte("old-fest"), 0o755); err != nil {
		t.Fatalf("seed fest: %v", err)
	}

	tx, err := installer.Begin(ctx, db.Raw(), home)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	good := stageBlob(t, tx.StagingDir(), "camp", "new-camp")
	badSrc := filepath.Join(tx.StagingDir(), "missing")
	if err := tx.Stage(ctx, installer.StagedFile{StagedPath: good, DestPath: campDst, Mode: 0o755}); err != nil {
		t.Fatalf("Stage good: %v", err)
	}
	if err := tx.Stage(ctx, installer.StagedFile{StagedPath: badSrc, DestPath: festDst, Mode: 0o755}); err != nil {
		t.Fatalf("Stage bad: %v", err)
	}

	if _, err := tx.Commit(ctx, installer.ReceiptInfo{
		PackageID: "obedience-corp/festival", Version: "0.3.0", Channel: "stable", Source: "official-obey",
	}); err == nil {
		t.Fatal("expected commit failure")
	}
	// Journal-backed abort should restore both priors and clear the intent.
	if _, err := os.Stat(filepath.Join(home, installer.JournalName)); !os.IsNotExist(err) {
		t.Fatalf("journal should be cleared after successful abort, err=%v", err)
	}
	gotCamp, err := os.ReadFile(campDst)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotCamp) != "old-camp" {
		t.Fatalf("camp = %q, want old-camp", gotCamp)
	}
	gotFest, err := os.ReadFile(festDst)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotFest) != "old-fest" {
		t.Fatalf("fest = %q, want old-fest", gotFest)
	}
	_ = tx.Rollback(ctx)
}

// writeRawJournal hand-writes an install-intent journal file, simulating a
// process that was killed after the journal was durably written but before
// the in-process Commit()/abortActivation ever ran. Recovery in this
// scenario only happens later, via a fresh Reconcile call, which is what a
// new process's Begin() performs before starting its own activation.
func writeRawJournal(t *testing.T, home, journalID, packageID string, placed []map[string]string) {
	t.Helper()
	journal := map[string]any{
		"id":         journalID,
		"package_id": packageID,
		"version":    "1.1.0",
		"channel":    "stable",
		"source":     "official-obey",
		"placed":     placed,
	}
	raw, err := json.Marshal(journal)
	if err != nil {
		t.Fatalf("marshal journal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, installer.JournalName), raw, 0o600); err != nil {
		t.Fatalf("write journal: %v", err)
	}
}

// TestReconcile_KillDuringHubPlacementRollsBackAtomically is the test that
// justifies "hub last": camp and fest were already fully placed by this
// transaction (new bytes on disk, old bytes backed up); festival was
// journaled last but the crash lands before its backup or move ever ran.
//
// The task doc for this sequence predicted that camp and fest would keep
// their new bytes after Reconcile, with only festival reverted. That is not
// what the code does: Reconcile reverses every entry recorded in the
// journal, not only the one that was mid-flight. Verified empirically
// before writing this assertion (see the sequence's task-05 report). The
// real, load-bearing guarantee "hub last" provides is atomicity: a crash
// anywhere in suite placement, including mid-hub, always unwinds the whole
// transaction back to the prior consistent state. Camp and fest are never
// left stale next to a newer hub, and the hub is never left stale next to a
// newer camp/fest, because nothing is ever left partially newer at all.
func TestReconcile_KillDuringHubPlacementRollsBackAtomically(t *testing.T) {
	home, _ := newHomeDB(t)
	ctx := context.Background()

	binDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	campDst := filepath.Join(binDir, "camp")
	festDst := filepath.Join(binDir, "fest")
	hubDst := filepath.Join(binDir, "festival")
	if err := os.WriteFile(hubDst, []byte("old-festival"), 0o755); err != nil {
		t.Fatalf("seed festival: %v", err)
	}

	// Simulate the state a real Commit() reaches partway through: camp and
	// fest already fully activated (new bytes live, old bytes backed up).
	if err := os.WriteFile(campDst, []byte("new-camp"), 0o755); err != nil {
		t.Fatalf("place camp: %v", err)
	}
	if err := os.WriteFile(festDst, []byte("new-fest"), 0o755); err != nil {
		t.Fatalf("place fest: %v", err)
	}
	backupDir := filepath.Join(home, installer.IntentBackupsDir, "kill-hub")
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		t.Fatalf("mkdir intent-backups: %v", err)
	}
	campBackup := filepath.Join(backupDir, "backup-0")
	festBackup := filepath.Join(backupDir, "backup-1")
	hubBackup := filepath.Join(backupDir, "backup-2") // never created: hub backup/move never ran
	if err := os.WriteFile(campBackup, []byte("old-camp"), 0o755); err != nil {
		t.Fatalf("seed camp backup: %v", err)
	}
	if err := os.WriteFile(festBackup, []byte("old-fest"), 0o755); err != nil {
		t.Fatalf("seed fest backup: %v", err)
	}

	writeRawJournal(t, home, "kill-hub", "obedience-corp/festival", []map[string]string{
		{"dest": campDst, "backup": campBackup},
		{"dest": festDst, "backup": festBackup},
		{"dest": hubDst, "backup": hubBackup},
	})

	if err := installer.Reconcile(ctx, home, nil); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	gotCamp, err := os.ReadFile(campDst)
	if err != nil || string(gotCamp) != "old-camp" {
		t.Fatalf("camp = %q err=%v, want old-camp (backup-present state: removed and restored)", gotCamp, err)
	}
	gotFest, err := os.ReadFile(festDst)
	if err != nil || string(gotFest) != "old-fest" {
		t.Fatalf("fest = %q err=%v, want old-fest (backup-present state: removed and restored)", gotFest, err)
	}
	gotHub, err := os.ReadFile(hubDst)
	if err != nil || string(gotHub) != "old-festival" {
		t.Fatalf("festival = %q err=%v, want old-festival untouched (backup-missing state: not started)", gotHub, err)
	}

	if _, err := os.Stat(filepath.Join(home, installer.JournalName)); !os.IsNotExist(err) {
		t.Fatalf("journal should be cleared after reconcile, err=%v", err)
	}
	entries, err := os.ReadDir(filepath.Join(home, installer.IntentBackupsDir))
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read intent-backups: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("intent-backups should be empty after reconcile, got %v", entries)
	}
}

// TestReconcile_KillDuringFreshInstallRemovesPartialPlacements covers the
// third journal recovery state that TestReconcile_KillDuringHubPlacementRollsBackAtomically
// does not reach: a fresh install (nothing pre-existing, so Backup is empty)
// killed mid-placement. There is no pre-image to restore, so the correct
// reversal is deletion, not restoration.
func TestReconcile_KillDuringFreshInstallRemovesPartialPlacements(t *testing.T) {
	home, _ := newHomeDB(t)
	ctx := context.Background()

	binDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	campDst := filepath.Join(binDir, "camp")
	festDst := filepath.Join(binDir, "fest")
	hubDst := filepath.Join(binDir, "festival")

	// camp and fest were placed fresh (no pre-existing file, so no backup).
	if err := os.WriteFile(campDst, []byte("new-camp"), 0o755); err != nil {
		t.Fatalf("place camp: %v", err)
	}
	if err := os.WriteFile(festDst, []byte("new-fest"), 0o755); err != nil {
		t.Fatalf("place fest: %v", err)
	}
	// festival never existed and was never placed: no file, no backup.

	writeRawJournal(t, home, "kill-fresh", "obedience-corp/festival", []map[string]string{
		{"dest": campDst, "backup": ""},
		{"dest": festDst, "backup": ""},
		{"dest": hubDst, "backup": ""},
	})

	if err := installer.Reconcile(ctx, home, nil); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if _, err := os.Stat(campDst); !os.IsNotExist(err) {
		t.Fatalf("camp should be removed (fresh-install state: no backup to restore), err=%v", err)
	}
	if _, err := os.Stat(festDst); !os.IsNotExist(err) {
		t.Fatalf("fest should be removed (fresh-install state: no backup to restore), err=%v", err)
	}
	if _, err := os.Stat(hubDst); !os.IsNotExist(err) {
		t.Fatalf("festival should not exist, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, installer.JournalName)); !os.IsNotExist(err) {
		t.Fatalf("journal should be cleared after reconcile, err=%v", err)
	}
}

// TestReconcile_CompletedTransactionNotUndoneByLaterReconcile proves the
// other direction of correctness: once a receipt records the same
// install_journal_id as a stale journal, Reconcile treats the transaction as
// a completed success and clears the journal without reversing anything.
// Getting this backwards would silently roll a user back a version every
// time a leftover journal (e.g. from a clear that failed after Commit
// otherwise succeeded) was found on a later run.
func TestReconcile_CompletedTransactionNotUndoneByLaterReconcile(t *testing.T) {
	home, db := newHomeDB(t)
	ctx := context.Background()

	binDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	campDst := filepath.Join(binDir, "camp")
	if err := os.WriteFile(campDst, []byte("new-camp"), 0o755); err != nil {
		t.Fatalf("place camp: %v", err)
	}
	campBackupDir := filepath.Join(home, installer.IntentBackupsDir, "completed-txn")
	if err := os.MkdirAll(campBackupDir, 0o700); err != nil {
		t.Fatalf("mkdir intent-backups: %v", err)
	}
	campBackup := filepath.Join(campBackupDir, "backup-0")
	if err := os.WriteFile(campBackup, []byte("old-camp"), 0o755); err != nil {
		t.Fatalf("seed camp backup: %v", err)
	}

	rec := receipts.Receipt{
		PackageID:   "obedience-corp/festival",
		Version:     "1.1.0",
		Source:      "official-obey",
		Channel:     "stable",
		InstalledAt: time.Now().UTC(),
		OwnedFiles: []receipts.OwnedFile{
			{Path: campDst, Hash: "deadbeef", Mode: 0o755},
		},
		Metadata: map[string]string{"install_journal_id": "completed-txn"},
	}
	if err := receipts.Write(ctx, db.Raw(), rec); err != nil {
		t.Fatalf("write receipt: %v", err)
	}

	// A stale journal survives even though Commit() actually succeeded (its
	// best-effort clearJournal call failed or never ran).
	writeRawJournal(t, home, "completed-txn", "obedience-corp/festival", []map[string]string{
		{"dest": campDst, "backup": campBackup},
	})

	if err := installer.Reconcile(ctx, home, db.Raw()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	got, err := os.ReadFile(campDst)
	if err != nil || string(got) != "new-camp" {
		t.Fatalf("camp = %q err=%v, want new-camp: a completed transaction must not be reversed", got, err)
	}
	if _, err := os.Stat(filepath.Join(home, installer.JournalName)); !os.IsNotExist(err) {
		t.Fatalf("journal should still be cleared, err=%v", err)
	}
}
