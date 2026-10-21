//go:build container_fs

package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUninstallObeyRootPreservesReceiptBinaryAndHooks(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires a root container")
	}
	f := obeyServiceFixture(t, "0.2.0")
	if _, stderr, err := runInstaller(t, "install", "obey", "--allow-unverified", "--json"); err != nil {
		t.Fatalf("install: %v %s", err, stderr)
	}
	bin := filepath.Join(f.home, "bin", "obey")
	marker := filepath.Join(t.TempDir(), "cleanup-called")
	original := "#!/bin/sh\nprintf invoked > '" + marker + "'\n"
	writeFile(t, bin, original)
	if err := os.Chmod(bin, 0755); err != nil {
		t.Fatal(err)
	}
	out, stderr, err := runInstaller(t, "uninstall", "obey", "--json")
	if err == nil || !strings.Contains(out+stderr, "E_UNINSTALL_ROOT") || !strings.Contains(out+stderr, "without sudo") {
		t.Fatalf("root uninstall: %v %s %s", err, out, stderr)
	}
	if got, err := os.ReadFile(bin); err != nil || string(got) != original {
		t.Fatalf("root changed binary: %q %v", got, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("root invoked cleanup: %v", err)
	}
	out, stderr, err = runInstaller(t, "list", "--json")
	if err != nil || !strings.Contains(out, "obey") {
		t.Fatalf("receipt not retained: %v %s %s", err, out, stderr)
	}
}
