package installer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Obedience-Corp/obey-installer/internal/state"
	"github.com/Obedience-Corp/obey-installer/internal/state/receipts"
)

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
