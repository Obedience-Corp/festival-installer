//go:build container_fs && linux

package app

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/source"
	"github.com/Obedience-Corp/festival-installer/internal/state"
)

func plistXML(id, version string) string {
	return `<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict>` +
		`<key>CFBundleIdentifier</key><string>` + id + `</string>` +
		`<key>CFBundleShortVersionString</key><string>` + version + `</string>` +
		`<key>CFBundleExecutable</key><string>festival-app</string></dict></plist>`
}

// writeBundleTarGz builds a tarball with one full bundle per root name.
func writeBundleTarGz(t *testing.T, id, version string, roots ...string) string {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	add := func(name string, mode int64, body string, dir bool) {
		hdr := &tar.Header{Name: name, Mode: mode, Typeflag: tar.TypeReg, Size: int64(len(body))}
		if dir {
			hdr.Typeflag, hdr.Size = tar.TypeDir, 0
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if !dir {
			if _, err := tw.Write([]byte(body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, root := range roots {
		add(root+"/", 0o755, "", true)
		add(root+"/Contents/", 0o755, "", true)
		add(root+"/Contents/MacOS/", 0o755, "", true)
		add(root+"/Contents/Info.plist", 0o644, plistXML(id, version), false)
		add(root+"/Contents/MacOS/festival-app", 0o755, "new-"+version, false)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "app.tar.gz")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeInstalledBundle places an existing bundle whose executable holds "old-<version>".
func writeInstalledBundle(t *testing.T, bundle, id, version string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(bundle, "Contents", "MacOS"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "Contents", "Info.plist"), []byte(plistXML(id, version)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "Contents", "MacOS", "festival-app"), []byte("old-"+version), 0o755); err != nil {
		t.Fatal(err)
	}
}

func bundleExe(t *testing.T, bundle string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(bundle, "Contents", "MacOS", "festival-app"))
	if err != nil {
		return ""
	}
	return string(b)
}

type bundleEnv struct {
	t                    *testing.T
	root                 string
	systemApps, userApps string
	owners               map[string]int
	unwritable           map[string]bool
	codesignErr          error
	archive              string
	store                *fakeAppStore
	resolves, downloads  int
	resolveErr           error
	ai                   appInstaller
}

func newBundleEnv(t *testing.T, latest string) *bundleEnv {
	t.Helper()
	root := t.TempDir()
	e := &bundleEnv{
		t: t, root: root, systemApps: filepath.Join(root, "Applications"), userApps: filepath.Join(root, "home", "Applications"),
		owners: map[string]int{}, unwritable: map[string]bool{}, store: &fakeAppStore{},
		archive: writeBundleTarGz(t, AppBundleID, latest, appBundleName),
	}
	if err := os.MkdirAll(e.systemApps, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FESTIVAL_HOME", filepath.Join(root, "home", "installer"))
	plat := newBundlePlatform(e.systemApps, e.userApps, 501,
		func(p string) (int, error) {
			if uid, ok := e.owners[p]; ok {
				return uid, nil
			}
			return 501, nil
		},
		func(p string) bool { return !e.unwritable[p] },
		func(context.Context, string) error { return e.codesignErr })
	e.ai = appInstaller{
		goos: "darwin", euid: func() int { return 501 }, platform: plat,
		resolve: func(context.Context, string, string, source.VerifyOptions) (appRelease, error) {
			e.resolves++
			if e.resolveErr != nil {
				return appRelease{}, e.resolveErr
			}
			rel := appRelease{PackageID: AppPackageID, Version: latest, Channel: "stable", Source: "official-obey", Format: "app-bundle", BundleID: AppBundleID}
			rel.Artifact.URL = "https://example.invalid/Festival.App.app.tar.gz"
			rel.Artifact.Sha256 = strings.Repeat("a", 64)
			rel.Artifact.Filename = "Festival.App.app.tar.gz"
			return rel, nil
		},
		download: func(_ context.Context, _ string, rel appRelease, _ appDownloadOptions) (verifiedAppArtifact, error) {
			e.downloads++
			return verifiedAppArtifact{Path: e.archive, Release: rel}, nil
		},
		begin: beginAppTxn, refuseRunning: refuseRunningApp, receipts: e.store.factory(),
	}
	return e
}

func (e *bundleEnv) install() (AppResult, error) {
	return e.ai.install(context.Background(), AppInstallOptions{})
}

func (e *bundleEnv) sys() string  { return filepath.Join(e.systemApps, appBundleName) }
func (e *bundleEnv) user() string { return filepath.Join(e.userApps, appBundleName) }

func assertNoBundleSiblings(t *testing.T, dirs ...string) {
	t.Helper()
	for _, dir := range dirs {
		entries, _ := os.ReadDir(dir)
		for _, en := range entries {
			if strings.Contains(en.Name(), ".festival-") {
				t.Fatalf("sibling left in %s: %s", dir, en.Name())
			}
		}
	}
}

func TestAppBundleFreshInstallPrefersSystemApplications(t *testing.T) {
	e := newBundleEnv(t, "0.1.8")
	res, err := e.install()
	if err != nil {
		t.Fatal(err)
	}
	if res.Location != e.sys() || res.Version != "0.1.8" || res.Adopted || !res.Changed || res.Package != AppPackageID {
		t.Fatalf("%+v", res)
	}
	if bundleExe(t, e.sys()) != "new-0.1.8" || e.store.r == nil || e.store.r.Location != e.sys() || e.store.r.Executable != filepath.Join(e.sys(), "Contents/MacOS/festival-app") {
		t.Fatalf("exe=%q receipt=%+v", bundleExe(t, e.sys()), e.store.r)
	}
	assertNoBundleSiblings(t, e.systemApps)
}

func TestAppBundleFallsBackToUserApplicationsWithNote(t *testing.T) {
	e := newBundleEnv(t, "0.1.8")
	e.unwritable[e.systemApps] = true
	res, err := e.install()
	if err != nil {
		t.Fatal(err)
	}
	want := "Installed to ~/Applications because this account cannot write to " + e.systemApps + "."
	if res.Location != e.user() || len(res.Notes) != 1 || res.Notes[0] != want {
		t.Fatalf("%+v want note %q", res, want)
	}
	if bundleExe(t, e.user()) != "new-0.1.8" {
		t.Fatal("bundle not placed")
	}
}

func TestAppBundleReplacesIdentifiedCopyInPlaceAndAdopts(t *testing.T) {
	e := newBundleEnv(t, "0.1.8")
	writeInstalledBundle(t, e.sys(), AppBundleID, "0.1.0")
	res, err := e.install()
	if err != nil {
		t.Fatal(err)
	}
	if res.Location != e.sys() || !res.Adopted || res.PreviousVersion != "0.1.0" || len(res.Notes) == 0 || res.Notes[0] != "Replaced the existing Festival App at "+e.sys() {
		t.Fatalf("%+v", res)
	}
	if bundleExe(t, e.sys()) != "new-0.1.8" || !e.store.r.Adopted {
		t.Fatalf("exe=%q receipt=%+v", bundleExe(t, e.sys()), e.store.r)
	}
	assertNoBundleSiblings(t, e.systemApps)
}

func TestAppBundleDiscoveryUsesReceiptLocationFirst(t *testing.T) {
	e := newBundleEnv(t, "0.1.8")
	writeInstalledBundle(t, e.sys(), AppBundleID, "0.1.0")
	writeInstalledBundle(t, e.user(), AppBundleID, "0.1.0")
	e.store.r = &appReceipt{Version: "0.1.0", Format: "app-bundle", Location: e.user()}
	res, err := e.install()
	if err != nil {
		t.Fatal(err)
	}
	if res.Location != e.user() || res.Adopted {
		t.Fatalf("%+v", res)
	}
	if bundleExe(t, e.sys()) != "old-0.1.0" || bundleExe(t, e.user()) != "new-0.1.8" {
		t.Fatalf("sys=%q user=%q", bundleExe(t, e.sys()), bundleExe(t, e.user()))
	}
}

func TestAppBundleRefusesForeignCopy(t *testing.T) {
	e := newBundleEnv(t, "0.1.8")
	writeInstalledBundle(t, e.sys(), "com.example.other", "1.0.0")
	_, err := e.install()
	if errpkg.Code(err) != "E_APP_FOREIGN" || e.downloads != 0 || bundleExe(t, e.sys()) != "old-1.0.0" {
		t.Fatalf("err=%v downloads=%d exe=%q", err, e.downloads, bundleExe(t, e.sys()))
	}
}

func TestAppBundleRefusesSymlinkAtTarget(t *testing.T) {
	e := newBundleEnv(t, "0.1.8")
	elsewhere := filepath.Join(e.root, "elsewhere")
	writeInstalledBundle(t, elsewhere, AppBundleID, "0.1.0")
	if err := os.Symlink(elsewhere, e.sys()); err != nil {
		t.Fatal(err)
	}
	_, err := e.install()
	if errpkg.Code(err) != "E_APP_FOREIGN" || e.downloads != 0 || bundleExe(t, elsewhere) != "old-0.1.0" {
		t.Fatalf("err=%v", err)
	}
}

func TestAppBundleCodesignFailureLeavesTargetAndNoSibling(t *testing.T) {
	e := newBundleEnv(t, "0.1.8")
	writeInstalledBundle(t, e.sys(), AppBundleID, "0.1.0")
	e.codesignErr = errors.New("a sealed resource is missing")
	_, err := e.install()
	if errpkg.Code(err) != "E_APP_BUNDLE_INVALID" {
		t.Fatalf("err = %v", err)
	}
	if bundleExe(t, e.sys()) != "old-0.1.0" || e.store.r != nil {
		t.Fatalf("exe=%q receipt=%+v", bundleExe(t, e.sys()), e.store.r)
	}
	assertNoBundleSiblings(t, e.systemApps)
}

func TestAppBundleRejectsWrongArchiveRoot(t *testing.T) {
	for name, roots := range map[string][]string{"wrong root": {"Other.app"}, "two roots": {appBundleName, "Extra.app"}} {
		t.Run(name, func(t *testing.T) {
			e := newBundleEnv(t, "0.1.8")
			e.archive = writeBundleTarGz(t, AppBundleID, "0.1.8", roots...)
			_, err := e.install()
			if errpkg.Code(err) != "E_APP_BUNDLE_INVALID" {
				t.Fatalf("err = %v", err)
			}
			if _, statErr := os.Lstat(e.sys()); !os.IsNotExist(statErr) {
				t.Fatal("a target was created")
			}
			assertNoBundleSiblings(t, e.systemApps)
		})
	}
}

func TestAppBundleRejectsIDOrVersionMismatch(t *testing.T) {
	cases := map[string][2]string{"wrong id": {"com.example.other", "0.1.8"}, "wrong version": {AppBundleID, "0.1.9"}}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			e := newBundleEnv(t, "0.1.8")
			e.archive = writeBundleTarGz(t, c[0], c[1], appBundleName)
			_, err := e.install()
			if errpkg.Code(err) != "E_APP_BUNDLE_INVALID" {
				t.Fatalf("err = %v", err)
			}
			assertNoBundleSiblings(t, e.systemApps)
		})
	}
}

func TestAppBundleReceiptFailureRestoresOldCopy(t *testing.T) {
	e := newBundleEnv(t, "0.1.8")
	writeInstalledBundle(t, e.sys(), AppBundleID, "0.1.0")
	boom := errors.New("receipt store down")
	e.store.failWrite = boom
	_, err := e.install()
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if bundleExe(t, e.sys()) != "old-0.1.0" {
		t.Fatalf("old copy not restored: %q", bundleExe(t, e.sys()))
	}
	assertNoBundleSiblings(t, e.systemApps)
}

func TestAppBundleOtherAccountCopyInstallsForThisAccount(t *testing.T) {
	e := newBundleEnv(t, "0.1.8")
	writeInstalledBundle(t, e.sys(), AppBundleID, "0.1.0")
	e.owners[e.sys()] = 999
	res, err := e.install()
	if err != nil {
		t.Fatal(err)
	}
	want := "Festival App in " + e.systemApps + " belongs to another account; installed for this account in ~/Applications/Festival App.app"
	if res.Location != e.user() || res.Adopted || len(res.Notes) != 1 || res.Notes[0] != want {
		t.Fatalf("%+v want note %q", res, want)
	}
	if bundleExe(t, e.sys()) != "old-0.1.0" || bundleExe(t, e.user()) != "new-0.1.8" || e.store.r.Location != e.user() {
		t.Fatalf("sys=%q user=%q receipt=%+v", bundleExe(t, e.sys()), bundleExe(t, e.user()), e.store.r)
	}
}

func TestAppBundleUnwritableCopyIsTreatedAsOtherAccount(t *testing.T) {
	e := newBundleEnv(t, "0.1.8")
	writeInstalledBundle(t, e.sys(), AppBundleID, "0.1.0")
	e.unwritable[e.systemApps] = true
	res, err := e.install()
	if err != nil {
		t.Fatal(err)
	}
	if res.Location != e.user() || bundleExe(t, e.sys()) != "old-0.1.0" || !strings.Contains(strings.Join(res.Notes, "\n"), "belongs to another account") {
		t.Fatalf("%+v", res)
	}
}

func TestAppInstallerCurrentSkipsDownload(t *testing.T) {
	e := newBundleEnv(t, "0.1.8")
	writeInstalledBundle(t, e.sys(), AppBundleID, "0.1.8")
	e.store.r = &appReceipt{Version: "0.1.8", Format: "app-bundle", Location: e.sys()}
	res, err := e.install()
	if err != nil || res.Changed || res.Package != AppPackageID || e.downloads != 0 || len(res.Notes) != 1 {
		t.Fatalf("%+v %v downloads=%d", res, err, e.downloads)
	}
}

func TestAppInstallerNewerOnDiskLeftUntouched(t *testing.T) {
	e := newBundleEnv(t, "0.1.8")
	writeInstalledBundle(t, e.sys(), AppBundleID, "0.9.0")
	e.store.r = &appReceipt{Version: "0.1.0", Format: "app-bundle", Location: e.sys()}
	res, err := e.install()
	if err != nil || res.Changed || res.Package != AppPackageID || e.downloads != 0 || bundleExe(t, e.sys()) != "old-0.9.0" {
		t.Fatalf("%+v %v downloads=%d", res, err, e.downloads)
	}
}

func TestAppInstallerRunningAppRefusedBeforeDownload(t *testing.T) {
	e := newBundleEnv(t, "0.1.8")
	writeInstalledBundle(t, e.sys(), AppBundleID, "0.1.0")
	e.store.r = &appReceipt{Version: "0.1.0", Format: "app-bundle", Location: e.sys()}
	cmd := startSleeper(t, filepath.Join(e.sys(), "Contents", "MacOS", "festival-app"))
	_ = waitRunning(t, appRunScope{Dirs: []string{e.sys()}}, cmd.Process.Pid)
	_, err := e.install()
	if errpkg.Code(err) != "E_APP_RUNNING" || e.downloads != 0 {
		t.Fatalf("err=%v downloads=%d", err, e.downloads)
	}
}

func TestAppInstallerFreshHomeReachesResolve(t *testing.T) {
	e := newBundleEnv(t, "0.1.8")
	home := filepath.Join(e.root, "fresh", "installer")
	t.Setenv("FESTIVAL_HOME", home)
	e.resolveErr = errpkg.New("E_APP_NOT_AVAILABLE", "fixture")
	_, err := e.install()
	if errpkg.Code(err) != "E_APP_NOT_AVAILABLE" || e.resolves != 1 {
		t.Fatalf("err=%v resolves=%d", err, e.resolves)
	}
	fi, statErr := os.Stat(home)
	if statErr != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("home: %v %v", fi, statErr)
	}
	if _, statErr := os.Stat(state.DatabasePath(home)); statErr != nil {
		t.Fatalf("state.db missing: %v", statErr)
	}
}

func TestAppInstallerUpdateOnlyFreshHomeCreatesNothing(t *testing.T) {
	e := newBundleEnv(t, "0.1.8")
	home := filepath.Join(e.root, "fresh", "installer")
	t.Setenv("FESTIVAL_HOME", home)
	res, err := e.ai.install(context.Background(), AppInstallOptions{UpdateOnly: true})
	if err != nil || e.resolves != 0 || len(res.Notes) != 1 || res.Notes[0] != notInstalledNote || res.Package != AppPackageID {
		t.Fatalf("%+v %v resolves=%d", res, err, e.resolves)
	}
	if _, statErr := os.Stat(home); !os.IsNotExist(statErr) {
		t.Fatalf("home was created: %v", statErr)
	}
}
