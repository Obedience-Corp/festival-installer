package installer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Obedience-Corp/festival-installer/internal/state"
	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
)

func TestBeginPrunesCompletedWindowsImageBackup(t *testing.T) {
	home := t.TempDir()
	ctx := context.Background()
	db, err := state.OpenDB(ctx, home)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(context.Background()) })
	root := filepath.Join(home, IntentBackupsDir)
	stale := filepath.Join(root, strings.Repeat("a", 32))
	userDir := filepath.Join(root, "keep-user-data")
	for _, dir := range []string{stale, userDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "backup-0"), []byte("old image"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := Begin(ctx, db.Raw(), home)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("completed transaction backup remains: %v", err)
	}
	if _, err := os.Stat(filepath.Join(userDir, "backup-0")); err != nil {
		t.Fatalf("unrelated directory was removed: %v", err)
	}
}

func TestReconcile_ReversesPartialPlacement(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	// Pre-existing file that was "backed up" then replaced mid-install.
	dest := filepath.Join(bin, "camp")
	if err := os.WriteFile(dest, []byte("NEW"), 0o755); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(home, "backup-camp")
	if err := os.WriteFile(backup, []byte("OLD"), 0o755); err != nil {
		t.Fatal(err)
	}
	j := Journal{
		ID:        "journal-1",
		PackageID: "obedience-corp/festival",
		Version:   "9.9.9",
		Placed:    []JournalPlace{{Dest: dest, Backup: backup}},
	}
	if err := writeJournal(home, j); err != nil {
		t.Fatal(err)
	}

	if err := Reconcile(context.Background(), home, nil); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "OLD" {
		t.Fatalf("want restored OLD, got %q", got)
	}
	if _, err := os.Stat(journalPath(home)); !os.IsNotExist(err) {
		t.Fatal("journal should be cleared")
	}
}

func TestReconcile_NoJournal(t *testing.T) {
	home := t.TempDir()
	if err := Reconcile(context.Background(), home, nil); err != nil {
		t.Fatal(err)
	}
}

func TestReconcile_DoesNotTrustOlderReceipt(t *testing.T) {
	home := t.TempDir()
	db, err := state.OpenDB(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close(context.Background()) }()

	dest := filepath.Join(home, "bin", "camp")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("NEW"), 0o755); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(home, "backup-camp")
	if err := os.WriteFile(backup, []byte("OLD"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := receipts.Write(context.Background(), db.Raw(), receipts.Receipt{
		PackageID:   "obedience-corp/festival",
		Version:     "1.0.0",
		Source:      "official-obey",
		Channel:     "stable",
		InstalledAt: time.Now().UTC(),
		OwnedFiles:  []receipts.OwnedFile{{Path: dest}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := writeJournal(home, Journal{
		ID:        "new-transaction",
		PackageID: "obedience-corp/festival",
		Version:   "2.0.0",
		Placed:    []JournalPlace{{Dest: dest, Backup: backup}},
	}); err != nil {
		t.Fatal(err)
	}

	if err := Reconcile(context.Background(), home, db.Raw()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "OLD" {
		t.Fatalf("want older binary restored, got %q", got)
	}
}

func TestReconcile_RetainsJournalWhenRollbackFails(t *testing.T) {
	home := t.TempDir()
	dest := filepath.Join(home, "non-empty")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "child"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeJournal(home, Journal{Placed: []JournalPlace{{Dest: dest}}}); err != nil {
		t.Fatal(err)
	}

	if err := Reconcile(context.Background(), home, nil); err == nil {
		t.Fatal("expected rollback failure")
	}
	if _, err := os.Stat(journalPath(home)); err != nil {
		t.Fatalf("journal should remain for retry: %v", err)
	}
}

func TestReconcile_PreBackupWindowLeavesLiveBinary(t *testing.T) {
	// Crash after journaling Dest+Backup but before backupExisting renames the
	// live binary. Reconcile must not delete the still-live dest.
	home := t.TempDir()
	dest := filepath.Join(home, "bin", "camp")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("LIVE"), 0o755); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(home, IntentBackupsDir, "jid", "backup-0")
	// backup path is planned but the file was never created.
	if err := writeJournal(home, Journal{
		ID:        "jid",
		PackageID: "obedience-corp/festival",
		Version:   "2.0.0",
		Placed:    []JournalPlace{{Dest: dest, Backup: backup}},
	}); err != nil {
		t.Fatal(err)
	}

	if err := Reconcile(context.Background(), home, nil); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("live binary should remain: %v", err)
	}
	if string(got) != "LIVE" {
		t.Fatalf("dest = %q, want LIVE (pre-backup window must be a no-op)", got)
	}
	if _, err := os.Stat(journalPath(home)); !os.IsNotExist(err) {
		t.Fatal("journal should be cleared after no-op reverse")
	}
}

func TestReconcile_PostBackupPreMoveRestores(t *testing.T) {
	// Crash after backupExisting (dest gone, backup holds OLD) before AtomicMove.
	home := t.TempDir()
	dest := filepath.Join(home, "bin", "camp")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(home, IntentBackupsDir, "jid", "backup-0")
	if err := os.MkdirAll(filepath.Dir(backup), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backup, []byte("OLD"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJournal(home, Journal{
		ID:     "jid",
		Placed: []JournalPlace{{Dest: dest, Backup: backup}},
	}); err != nil {
		t.Fatal(err)
	}

	if err := Reconcile(context.Background(), home, nil); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "OLD" {
		t.Fatalf("want restored OLD, got %q", got)
	}
}

func TestReconcile_MatchingReceiptClearsWithoutReverse(t *testing.T) {
	home := t.TempDir()
	db, err := state.OpenDB(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close(context.Background()) }()

	dest := filepath.Join(home, "bin", "camp")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("NEW"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := receipts.Write(context.Background(), db.Raw(), receipts.Receipt{
		PackageID:   "obedience-corp/festival",
		Version:     "2.0.0",
		Source:      "official-obey",
		Channel:     "stable",
		InstalledAt: time.Now().UTC(),
		OwnedFiles:  []receipts.OwnedFile{{Path: dest}},
		Metadata:    map[string]string{"install_journal_id": "same-id"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := writeJournal(home, Journal{
		ID:        "same-id",
		PackageID: "obedience-corp/festival",
		Version:   "2.0.0",
		Placed:    []JournalPlace{{Dest: dest}},
	}); err != nil {
		t.Fatal(err)
	}

	if err := Reconcile(context.Background(), home, db.Raw()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "NEW" {
		t.Fatalf("matching receipt must not reverse; got %q", got)
	}
	if _, err := os.Stat(journalPath(home)); !os.IsNotExist(err) {
		t.Fatal("journal should be cleared")
	}
}

func TestReconcile_MissingBackupDoesNotDeleteLive(t *testing.T) {
	// Explicit guard: backup path set, unreadable parent (lost staging), dest live.
	home := t.TempDir()
	dest := filepath.Join(home, "bin", "camp")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("LIVE"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Point at a backup under a path that does not exist: Lstat fails with NotExist.
	backup := filepath.Join(home, "intent-backups", "gone", "backup-0")
	if err := writeJournal(home, Journal{
		ID:     "jid",
		Placed: []JournalPlace{{Dest: dest, Backup: backup}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := Reconcile(context.Background(), home, nil); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "LIVE" {
		t.Fatalf("got %q, want LIVE", got)
	}
}
