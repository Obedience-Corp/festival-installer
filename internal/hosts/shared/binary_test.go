package shared_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/Obedience-Corp/obey-installer/internal/hosts/shared"
	"github.com/Obedience-Corp/obey-installer/internal/state"
	"github.com/Obedience-Corp/obey-installer/internal/state/receipts"
)

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func stage(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("stage: %v", err)
	}
	return p
}

func TestActivateBinary_EmptyNameRejected(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	staged := stage(t, t.TempDir(), "blob", "x")
	if _, err := shared.ActivateBinary(context.Background(), staged, ""); err != shared.ErrEmptyName {
		t.Fatalf("expected ErrEmptyName, got %v", err)
	}
}

func TestActivateBinary_SlashNameRejected(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	staged := stage(t, t.TempDir(), "blob", "x")
	if _, err := shared.ActivateBinary(context.Background(), staged, "../evil"); err != shared.ErrEmptyName {
		t.Fatalf("expected ErrEmptyName for slash name, got %v", err)
	}
}

func TestActivateBinary_LandsExecutableWithHash(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	ctx := context.Background()

	body := "camp-binary-bytes"
	staged := stage(t, t.TempDir(), "camp", body)

	rec, err := shared.ActivateBinary(ctx, staged, "camp")
	if err != nil {
		t.Fatalf("ActivateBinary: %v", err)
	}

	binDir, err := state.BinDir(ctx)
	if err != nil {
		t.Fatalf("BinDir: %v", err)
	}
	want := filepath.Join(binDir, "camp")
	if rec.Path != want {
		t.Fatalf("path: got %s want %s", rec.Path, want)
	}
	if rec.Hash != sha256Hex([]byte(body)) {
		t.Fatalf("hash mismatch: %s", rec.Hash)
	}

	fi, err := os.Stat(want)
	if err != nil {
		t.Fatalf("stat activated: %v", err)
	}
	if fi.Mode().Perm() != 0o755 {
		t.Fatalf("mode: got %o want 0755", fi.Mode().Perm())
	}
}

func TestRemoveByRecord_DeletesOnlyThatFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	ctx := context.Background()

	campRec, err := shared.ActivateBinary(ctx, stage(t, t.TempDir(), "camp", "camp"), "camp")
	if err != nil {
		t.Fatalf("activate camp: %v", err)
	}
	if _, err := shared.ActivateBinary(ctx, stage(t, t.TempDir(), "fest", "fest"), "fest"); err != nil {
		t.Fatalf("activate fest: %v", err)
	}

	if err := shared.RemoveByRecord(ctx, campRec); err != nil {
		t.Fatalf("RemoveByRecord: %v", err)
	}

	binDir, _ := state.BinDir(ctx)
	if _, err := os.Stat(filepath.Join(binDir, "camp")); !os.IsNotExist(err) {
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
