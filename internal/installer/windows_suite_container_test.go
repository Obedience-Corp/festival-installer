//go:build windows && container_fs

package installer_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/Obedience-Corp/festival-installer/internal/installer"
	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
)

// This runs only inside an isolated Windows container. The first two names are
// deliberately placed before festival.exe, matching the suite activation order.
func TestWindowsSuiteUpgradeRollbackOnLockedExecutable(t *testing.T) {
	home, db := newHomeDB(t)
	ctx := context.Background()
	names := []string{"camp.exe", "fest.exe", "festival.exe"}
	binDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(binDir, name), []byte("old-"+name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	oldReceipt := receipts.Receipt{PackageID: "obedience-corp/festival", Version: "1.0.0", Channel: "stable", Source: "official-obey"}
	if err := receipts.Write(ctx, db.Raw(), oldReceipt); err != nil {
		t.Fatal(err)
	}

	locked := filepath.Join(binDir, "festival.exe")
	ptr, err := windows.UTF16PtrFromString(locked)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(ptr, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = windows.CloseHandle(handle) }()

	tx, err := installer.Begin(ctx, db.Raw(), home)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		src := stageBlob(t, tx.StagingDir(), name, "new-"+name)
		if err := tx.Stage(ctx, installer.StagedFile{StagedPath: src, DestPath: filepath.Join(binDir, name), Mode: 0o755}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.Commit(ctx, installer.ReceiptInfo{PackageID: oldReceipt.PackageID, Version: "2.0.0", Channel: "stable", Source: "official-obey"}); err == nil {
		t.Fatal("locked festival.exe upgrade claimed success")
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if err := windows.CloseHandle(handle); err != nil {
		t.Fatalf("close locked festival.exe: %v", err)
	}
	for _, name := range names {
		got, err := os.ReadFile(filepath.Join(binDir, name))
		if err != nil || string(got) != "old-"+name {
			t.Fatalf("%s after failed upgrade = %q, %v", name, got, err)
		}
	}
	gotReceipt, err := receipts.Get(ctx, db.Raw(), oldReceipt.PackageID)
	if err != nil || gotReceipt.Version != oldReceipt.Version {
		t.Fatalf("old receipt changed: %+v, %v", gotReceipt, err)
	}
	if _, err := os.Stat(filepath.Join(home, installer.JournalName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale install journal after successful rollback: %v", err)
	}
}
