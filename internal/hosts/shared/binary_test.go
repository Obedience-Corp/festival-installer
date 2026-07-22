package shared_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Obedience-Corp/obey-installer/internal/hosts/shared"
	"github.com/Obedience-Corp/obey-installer/internal/state"
	"github.com/Obedience-Corp/obey-installer/internal/state/receipts"
)

func TestRemoveByRecord_DeletesOnlyThatFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	ctx := context.Background()

	binDir, err := state.BinDir(ctx)
	if err != nil {
		t.Fatalf("BinDir: %v", err)
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir binDir: %v", err)
	}
	campPath := filepath.Join(binDir, "camp")
	if err := os.WriteFile(campPath, []byte("camp"), 0o755); err != nil {
		t.Fatalf("write camp: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "fest"), []byte("fest"), 0o755); err != nil {
		t.Fatalf("write fest: %v", err)
	}

	if err := shared.RemoveByRecord(ctx, receipts.OwnedFile{Path: campPath}); err != nil {
		t.Fatalf("RemoveByRecord: %v", err)
	}

	if _, err := os.Stat(campPath); !os.IsNotExist(err) {
		t.Fatalf("camp should be removed, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(binDir, "fest")); err != nil {
		t.Fatalf("fest should remain: %v", err)
	}
}

func TestRemoveByRecord_MissingIsNoError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	binDir, _ := state.BinDir(context.Background())
	if err := shared.RemoveByRecord(context.Background(), receipts.OwnedFile{Path: filepath.Join(binDir, "ghost")}); err != nil {
		t.Fatalf("removing missing file should be no-op, got %v", err)
	}
}
