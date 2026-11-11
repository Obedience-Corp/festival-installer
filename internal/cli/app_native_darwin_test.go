//go:build app_native && darwin

package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Obedience-Corp/festival-installer/internal/apptest"
)

const nativeBundle = "/Applications/Festival App.app"

// appNativeRunning returns the pids whose command is under the bundle's MacOS dir.
func appNativeRunning(t *testing.T, exact bool) []int {
	t.Helper()
	out, err := exec.Command("ps", "-axo", "pid=,comm=").Output()
	if err != nil {
		t.Fatalf("ps: %v", err)
	}
	var pids []int
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.SplitN(strings.TrimSpace(line), " ", 2)
		if len(fields) != 2 {
			continue
		}
		cmd := strings.TrimSpace(fields[1])
		match := strings.HasPrefix(cmd, nativeBundle+"/Contents/MacOS/")
		if exact {
			match = cmd == nativeBundle+"/Contents/MacOS/festival-app"
		}
		if pid, err := strconv.Atoi(fields[0]); err == nil && match {
			pids = append(pids, pid)
		}
	}
	return pids
}

func TestAppNativeMacOSInstallStatusAdoptUninstall(t *testing.T) {
	apptest.RequireHostedRunner(t)
	asset := appNativeAssetFor(t, "app-bundle")
	key := appNativeKey(t)
	good, bad := t.TempDir(), t.TempDir()
	appNativeMarketplace(t, key, good, []map[string]any{appNativeArtifact(t, asset, asset)})
	appNativeMarketplace(t, key, bad, []map[string]any{appNativeArtifact(t, asset, appNativeOtherAsset(t, asset))})

	appNativeStep(t, "refuses_mismatched_updater_signature")
	appNativeHome(t, bad)
	appNativeFail(t, "refuses_mismatched_updater_signature", "E_APP_SIGNATURE", appNativeJSON(t, "install", "app", "--json"))
	if _, err := os.Lstat(nativeBundle); !os.IsNotExist(err) {
		t.Fatalf("a bundle was placed despite the bad signature: %v", err)
	}
	if s := appNativeStatusOf(t, "refuses_mismatched_updater_signature"); s.App.Installed {
		t.Fatalf("status reports an install after a refusal: %+v", s.App)
	}

	appNativeStep(t, "installs_bundle")
	appNativeHome(t, good)
	r := appNativeResultOf(t, "installs_bundle", appNativeJSON(t, "install", "app", "--json"))
	if r.Version != "0.1.7" || r.Format != "app-bundle" || r.Location != nativeBundle || r.Adopted {
		t.Fatalf("install result %+v", r)
	}

	appNativeStep(t, "bundle_is_unquarantined_and_signed")
	if err := exec.Command("xattr", "-p", "com.apple.quarantine", nativeBundle).Run(); err == nil {
		t.Fatal("the bundle carries com.apple.quarantine")
	}
	if out, err := exec.Command("xattr", "-r", "-l", nativeBundle).CombinedOutput(); err != nil || strings.Contains(string(out), "com.apple.quarantine") {
		t.Fatalf("xattr -r -l: %v\n%s", err, out)
	}
	if out, err := exec.Command("codesign", "--verify", "--deep", "--strict", nativeBundle).CombinedOutput(); err != nil {
		t.Fatalf("codesign: %v\n%s", err, out)
	}
	if out, err := exec.Command("/usr/libexec/PlistBuddy", "-c", "Print :CFBundleIdentifier", filepath.Join(nativeBundle, "Contents", "Info.plist")).Output(); err != nil || strings.TrimSpace(string(out)) != appNativeBundleID {
		t.Fatalf("bundle id %q: %v", out, err)
	}

	appNativeStep(t, "status_reports_app")
	s := appNativeStatusOf(t, "status_reports_app")
	if !s.App.Installed || str(s.App.Version) != "0.1.7" || str(s.App.ReceiptVersion) != "0.1.7" || str(s.App.Format) != "app-bundle" ||
		str(s.App.Location) != nativeBundle || s.App.UpdatedByApp == nil || *s.App.UpdatedByApp ||
		str(s.App.Latest) != "0.1.7" || s.App.UpdateAvailable == nil || *s.App.UpdateAvailable {
		t.Fatalf("status.app = %+v", s.App)
	}
	appNativeToolOrder(t, "status_reports_app", s)

	appNativeStep(t, "running_app_blocks_uninstall")
	if err := exec.Command("open", "-n", "-g", nativeBundle, "--env", "HOME="+t.TempDir(), "--env", "FESTIVAL_APP_DISABLE_DAEMON_SPAWN=1").Run(); err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { appNativeStop(t) })
	deadline := time.Now().Add(30 * time.Second)
	var pid int
	for pid == 0 {
		if pids := appNativeRunning(t, true); len(pids) > 0 {
			pid = pids[0]
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Festival App never started")
		}
		time.Sleep(500 * time.Millisecond)
	}
	time.Sleep(5 * time.Second)
	if err := syscall.Kill(pid, 0); err != nil {
		t.Fatalf("Festival App (pid %d) did not stay running: %v", pid, err)
	}
	appNativeFail(t, "running_app_blocks_uninstall", "E_APP_RUNNING", appNativeJSON(t, "uninstall", "app", "--json"))
	if _, err := os.Lstat(nativeBundle); err != nil {
		t.Fatalf("the bundle was touched while running: %v", err)
	}
	appNativeStop(t)

	appNativeStep(t, "adopts_identified_copy")
	appNativeHome(t, good)
	if r := appNativeResultOf(t, "adopts_identified_copy", appNativeJSON(t, "install", "app", "--json")); !r.Adopted {
		t.Fatalf("expected adopted: %+v", r)
	}
	appNativeHome(t, good)
	out, errOut, err := runInstaller(t, "install", "app")
	if err != nil || !strings.Contains(out, "Replaced the existing Festival App at "+nativeBundle) {
		t.Fatalf("human adopt output: %v\n%s\n%s", err, out, errOut)
	}

	appNativeStep(t, "uninstall_removes_recorded_location")
	appNativeResultOf(t, "uninstall_removes_recorded_location", appNativeJSON(t, "uninstall", "app", "--json"))
	for _, p := range []string{nativeBundle, filepath.Join(os.Getenv("HOME"), "Applications", "Festival App.app")} {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Fatalf("%s still present: %v", p, err)
		}
	}
	if s := appNativeStatusOf(t, "uninstall_removes_recorded_location"); s.App.Installed {
		t.Fatalf("status.app = %+v", s.App)
	}
}

// appNativeStop sends SIGTERM by pid to every process under the bundle, then
// SIGKILL after 15 seconds. Never by name.
func appNativeStop(t *testing.T) {
	t.Helper()
	pids := appNativeRunning(t, false)
	for _, pid := range pids {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
	deadline := time.Now().Add(15 * time.Second)
	for len(appNativeRunning(t, false)) > 0 && time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
	}
	for _, pid := range appNativeRunning(t, false) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}
