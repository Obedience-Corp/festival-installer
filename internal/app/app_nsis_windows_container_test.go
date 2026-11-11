//go:build windows && container_fs

package app

import (
	"context"
	"database/sql"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/installer"
	"github.com/Obedience-Corp/festival-installer/internal/source"
)

const (
	fakeNSISKey    = `Software\FestivalInstallerTest\Uninstall\Festival App`
	envFakeNSIS    = "FESTIVAL_TEST_FAKE_NSIS"
	envFakeDir     = "FESTIVAL_TEST_FAKE_NSIS_DIR"
	envFakeVersion = "FESTIVAL_TEST_FAKE_NSIS_VERSION"
	envFakeExit    = "FESTIVAL_TEST_FAKE_NSIS_EXIT"
)

// init lets a copy of the test binary act as setup.exe. It runs before TestMain.
func init() {
	if os.Getenv(envFakeNSIS) != "install" {
		return
	}
	dir, exit := os.Getenv(envFakeDir), 0
	if v := os.Getenv(envFakeExit); v != "" {
		exit, _ = strconv.Atoi(v)
	}
	if exit == 0 {
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(filepath.Join(dir, "festival-app.exe"), []byte("fake app"), 0o755)
		_ = os.WriteFile(filepath.Join(dir, "fake-nsis-ran"), []byte(strings.Join(os.Args[1:], " ")), 0o644)
		writeFakeRegistration(dir, os.Getenv(envFakeVersion))
	}
	os.Exit(exit)
}

func writeFakeRegistration(dir, version string) {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, fakeNSISKey, registry.SET_VALUE)
	if err != nil {
		os.Exit(90)
	}
	defer func() { _ = k.Close() }()
	for name, v := range map[string]string{
		"DisplayName": "Festival App", "DisplayVersion": version, "MainBinaryName": "festival-app.exe",
		"InstallLocation": `"` + dir + `"`, "UninstallString": `"` + filepath.Join(dir, "uninstall.exe") + `"`,
	} {
		if err := k.SetStringValue(name, v); err != nil {
			os.Exit(91)
		}
	}
}

type recordingTxn struct {
	appTxn
	closeErr error
	closed   bool
}

func (r *recordingTxn) Close(ctx context.Context) error {
	r.closeErr, r.closed = r.appTxn.Close(ctx), true
	return r.closeErr
}

type nsisEnv struct {
	t        *testing.T
	root     string
	installD string
	setup    string
	store    *fakeAppStore
	ai       appInstaller
	txn      *recordingTxn
	runs     int
}

func newNSISEnv(t *testing.T, version string) *nsisEnv {
	t.Helper()
	root := t.TempDir()
	t.Setenv("FESTIVAL_HOME", filepath.Join(root, "home"))
	t.Cleanup(func() { _ = registry.DeleteKey(registry.CURRENT_USER, fakeNSISKey) })
	_ = registry.DeleteKey(registry.CURRENT_USER, fakeNSISKey)
	e := &nsisEnv{t: t, root: root, installD: filepath.Join(root, "Festival App"), store: &fakeAppStore{}}
	e.setup = filepath.Join(root, "Festival.App_"+version+"_x64-setup.exe")
	copyFile(t, os.Args[0], e.setup)
	t.Setenv(envFakeDir, e.installD)
	t.Setenv(envFakeVersion, version)
	t.Setenv(envFakeNSIS, "install")
	plat := nsisPlatform{
		readReg:      func() (nsisRegistration, bool, error) { return readNSISRegistration(fakeNSISKey) },
		run:          runAppCommand,
		localAppData: root,
	}
	e.ai = appInstaller{
		goos: "windows", euid: func() int { return 1 }, platform: plat,
		resolve: func(context.Context, string, string, source.VerifyOptions) (appRelease, error) {
			rel := appRelease{PackageID: AppPackageID, Version: version, Channel: "stable", Source: "official-obey", Format: "nsis", BundleID: AppBundleID}
			rel.Artifact.URL, rel.Artifact.Sha256, rel.Artifact.Filename = "https://example.invalid/setup.exe", strings.Repeat("a", 64), filepath.Base(e.setup)
			return rel, nil
		},
		download: func(_ context.Context, _ string, rel appRelease, _ appDownloadOptions) (verifiedAppArtifact, error) {
			e.runs++
			return verifiedAppArtifact{Path: e.setup, Dir: root, Release: rel}, nil
		},
		begin: func(ctx context.Context, db *sql.DB, home string) (appTxn, error) {
			tx, err := beginAppTxn(ctx, db, home)
			if err != nil {
				return nil, err
			}
			e.txn = &recordingTxn{appTxn: tx}
			return e.txn, nil
		},
		refuseRunning: refuseRunningApp, receipts: e.store.factory(),
	}
	return e
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

func (e *nsisEnv) install() (AppResult, error) {
	return e.ai.install(context.Background(), AppInstallOptions{})
}

func (e *nsisEnv) marker() string {
	b, _ := os.ReadFile(filepath.Join(e.installD, "fake-nsis-ran"))
	return string(b)
}

func TestAppNSISContainerInstallReadsRegistrationAndCommits(t *testing.T) {
	e := newNSISEnv(t, "0.2.0")
	res, err := e.install()
	if err != nil {
		t.Fatal(err)
	}
	wantExe := e.installD + `\festival-app.exe`
	if res.Location != e.installD || e.marker() != "/S" || e.store.r == nil || e.store.r.Executable != wantExe || e.store.r.Location != e.installD {
		t.Fatalf("res=%+v marker=%q receipt=%+v", res, e.marker(), e.store.r)
	}
}

func TestAppNSISContainerUpdatePassesUpdateFlag(t *testing.T) {
	e := newNSISEnv(t, "0.2.0")
	_ = os.MkdirAll(e.installD, 0o755)
	t.Setenv(envFakeVersion, "0.1.0")
	writeFakeRegistrationForTest(t, e.installD, "0.1.0")
	e.store.r = &appReceipt{Version: "0.1.0", Format: "nsis", Location: e.installD}
	t.Setenv(envFakeVersion, "0.2.0")
	if _, err := e.install(); err != nil {
		t.Fatal(err)
	}
	if e.marker() != "/S /UPDATE" {
		t.Fatalf("marker = %q", e.marker())
	}
}

func writeFakeRegistrationForTest(t *testing.T, dir, version string) {
	t.Helper()
	k, _, err := registry.CreateKey(registry.CURRENT_USER, fakeNSISKey, registry.SET_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = k.Close() }()
	for name, v := range map[string]string{
		"DisplayName": "Festival App", "DisplayVersion": version, "MainBinaryName": "festival-app.exe",
		"InstallLocation": `"` + dir + `"`, "UninstallString": `"` + filepath.Join(dir, "uninstall.exe") + `"`,
	} {
		if err := k.SetStringValue(name, v); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAppNSISContainerNonZeroExitIsInstallerFailed(t *testing.T) {
	e := newNSISEnv(t, "0.2.0")
	t.Setenv(envFakeExit, "2")
	_, err := e.install()
	if errpkg.Code(err) != "E_APP_INSTALLER_FAILED" || e.store.r != nil {
		t.Fatalf("err=%v receipt=%+v", err, e.store.r)
	}
	if e.txn == nil || !e.txn.closed || e.txn.closeErr != nil {
		t.Fatalf("Close was not called cleanly: %+v", e.txn)
	}
	home := os.Getenv("FESTIVAL_HOME")
	if _, statErr := os.Stat(filepath.Join(home, installer.AppIntentName)); !os.IsNotExist(statErr) {
		t.Fatalf("app-intent.json still present: %v", statErr)
	}
	db := openTestDB(t, home)
	tx, err := installer.BeginApp(context.Background(), db, home)
	if err != nil {
		t.Fatalf("next BeginApp: %v", err)
	}
	if err := tx.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestAppNSISContainerVersionMismatchIsInstallerFailed(t *testing.T) {
	e := newNSISEnv(t, "0.2.0")
	t.Setenv(envFakeVersion, "0.1.9")
	_, err := e.install()
	if errpkg.Code(err) != "E_APP_INSTALLER_FAILED" || !strings.Contains(err.Error(), "0.1.9") || e.store.r != nil {
		t.Fatalf("err=%v receipt=%+v", err, e.store.r)
	}
}

func TestAppNSISContainerRefusesRunningAppBeforeInstaller(t *testing.T) {
	e := newNSISEnv(t, "0.2.0")
	if err := os.MkdirAll(e.installD, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envFakeNSIS, "")
	exe := filepath.Join(e.installD, "festival-app.exe")
	copyFile(t, os.Args[0], exe)
	child := exec.Command(exe, "-test.run=^$")
	child.Env = append(os.Environ(), envAppTestHelper+"=sleep")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Process.Kill(); _, _ = child.Process.Wait() })
	writeFakeRegistrationForTest(t, e.installD, "0.1.0")
	e.store.r = &appReceipt{Version: "0.1.0", Format: "nsis", Location: e.installD}
	scope := appRunScope{Dirs: []string{e.installD}}
	deadline := time.Now().Add(15 * time.Second)
	for refuseRunningApp(context.Background(), scope) == nil {
		if time.Now().After(deadline) {
			t.Fatal("the scan never saw the child")
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Setenv(envFakeNSIS, "install")
	_ = os.Remove(filepath.Join(e.installD, "fake-nsis-ran"))
	_, err := e.install()
	if errpkg.Code(err) != "E_APP_RUNNING" || e.marker() != "" {
		t.Fatalf("err=%v marker=%q", err, e.marker())
	}
}
