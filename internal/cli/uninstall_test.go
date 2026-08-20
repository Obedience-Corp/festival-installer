package cli_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
)

func TestUninstall_RemovesOwnedFilesOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	ctx := context.Background()
	binDir := filepath.Join(home, "bin")

	writeManagedBinary(t, binDir, "camp", "0.2.10")
	writeManagedBinary(t, binDir, "fest", "0.2.10")
	writeFestivalReceipt(t, ctx, home, "0.2.10", "official-obey", binDir)

	unrelated := filepath.Join(binDir, "other")
	if err := os.WriteFile(unrelated, []byte("keep"), 0o755); err != nil {
		t.Fatalf("write unrelated: %v", err)
	}
	outside := filepath.Join(home, "outside")
	if err := os.WriteFile(outside, []byte("keep"), 0o644); err != nil {
		t.Fatalf("write outside: %v", err)
	}

	out, _, err := runInstaller(t, "uninstall", "festival", "--json")
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	var res struct {
		Removed []string `json:"removed"`
	}
	dataOf(t, out, &res)
	if len(res.Removed) != 2 {
		t.Fatalf("expected 2 removed, got %v", res.Removed)
	}

	if _, err := os.Stat(filepath.Join(binDir, "camp")); !os.IsNotExist(err) {
		t.Fatalf("camp should be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(binDir, "fest")); !os.IsNotExist(err) {
		t.Fatalf("fest should be removed, err=%v", err)
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Fatalf("unrelated managed-bin file should remain: %v", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside file should remain: %v", err)
	}
	if _, err := receipts.Get(ctx, mustDB(t, ctx, home), festivalPackageIDForTest); !errors.Is(err, receipts.ErrNotFound) {
		t.Fatalf("receipt should be deleted, got %v", err)
	}
}

func TestUninstall_RefusesToRemoveRunningHub(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	ctx := context.Background()
	binDir := filepath.Join(home, "bin")

	writeManagedBinary(t, binDir, "camp", "0.2.10")
	writeManagedBinary(t, binDir, "fest", "0.2.10")
	symlinkSelfAsManagedFestival(t, home)
	writeFestivalReceiptWithHub(t, ctx, home, "0.2.10", "official-obey", binDir)

	_, errOut, err := runInstaller(t, "uninstall", "festival")
	if err == nil {
		t.Fatal("expected uninstall to refuse removing the running hub")
	}
	if !hasErrorCode(err, "E_UNINSTALL_SELF") {
		t.Fatalf("expected E_UNINSTALL_SELF, got err=%v errOut=%s", err, errOut)
	}

	for _, name := range []string{"camp", "fest", "festival"} {
		if _, statErr := os.Stat(filepath.Join(binDir, name)); statErr != nil {
			t.Fatalf("%s should still exist after a refused uninstall: %v", name, statErr)
		}
	}
	if _, err := receipts.Get(ctx, mustDB(t, ctx, home), festivalPackageIDForTest); err != nil {
		t.Fatalf("receipt should still exist after a refused uninstall: %v", err)
	}
}

func TestUninstall_RefusesWhenSelfPlacementUnknown(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses directory permission checks")
	}
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	binDir := filepath.Join(home, "bin")

	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(binDir, 0o755); err != nil {
			t.Fatalf("restore bin dir mode: %v", err)
		}
	})
	// Strip search permission on the managed bin dir so ResolveSelf cannot
	// resolve binDir/festival and reports SelfUnknown instead of SelfManaged
	// or SelfExternal.
	if err := os.Chmod(binDir, 0o000); err != nil {
		t.Fatalf("chmod bin dir: %v", err)
	}

	_, errOut, err := runInstaller(t, "uninstall", "festival")
	if err == nil {
		t.Fatal("expected uninstall to refuse when self placement cannot be resolved")
	}
	if !hasErrorCode(err, "E_UNINSTALL_SELF") {
		t.Fatalf("expected E_UNINSTALL_SELF, got err=%v errOut=%s", err, errOut)
	}
}

func TestUninstall_NoReceiptIsNoOp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)

	out, _, err := runInstaller(t, "uninstall", "festival")
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if !strings.Contains(out, "nothing to uninstall") {
		t.Fatalf("expected nothing-to-uninstall note, got %q", out)
	}
}
