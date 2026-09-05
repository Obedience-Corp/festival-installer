package cli_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
)

// doctorChecks reads the checks out of a doctor envelope without asserting on
// the ok field, because doctor now emits its checks on failing runs too. Tests
// that care about success or failure assert on the returned error instead.
func doctorChecks(t *testing.T, out string) map[string]string {
	t.Helper()
	var env struct {
		Data struct {
			Checks []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"checks"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("decode envelope: %v\n%s", err, out)
	}
	status := map[string]string{}
	for _, c := range env.Data.Checks {
		status[c.ID] = c.Status
	}
	return status
}

func TestDoctor_FreshHomeIsPendingSetupAndExitsZero(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	t.Setenv("PATH", t.TempDir())

	out, errOut, err := runInstaller(t, "doctor", "--json")
	if err != nil {
		t.Fatalf("a fresh home must not fail its own health check: %v\n%s", err, errOut)
	}
	status := doctorChecks(t, out)
	if status["managed_bin_on_path"] != "pending" {
		t.Fatalf("expected managed_bin_on_path pending, got %q", status["managed_bin_on_path"])
	}
	if !strings.Contains(out, "festival shell-init") {
		t.Fatalf("pending message must name the command that fixes it:\n%s", out)
	}
}

// TestDoctor_PostSetupBrokenPathStillFails is the leak test for the pending
// grading. The home here has a receipt, so it is past its first run, and the
// same missing PATH entry that reads as pending above must still fail and still
// exit nonzero. Agents depend on that exit code. If someone later simplifies
// the grading to "pending whenever nothing is on PATH", this test fails.
func TestDoctor_PostSetupBrokenPathStillFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	t.Setenv("PATH", t.TempDir())
	writeCleanReceipt(t, home)

	out, _, err := runInstaller(t, "doctor", "--json")
	if err == nil {
		t.Fatalf("a set-up home with no suite on PATH must still fail:\n%s", out)
	}
	status := doctorChecks(t, out)
	if status["managed_bin_on_path"] != "fail" {
		t.Fatalf("expected managed_bin_on_path fail after setup, got %q", status["managed_bin_on_path"])
	}
}

// TestDoctor_JSONFailureEnvelopeCarriesChecks pins the fix for an envelope that
// used to print "ok": true on a run that exited 1.
func TestDoctor_JSONFailureEnvelopeCarriesChecks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	t.Setenv("PATH", t.TempDir())
	writeCleanReceipt(t, home)

	out, _, err := runInstaller(t, "doctor", "--json")
	if err == nil {
		t.Fatal("expected a failing doctor run")
	}
	var env struct {
		OK    bool `json:"ok"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
		Data struct {
			Checks []struct {
				ID, Status string
			} `json:"checks"`
		} `json:"data"`
	}
	if derr := json.Unmarshal([]byte(out), &env); derr != nil {
		t.Fatalf("decode: %v\n%s", derr, out)
	}
	if env.OK {
		t.Fatalf("a nonzero doctor run must not report ok:true\n%s", out)
	}
	if env.Error == nil || env.Error.Code != "E_DOCTOR_FAIL" {
		t.Fatalf("failure envelope lost its error code:\n%s", out)
	}
	if len(env.Data.Checks) == 0 {
		t.Fatalf("failure envelope must still carry the checks:\n%s", out)
	}
	if strings.Count(out, `"schema_version"`) != 1 {
		t.Fatalf("expected exactly one envelope:\n%s", out)
	}
}

// writeCleanReceipt records an installed package owning no files, which makes
// the home past its first run without giving receipts_integrity anything to
// complain about.
func writeCleanReceipt(t *testing.T, home string) {
	t.Helper()
	ctx := context.Background()
	rec := receipts.Receipt{
		PackageID:   festivalPackageIDForTest,
		Version:     "0.2.10",
		Source:      "official-obey",
		Channel:     "stable",
		InstalledAt: time.Now().UTC(),
		Metadata:    map[string]string{},
	}
	if err := receipts.Write(ctx, mustDB(t, ctx, home), rec); err != nil {
		t.Fatalf("write receipt: %v", err)
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
	fakeBinary(t, binDir, "camp")
	fakeBinary(t, binDir, "fest")
	fakeBinary(t, binDir, "festival")

	out, _, _ := runInstaller(t, "doctor", "--json")
	status := doctorChecks(t, out)
	if status["managed_bin_on_path"] != "ok" {
		t.Fatalf("expected managed_bin_on_path ok, got %q", status["managed_bin_on_path"])
	}
}

func TestDoctor_PackagePrefixOk(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	root := t.TempDir()
	bin := filepath.Join(root, "usr", "bin")
	shell := filepath.Join(root, "usr", "share", "festival", "shell")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(shell, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeBinary(t, bin, "camp")
	fakeBinary(t, bin, "fest")
	fakeBinary(t, bin, "festival")
	if err := os.WriteFile(filepath.Join(shell, "festival.zsh"), []byte("# helper\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	out, errOut, err := runInstaller(t, "doctor", "--json")
	status := doctorChecks(t, out)
	if status["managed_bin_on_path"] != "ok" {
		t.Fatalf("expected managed_bin_on_path ok on package prefix, got %q\n%s\n%s", status["managed_bin_on_path"], out, errOut)
	}
	if err != nil {
		t.Fatalf("doctor err %v (marketplace warn is not fail)\n%s", err, errOut)
	}
	if _, err := os.Stat(filepath.Join(home, "state.db")); !os.IsNotExist(err) {
		t.Fatal("package doctor must not create state.db")
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

// TestDoctor_JSONShapeStable pins the success envelope. PATH points at an
// unrelated directory rather than at the (nonexistent) managed bin dir, so this
// is a genuine first run that grades pending and exits zero. Before pending
// grading existed, the same run exited 1 while still printing "ok": true, and
// this test passed on that lie.
func TestDoctor_JSONShapeStable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	t.Setenv("PATH", t.TempDir())

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
