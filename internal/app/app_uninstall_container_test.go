//go:build container_fs && linux

package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/state"
	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
)

func bundleRec(loc string) appReceipt {
	return appReceipt{Version: "0.1.0", Format: "app-bundle", Location: loc}
}

func TestAppBundleRemoveDeletesOnlyValidatedBundle(t *testing.T) {
	e := newBundleEnv(t, "0.1.8")
	writeInstalledBundle(t, e.sys(), AppBundleID, "0.1.0")
	neighbor := filepath.Join(e.systemApps, "Other Thing.app")
	writeInstalledBundle(t, neighbor, "com.example.other", "1.0.0")
	removed, notes, err := e.ai.platform.remove(context.Background(), bundleRec(e.sys()), nil)
	if err != nil || !removed || len(notes) != 0 {
		t.Fatalf("%v %v %v", removed, notes, err)
	}
	if _, statErr := os.Lstat(e.sys()); !os.IsNotExist(statErr) {
		t.Fatal("bundle still present")
	}
	if bundleExe(t, neighbor) != "old-1.0.0" {
		t.Fatal("a neighbor was touched")
	}
	assertNoBundleSiblings(t, e.systemApps)
}

func TestAppBundleRemoveRefusesWrongBundleID(t *testing.T) {
	e := newBundleEnv(t, "0.1.8")
	writeInstalledBundle(t, e.sys(), "com.example.other", "1.0.0")
	_, _, err := e.ai.platform.remove(context.Background(), bundleRec(e.sys()), nil)
	if errpkg.Code(err) != "E_APP_FOREIGN" || bundleExe(t, e.sys()) != "old-1.0.0" {
		t.Fatalf("err=%v", err)
	}
}

func TestAppBundleRemoveRefusesLocationOutsideRoots(t *testing.T) {
	e := newBundleEnv(t, "0.1.8")
	elsewhere := filepath.Join(e.root, "elsewhere", appBundleName)
	writeInstalledBundle(t, elsewhere, AppBundleID, "0.1.0")
	renamed := filepath.Join(e.systemApps, "Renamed.app")
	writeInstalledBundle(t, renamed, AppBundleID, "0.1.0")
	for _, loc := range []string{elsewhere, renamed, "Festival App.app", e.systemApps + "/../Applications/" + appBundleName} {
		if _, _, err := e.ai.platform.remove(context.Background(), bundleRec(loc), nil); errpkg.Code(err) != "E_APP_FOREIGN" {
			t.Errorf("%s: %v", loc, err)
		}
	}
	if bundleExe(t, elsewhere) != "old-0.1.0" || bundleExe(t, renamed) != "old-0.1.0" {
		t.Fatal("a refused location was touched")
	}
}

func TestAppImageRemoveDeletesBinaryEntryAndIcon(t *testing.T) {
	e := newUpdEnv(t)
	e.publish("0.2.0", []byte(fakeAppImageScript+"# 0.2.0\n"))
	res := e.install()
	desktop := filepath.Join(e.f.Home, ".local", "share", "applications", "festival-app.desktop")
	icon := filepath.Join(e.f.Home, ".local", "share", "icons", "hicolor", "512x512", "apps", "festival-app.png")
	for _, p := range []string{res.Location, desktop, icon} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("setup: %s missing (%v)", p, err)
		}
	}
	got, err := UninstallApp(context.Background(), nil)
	if err != nil || !got.Changed || got.Location != res.Location {
		t.Fatalf("%+v %v", got, err)
	}
	for _, p := range []string{res.Location, desktop, icon} {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Fatalf("%s still present", p)
		}
	}
	if rec, err := readAppReceipt(context.Background()); err != nil || rec != nil {
		t.Fatalf("receipt still present: %+v %v", rec, err)
	}
	t.Logf("uninstall result: %+v", got)
}

func TestAppImageRemoveKeepsForeignDesktopEntry(t *testing.T) {
	e := newUpdEnv(t)
	e.publish("0.2.0", []byte("appimage 0.2.0"))
	e.install()
	desktop := filepath.Join(e.f.Home, ".local", "share", "applications", "festival-app.desktop")
	if err := os.WriteFile(desktop, []byte("[Desktop Entry]\nName=Mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := UninstallApp(context.Background(), nil)
	if err != nil || !strings.Contains(strings.Join(got.Notes, "\n"), "Left your "+desktop+" because festival did not write it.") {
		t.Fatalf("%+v %v", got, err)
	}
	if _, statErr := os.Stat(desktop); statErr != nil {
		t.Fatal("the foreign desktop entry was deleted")
	}
}

func debRemoveEnv(t *testing.T, exit int) (*debEnv, *int) {
	t.Helper()
	e := newDebEnv(t)
	e.installed, e.version = true, "0.2.0"
	e.res = appCommandResult{ExitCode: exit}
	calls := 0
	e.onRun = func(string, []string) {
		calls++
		if exit == 0 {
			e.installed = false
		}
	}
	return e, &calls
}

func TestAppDebRemoveRunsPkexecAptGetRemove(t *testing.T) {
	e, calls := debRemoveEnv(t, 0)
	removed, notes, err := e.d.remove(context.Background(), appReceipt{Format: "deb", Location: "dpkg:festival-app"}, nil)
	if err != nil || !removed || len(notes) != 0 || *calls != 1 {
		t.Fatalf("%v %v %v calls=%d", removed, notes, err, *calls)
	}
	if strings.Join(e.calls[0], " ") != "/usr/bin/pkexec /usr/bin/apt-get remove --yes festival-app" {
		t.Fatalf("argv = %v", e.calls[0])
	}
}

func TestAppDebRemoveElevationKeepsReceipt(t *testing.T) {
	e, _ := debRemoveEnv(t, 126)
	statusEnv(t)
	store := storeFor(t)
	if err := store.Write(context.Background(), appReceipt{
		Version: "0.2.0", Source: "official-obey", Channel: "stable", Format: "deb", Location: "dpkg:festival-app",
		BundleID: AppBundleID, ArtifactSHA256: strings.Repeat("a", 64), Executable: "/usr/bin/festival-app", Mode: 0o755,
	}); err != nil {
		t.Fatal(err)
	}
	prev := appPlatformFor
	appPlatformFor = func(context.Context) (appPlatform, error) { return linuxPlatform{deb: e.d}, nil }
	t.Cleanup(func() { appPlatformFor = prev })
	_, err := UninstallApp(context.Background(), nil)
	if errpkg.Code(err) != "E_APP_ELEVATION" || !strings.Contains(err.Error(), "Or remove it yourself with: sudo apt-get remove --yes festival-app") {
		t.Fatalf("err = %v", err)
	}
	if rec, rerr := readAppReceipt(context.Background()); rerr != nil || rec == nil {
		t.Fatalf("receipt must be kept: %+v %v", rec, rerr)
	}
}

func TestAppUninstallRefusesRunningApp(t *testing.T) {
	e := newUpdEnv(t)
	e.publish("0.2.0", []byte("appimage 0.2.0"))
	res := e.install()
	cmd := startSleeper(t, res.Location)
	_ = waitRunning(t, appRunScope{Exes: []string{res.Location}}, cmd.Process.Pid)
	_, err := UninstallApp(context.Background(), nil)
	if errpkg.Code(err) != "E_APP_RUNNING" {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(res.Location); statErr != nil {
		t.Fatal("the running App's file was removed")
	}
	if rec, rerr := readAppReceipt(context.Background()); rerr != nil || rec == nil {
		t.Fatalf("receipt must be kept: %+v %v", rec, rerr)
	}
}

func TestAppUninstallRemovedOutsideFestivalClearsReceipt(t *testing.T) {
	e := newUpdEnv(t)
	e.publish("0.2.0", []byte("appimage 0.2.0"))
	res := e.install()
	if err := os.Remove(res.Location); err != nil {
		t.Fatal(err)
	}
	got, err := UninstallApp(context.Background(), nil)
	if err != nil || got.Changed || len(got.Notes) != 1 || got.Notes[0] != "Festival App was removed outside festival; removed festival's record of it." {
		t.Fatalf("%+v %v", got, err)
	}
	if rec, rerr := readAppReceipt(context.Background()); rerr != nil || rec != nil {
		t.Fatalf("receipt must be cleared: %+v %v", rec, rerr)
	}
}

func TestAppUninstallNoReceiptIsNoOp(t *testing.T) {
	_, festivalHome := statusEnv(t)
	got, err := UninstallApp(context.Background(), nil)
	if err != nil || got.Changed || len(got.Notes) != 1 || got.Notes[0] != "Festival App is not installed through festival; nothing to uninstall." {
		t.Fatalf("%+v %v", got, err)
	}
	if _, statErr := os.Stat(festivalHome); !os.IsNotExist(statErr) {
		t.Fatal("uninstall created the installer home")
	}
}

func TestAppUninstallPackageRoutesForTUI(t *testing.T) {
	e := newUpdEnv(t)
	e.publish("0.2.0", []byte("appimage 0.2.0"))
	res := e.install()
	got, err := UninstallPackage(context.Background(), AppPackageID)
	if err != nil {
		t.Fatalf("the TUI path must not fail E_UNINSTALL_PATH for the App: %v", err)
	}
	if len(got.Removed) != 1 || got.Removed[0] != res.Location {
		t.Fatalf("%+v", got)
	}
	if _, statErr := os.Lstat(res.Location); !os.IsNotExist(statErr) {
		t.Fatal("AppImage still present")
	}
}

func TestAppUninstallKeepsManagedRootsRuleForPlugins(t *testing.T) {
	home, _ := statusEnv(t)
	ctx := context.Background()
	if err := state.EnsureHome(ctx, 0o700); err != nil {
		t.Fatal(err)
	}
	fh, _ := state.Home(ctx)
	db := openTestDB(t, fh)
	outside := filepath.Join(home, "somewhere", "camp-x")
	if err := os.MkdirAll(filepath.Dir(outside), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("not managed"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := receipts.Write(ctx, db, receipts.Receipt{
		PackageID: "acme/camp-x", Version: "1.0.0", Source: "s", Channel: "stable", InstalledAt: time.Now().UTC(),
		OwnedFiles: []receipts.OwnedFile{{Path: outside, Hash: "x", Mode: 0o755}},
	}); err != nil {
		t.Fatal(err)
	}
	_, err := UninstallPackage(ctx, "acme/camp-x")
	if errpkg.Code(err) != "E_UNINSTALL_PATH" {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(outside); statErr != nil {
		t.Fatal("a file outside the managed roots was removed")
	}
}
