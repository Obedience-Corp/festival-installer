//go:build windows && container_fs

package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/shellsetup"
)

func TestWindowsDamagedExecutablesStayUnavailable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pwsh.exe")
	if err := os.WriteFile(path, make([]byte, 256), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := shellTarget(path); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("damaged login shell was accepted: %v", err)
	}
	t.Run("healthy edition survives", func(t *testing.T) {
		t.Setenv("SHELL", "")
		t.Setenv("PATH", filepath.Dir(path))
		targets, err := integrationShells(context.Background(), "")
		if err != nil || len(targets) != 2 || targets[0].Name != "pwsh" || targets[0].Unavailable == nil || targets[1].Name != "powershell" || targets[1].Unavailable != nil {
			t.Fatalf("damaged pwsh hid Windows PowerShell: %+v %v", targets, err)
		}
	})
	managed := filepath.Join(t.TempDir(), "festival-home")
	t.Setenv("FESTIVAL_HOME", managed)
	t.Setenv("OBEY_INSTALLER_HOME", "")
	t.Setenv("SHELL", path)
	report, err := ReconcileIntegrations(context.Background(), IntegrationOptions{})
	if err != nil || report.Ready || len(report.Checks) == 0 || report.Checks[0].ID != "shell" || report.Checks[0].Status != "unavailable" {
		t.Fatalf("damaged shell was not reported unavailable: %+v %v", report, err)
	}
	if _, err := os.Stat(filepath.Join(managed, "shell")); !os.IsNotExist(err) {
		t.Fatalf("damaged shell generated helpers: %v", err)
	}
	if version, _, _ := probeBinary(context.Background(), path, "camp"); version != "" {
		t.Fatalf("damaged suite executable was probed: %q", version)
	}
	if _, err := shellsetup.Run(context.Background(), path, "shell-init", "powershell"); err == nil {
		t.Fatal("damaged tool reached the launcher")
	}
}
