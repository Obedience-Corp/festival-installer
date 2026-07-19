package installer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
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
