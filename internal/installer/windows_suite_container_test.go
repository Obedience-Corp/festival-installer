//go:build windows && container_fs

package installer_test

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

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

// Run a copy of this test binary as managed festival.exe while its parent
// replaces all three tools. The old process must finish after the rename, and
// the receipt and files must report the new version.
func TestWindowsRunningExecutableReplacedAndOldProcessCompletes(t *testing.T) {
	if ready := os.Getenv("FESTIVAL_SELF_LOCK_READY"); ready != "" {
		if err := os.WriteFile(ready, []byte("ready"), 0o644); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(25 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(os.Getenv("FESTIVAL_SELF_LOCK_RELEASE")); err == nil {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("parent did not release managed executable")
	}

	home, db := newHomeDB(t)
	ctx := context.Background()
	binDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"camp.exe", "fest.exe"} {
		if err := os.WriteFile(filepath.Join(binDir, name), []byte("old-"+name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	running := filepath.Join(binDir, "festival.exe")
	src, err := os.Open(self)
	if err != nil {
		t.Fatal(err)
	}
	dst, err := os.Create(running)
	if err != nil {
		_ = src.Close()
		t.Fatal(err)
	}
	_, copyErr := io.Copy(dst, src)
	closeDstErr := dst.Close()
	_ = src.Close()
	if copyErr != nil || closeDstErr != nil {
		t.Fatalf("copy executable: %v, close: %v", copyErr, closeDstErr)
	}
	ready := filepath.Join(home, "child-ready")
	release := filepath.Join(home, "child-release")
	child := exec.Command(running, "-test.run=^TestWindowsRunningExecutableReplacedAndOldProcessCompletes$", "-test.count=1")
	child.Env = append(os.Environ(), "FESTIVAL_SELF_LOCK_READY="+ready, "FESTIVAL_SELF_LOCK_RELEASE="+release)
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Process.Kill(); _ = child.Wait() })
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("managed festival.exe child never became ready")
		}
		time.Sleep(20 * time.Millisecond)
	}
	oldReceipt := receipts.Receipt{PackageID: "obedience-corp/festival", Version: "1.0.0", Channel: "stable", Source: "official-obey"}
	if err := receipts.Write(ctx, db.Raw(), oldReceipt); err != nil {
		t.Fatal(err)
	}
	tx, err := installer.Begin(ctx, db.Raw(), home)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"camp.exe", "fest.exe", "festival.exe"} {
		staged := stageBlob(t, tx.StagingDir(), name, "new-"+name)
		if err := tx.Stage(ctx, installer.StagedFile{StagedPath: staged, DestPath: filepath.Join(binDir, name), Mode: 0o755}); err != nil {
			t.Fatal(err)
		}
	}
	rec, err := tx.Commit(ctx, installer.ReceiptInfo{PackageID: oldReceipt.PackageID, Version: "2.0.0", Channel: "stable", Source: "official-obey"})
	if err != nil {
		t.Fatalf("replace running festival.exe: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if rec.Version != "2.0.0" || len(rec.OwnedFiles) != 3 {
		t.Fatalf("upgraded receipt incomplete: %+v", rec)
	}
	if err := os.WriteFile(release, []byte("done"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := child.Wait(); err != nil {
		t.Fatalf("managed child did not exit cleanly: %v", err)
	}
	for _, name := range []string{"camp.exe", "fest.exe", "festival.exe"} {
		want := []byte("new-" + name)
		got, err := os.ReadFile(filepath.Join(binDir, name))
		if err != nil || string(got) != string(want) {
			t.Fatalf("%s after upgrade = %q, %v", name, got, err)
		}
	}
	gotReceipt, err := receipts.Get(ctx, db.Raw(), oldReceipt.PackageID)
	if err != nil || gotReceipt.Version != "2.0.0" || len(gotReceipt.OwnedFiles) != 3 {
		t.Fatalf("receipt did not upgrade: %+v, %v", gotReceipt, err)
	}
	if entries, err := os.ReadDir(filepath.Join(home, installer.IntentBackupsDir)); err == nil && len(entries) != 0 {
		t.Fatalf("old running executable left backup files after child exited: %v", entries)
	}
}
