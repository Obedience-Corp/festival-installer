package cli_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
)

func doctorChecks(t *testing.T, out string) map[string]string {
	t.Helper()
	var data struct {
		Checks []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"checks"`
	}
	dataOf(t, out, &data)
	status := map[string]string{}
	for _, c := range data.Checks {
		status[c.ID] = c.Status
	}
	return status
}

func TestDoctor_BrokenPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	t.Setenv("PATH", t.TempDir())

	out, _, _ := runInstaller(t, "doctor", "--json")
	status := doctorChecks(t, out)
	if status["managed_bin_on_path"] != "fail" {
		t.Fatalf("expected managed_bin_on_path fail, got %q", status["managed_bin_on_path"])
	}
}

func TestDoctor_PathOk(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	binDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Setenv("PATH", binDir)

	out, _, _ := runInstaller(t, "doctor", "--json")
	status := doctorChecks(t, out)
	if status["managed_bin_on_path"] != "ok" {
		t.Fatalf("expected managed_bin_on_path ok, got %q", status["managed_bin_on_path"])
	}
}

func TestDoctor_OrphanReceipt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	ctx := context.Background()
	binDir := filepath.Join(home, "bin")
	t.Setenv("PATH", binDir)

	rec := receipts.Receipt{
		PackageID:   festivalPackageIDForTest,
		Version:     "0.2.10",
		Source:      "official-obey",
		Channel:     "stable",
		InstalledAt: time.Now().UTC(),
		OwnedFiles: []receipts.OwnedFile{
			{Path: filepath.Join(binDir, "ghost"), Hash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", Mode: 0o755},
		},
		Metadata: map[string]string{},
	}
	if err := receipts.Write(ctx, mustDB(t, ctx, home), rec); err != nil {
		t.Fatalf("write receipt: %v", err)
	}

	out, _, _ := runInstaller(t, "doctor", "--json")
	status := doctorChecks(t, out)
	if status["receipts_integrity"] != "fail" {
		t.Fatalf("expected receipts_integrity fail for orphan, got %q", status["receipts_integrity"])
	}
}

func TestDoctor_PathShadowing_Festival(t *testing.T) {
	home := t.TempDir()
	managedBin := filepath.Join(home, "bin")
	fakeBinary(t, managedBin, "festival")
	otherDir := t.TempDir()
	fakeBinary(t, otherDir, "festival")

	t.Setenv("OBEY_INSTALLER_HOME", home)
	t.Setenv("PATH", otherDir+string(os.PathListSeparator)+managedBin)

	out, _, _ := runInstaller(t, "doctor", "--json")
	status := doctorChecks(t, out)
	if status["path_shadowing"] != "warn" {
		t.Fatalf("expected path_shadowing warn for a shadowed festival, got %q", status["path_shadowing"])
	}
}

func TestDoctor_JSONShapeStable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	t.Setenv("PATH", filepath.Join(home, "bin"))

	out, _, _ := runInstaller(t, "doctor", "--json")
	var env struct {
		OK     bool `json:"ok"`
		Action string
		Data   struct {
			Checks []struct {
				ID, Status, Message string
			} `json:"checks"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	if !env.OK || env.Action != "doctor" {
		t.Fatalf("unexpected doctor envelope: %+v", env)
	}
	wantIDs := map[string]bool{"managed_bin_on_path": true, "sources_reachable": true, "marketplace_trust": true, "receipts_integrity": true, "path_shadowing": true}
	for _, c := range env.Data.Checks {
		delete(wantIDs, c.ID)
		if c.Status == "" || c.ID == "" {
			t.Fatalf("incomplete check: %+v", c)
		}
	}
	if len(wantIDs) != 0 {
		t.Fatalf("missing doctor checks: %v", wantIDs)
	}
}
