//go:build app_native && windows

package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"

	"github.com/Obedience-Corp/festival-installer/internal/apptest"
)

const nativeUninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\Festival App`

func nativeRegValue(t *testing.T, name string) (string, bool) {
	t.Helper()
	k, err := registry.OpenKey(registry.CURRENT_USER, nativeUninstallKey, registry.QUERY_VALUE)
	if err == registry.ErrNotExist {
		return "", false
	}
	if err != nil {
		t.Fatalf("open HKCU\\%s: %v", nativeUninstallKey, err)
	}
	defer func() { _ = k.Close() }()
	v, _, err := k.GetStringValue(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return v, true
}

func TestAppNativeWindowsInstallStatusAdoptUninstall(t *testing.T) {
	apptest.RequireHostedRunner(t)
	asset := appNativeAssetFor(t, "nsis")
	key := appNativeKey(t)
	good, bad := t.TempDir(), t.TempDir()
	appNativeMarketplace(t, key, good, []map[string]any{appNativeArtifact(t, asset, asset)})
	appNativeMarketplace(t, key, bad, []map[string]any{appNativeArtifact(t, asset, appNativeAssets[3])}) // the deb .sig
	dir := filepath.Join(os.Getenv("LOCALAPPDATA"), "Festival App")

	appNativeStep(t, "refuses_mismatched_updater_signature")
	appNativeHome(t, bad)
	appNativeFail(t, "refuses_mismatched_updater_signature", "E_APP_SIGNATURE", appNativeJSON(t, "install", "app", "--json"))
	if _, ok := nativeRegValue(t, "DisplayVersion"); ok {
		t.Fatal("the registry key exists after a refusal")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("install dir exists after a refusal: %v", err)
	}

	appNativeStep(t, "installs")
	appNativeHome(t, good)
	r := appNativeResultOf(t, "installs", appNativeJSON(t, "install", "app", "--json"))
	if r.Format != "nsis" || !strings.EqualFold(r.Location, dir) || r.Adopted {
		t.Fatalf("install result %+v", r)
	}
	if v, ok := nativeRegValue(t, "DisplayVersion"); !ok || v != "0.1.7" {
		t.Fatalf("DisplayVersion %q ok=%v", v, ok)
	}
	if v, _ := nativeRegValue(t, "InstallLocation"); !strings.EqualFold(strings.Trim(v, `"`), dir) {
		t.Fatalf("InstallLocation %q, want %q", v, dir)
	}
	if _, err := os.Stat(filepath.Join(dir, "festival-app.exe")); err != nil {
		t.Fatalf("festival-app.exe: %v", err)
	}
	lnk := filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", "Festival App.lnk")
	if _, err := os.Stat(lnk); err != nil {
		t.Fatalf("Start menu shortcut: %v", err)
	}

	appNativeStep(t, "status_reports_app")
	s := appNativeStatusOf(t, "status_reports_app")
	if !s.App.Installed || str(s.App.Format) != "nsis" || str(s.App.Version) != "0.1.7" {
		t.Fatalf("status.app = %+v", s.App)
	}
	appNativeToolOrder(t, "status_reports_app", s)

	appNativeStep(t, "adopts_identified_copy")
	appNativeHome(t, good)
	if r := appNativeResultOf(t, "adopts_identified_copy", appNativeJSON(t, "install", "app", "--json")); !r.Adopted {
		t.Fatalf("expected adopted: %+v", r)
	}
	if v, _ := nativeRegValue(t, "DisplayVersion"); v != "0.1.7" {
		t.Fatalf("DisplayVersion after adopt %q", v)
	}

	appNativeStep(t, "uninstall_removes_recorded_location")
	appNativeResultOf(t, "uninstall_removes_recorded_location", appNativeJSON(t, "uninstall", "app", "--json"))
	if _, ok := nativeRegValue(t, "DisplayVersion"); ok {
		t.Fatal("registry key still present")
	}
	if _, err := os.Stat(filepath.Join(dir, "festival-app.exe")); !os.IsNotExist(err) {
		t.Fatalf("festival-app.exe still present: %v", err)
	}
}
