//go:build app_native && linux

package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/apptest"
)

const nativePolkitRule = "/etc/polkit-1/rules.d/49-festival-installer-ci.rules"

func nativeDpkgStatus() string {
	out, _ := exec.Command("dpkg-query", "-W", "-f", "${db:Status-Status}", "festival-app").Output()
	return strings.TrimSpace(string(out))
}

func nativeFileInfo(t *testing.T, path string) (sha string, mtime int64) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return appNativeSHA(raw), fi.ModTime().UnixNano()
}

func TestAppNativeDebElevationWithoutAgent(t *testing.T) {
	apptest.RequireHostedRunner(t)
	if _, err := os.Stat(nativePolkitRule); err == nil {
		t.Fatalf("%s exists; this test must run before the rule is installed", nativePolkitRule)
	}
	asset := appNativeAssetFor(t, "deb")
	key := appNativeKey(t)
	good := t.TempDir()
	appNativeMarketplace(t, key, good, []map[string]any{appNativeArtifact(t, asset, asset)})
	appNativeHome(t, good)
	env := appNativeJSON(t, "install", "app", "--format", "deb", "--json")
	appNativeFail(t, "elevation", "E_APP_ELEVATION", env)
	for _, want := range []string{"no password prompt is available in this session", "festival install app --format appimage", "sudo apt-get install --yes"} {
		if !strings.Contains(strings.ToLower(env.Error.Message), strings.ToLower(want)) {
			t.Fatalf("message lacks %q: %s", want, env.Error.Message)
		}
	}
	if st := nativeDpkgStatus(); st == "installed" {
		t.Fatalf("dpkg status %q after a refused install", st)
	}
}

func TestAppNativeDebInstallStatusAdoptUninstall(t *testing.T) {
	apptest.RequireHostedRunner(t)
	asset := appNativeAssetFor(t, "deb")
	key := appNativeKey(t)
	good, bad := t.TempDir(), t.TempDir()
	appNativeMarketplace(t, key, good, []map[string]any{appNativeArtifact(t, asset, asset)})
	appNativeMarketplace(t, key, bad, []map[string]any{appNativeArtifact(t, asset, appNativeAssetFor(t, "appimage"))})

	appNativeStep(t, "refuses_mismatched_updater_signature")
	appNativeHome(t, bad)
	appNativeFail(t, "refuses_mismatched_updater_signature", "E_APP_SIGNATURE", appNativeJSON(t, "install", "app", "--json"))
	if st := nativeDpkgStatus(); st == "installed" {
		t.Fatalf("dpkg status %q after a refusal", st)
	}

	appNativeStep(t, "installs_deb")
	appNativeHome(t, good)
	r := appNativeResultOf(t, "installs_deb", appNativeJSON(t, "install", "app", "--json"))
	if r.Format != "deb" || r.Location != "dpkg:festival-app" || r.Adopted {
		t.Fatalf("install result %+v", r)
	}
	if out, err := exec.Command("dpkg-query", "-W", "-f", "${Version}", "festival-app").Output(); err != nil || strings.TrimSpace(string(out)) != "0.1.7" {
		t.Fatalf("dpkg version %q: %v", out, err)
	}
	if _, err := os.Stat("/usr/bin/festival-app"); err != nil {
		t.Fatal(err)
	}

	appNativeStep(t, "status_reports_app")
	s := appNativeStatusOf(t, "status_reports_app")
	if !s.App.Installed || str(s.App.Format) != "deb" || str(s.App.Version) != "0.1.7" {
		t.Fatalf("status.app = %+v", s.App)
	}

	appNativeStep(t, "adopts_identified_copy")
	appNativeHome(t, good)
	if r := appNativeResultOf(t, "adopts_identified_copy", appNativeJSON(t, "install", "app", "--json")); !r.Adopted {
		t.Fatalf("expected adopted: %+v", r)
	}

	appNativeStep(t, "uninstall")
	appNativeResultOf(t, "uninstall", appNativeJSON(t, "uninstall", "app", "--json"))
	if st := nativeDpkgStatus(); st == "installed" {
		t.Fatalf("dpkg status %q after uninstall", st)
	}
	if _, err := os.Stat("/usr/bin/festival-app"); !os.IsNotExist(err) {
		t.Fatalf("/usr/bin/festival-app still present: %v", err)
	}
}

func TestAppNativeAppImageInstallStatusReinstallUninstall(t *testing.T) {
	apptest.RequireHostedRunner(t)
	asset := appNativeAssetFor(t, "appimage")
	key := appNativeKey(t)
	good, bad := t.TempDir(), t.TempDir()
	appNativeMarketplace(t, key, good, []map[string]any{appNativeArtifact(t, asset, asset)})
	appNativeMarketplace(t, key, bad, []map[string]any{appNativeArtifact(t, asset, appNativeAssetFor(t, "deb"))})

	appNativeStep(t, "refuses_mismatched_updater_signature")
	appNativeHome(t, bad)
	appNativeFail(t, "refuses_mismatched_updater_signature", "E_APP_SIGNATURE", appNativeJSON(t, "install", "app", "--format", "appimage", "--json"))
	exe := filepath.Join(os.Getenv("HOME"), ".local", "bin", "festival-app")
	if _, err := os.Stat(exe); !os.IsNotExist(err) {
		t.Fatalf("%s exists after a refusal: %v", exe, err)
	}

	appNativeStep(t, "installs_appimage")
	homeA := appNativeHome(t, good)
	r := appNativeResultOf(t, "installs_appimage", appNativeJSON(t, "install", "app", "--format", "appimage", "--json"))
	if r.Format != "appimage" || r.Location != exe || r.Adopted {
		t.Fatalf("install result %+v", r)
	}
	fi, err := os.Stat(exe)
	if err != nil || fi.Mode().Perm() != 0o755 {
		t.Fatalf("binary: %v %v", fi, err)
	}
	if sha, _ := nativeFileInfo(t, exe); sha != asset.SHA256 {
		t.Fatalf("sha256 %s, want %s", sha, asset.SHA256)
	}
	desktop := filepath.Join(os.Getenv("HOME"), ".local", "share", "applications", "festival-app.desktop")
	raw, err := os.ReadFile(desktop)
	if err != nil || !strings.Contains(string(raw), exe) {
		t.Fatalf("desktop entry: %v\n%s", err, raw)
	}
	icon := filepath.Join(os.Getenv("HOME"), ".local", "share", "icons", "hicolor", "512x512", "apps", "festival-app.png")
	if b, err := os.ReadFile(icon); err != nil || len(b) < 4 || string(b[:4]) != "\x89PNG" {
		t.Fatalf("icon: %v", err)
	}

	appNativeStep(t, "status_reports_app")
	s := appNativeStatusOf(t, "status_reports_app")
	if !s.App.Installed || str(s.App.Format) != "appimage" || str(s.App.Version) != "0.1.7" || s.App.UpdatedByApp == nil || *s.App.UpdatedByApp {
		t.Fatalf("status.app = %+v", s.App)
	}

	appNativeStep(t, "reinstall_same_home")
	shaBefore, _ := nativeFileInfo(t, exe)
	t.Setenv("FESTIVAL_HOME", homeA)
	r = appNativeResultOf(t, "reinstall_same_home", appNativeJSON(t, "install", "app", "--format", "appimage", "--json"))
	if r.Adopted {
		t.Fatalf("a reinstall over the receipt's own copy must not be adopted: %+v", r)
	}
	if sha, _ := nativeFileInfo(t, exe); sha != shaBefore {
		t.Fatal("the binary changed on a current reinstall")
	}

	appNativeStep(t, "foreign_in_a_fresh_home")
	appNativeHome(t, good)
	shaFor, mtimeFor := nativeFileInfo(t, exe)
	env := appNativeJSON(t, "install", "app", "--format", "appimage", "--json")
	appNativeFail(t, "foreign_in_a_fresh_home", "E_APP_FOREIGN", env)
	if !strings.Contains(env.Error.Message, exe) {
		t.Fatalf("message does not name the path: %s", env.Error.Message)
	}
	if sha, mtime := nativeFileInfo(t, exe); sha != shaFor || mtime != mtimeFor {
		t.Fatal("the foreign file was changed")
	}

	appNativeStep(t, "uninstall")
	t.Setenv("FESTIVAL_HOME", homeA)
	appNativeResultOf(t, "uninstall", appNativeJSON(t, "uninstall", "app", "--json"))
	for _, p := range []string{exe, desktop, icon} {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Fatalf("%s still present: %v", p, err)
		}
	}
}
