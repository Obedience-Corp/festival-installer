//go:build container_fs && linux

package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/installer"
)

type debEnv struct {
	t         *testing.T
	home      string
	db        *sql.DB
	tempRoot  string
	exe       string
	artifact  string
	rel       appRelease
	installed bool
	version   string
	calls     [][]string
	res       appCommandResult
	runErr    error
	onRun     func(name string, args []string)
	writes    int
	pkexec    bool
	d         debPlacer
}

func newDebEnv(t *testing.T) *debEnv {
	t.Helper()
	root := t.TempDir()
	e := &debEnv{t: t, home: filepath.Join(root, "home"), tempRoot: filepath.Join(root, "tmp"), exe: filepath.Join(root, "festival-app"), pkexec: true}
	for _, d := range []string{e.home, e.tempRoot} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	e.db = openTestDB(t, e.home)
	body := []byte("deb payload")
	e.artifact = filepath.Join(root, "download.deb")
	if err := os.WriteFile(e.artifact, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(e.exe, []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	e.rel = appRelease{PackageID: AppPackageID, Version: "0.2.0", Channel: "stable", Source: "official-obey", Format: "deb", BundleID: AppBundleID}
	e.rel.Artifact.Sha256, e.rel.Artifact.Filename = hex.EncodeToString(sum[:]), "Festival.App_0.2.0_amd64.deb"
	e.d = debPlacer{
		run: func(_ context.Context, _ time.Duration, _ string, _ io.Writer, name string, args ...string) (appCommandResult, error) {
			e.calls = append(e.calls, append([]string{name}, args...))
			if e.onRun != nil {
				e.onRun(name, args)
			}
			return e.res, e.runErr
		},
		probe: func(context.Context) (debInstall, error) {
			return debInstall{Installed: e.installed, Version: e.version}, nil
		},
		exists:     func(string) bool { return e.pkexec },
		tempRoot:   e.tempRoot,
		executable: e.exe,
	}
	return e
}

func (e *debEnv) place() (appPlacement, appTxn, error) {
	e.t.Helper()
	tx, err := beginAppTxn(context.Background(), e.db, e.home)
	if err != nil {
		e.t.Fatal(err)
	}
	p, err := e.d.place(context.Background(), tx, appPlaceRequest{
		Release: e.rel, Artifact: verifiedAppArtifact{Path: e.artifact, Dir: filepath.Dir(e.artifact), Release: e.rel}, Output: io.Discard,
		WriteReceipt: func(context.Context, *sql.DB, appPlacement) error { e.writes++; return nil },
	})
	return p, tx, err
}

func (e *debEnv) tempEntries() int {
	entries, _ := os.ReadDir(e.tempRoot)
	return len(entries)
}

// assertNextInstallStarts closes tx, then proves the external entry is gone.
func (e *debEnv) assertNextInstallStarts(tx appTxn) {
	e.t.Helper()
	if err := tx.Close(context.Background()); err != nil {
		e.t.Fatalf("Close: %v", err)
	}
	next, err := installer.BeginApp(context.Background(), e.db, e.home)
	if err != nil {
		e.t.Fatalf("next BeginApp: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(e.home, installer.AppIntentName)); !os.IsNotExist(statErr) {
		e.t.Fatalf("app-intent.json still present: %v", statErr)
	}
	if err := next.Close(context.Background()); err != nil {
		e.t.Fatal(err)
	}
}

func TestAppDebCopyIsReadableAndRehashed(t *testing.T) {
	e := newDebEnv(t)
	var debPath string
	e.onRun = func(_ string, args []string) {
		debPath = args[len(args)-1]
		fi, err := os.Stat(debPath)
		if err != nil || fi.Mode().Perm() != 0o644 {
			t.Errorf("deb mode: %v %v", fi, err)
		}
		di, err := os.Stat(filepath.Dir(debPath))
		if err != nil || di.Mode().Perm() != 0o755 {
			t.Errorf("dir mode: %v %v", di, err)
		}
		b, _ := os.ReadFile(debPath)
		sum := sha256.Sum256(b)
		if hex.EncodeToString(sum[:]) != e.rel.Artifact.Sha256 {
			t.Error("copy hash differs")
		}
		e.installed, e.version = true, "0.2.0"
	}
	_, tx, err := e.place()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Close(context.Background()) }()
	if !strings.HasPrefix(debPath, e.tempRoot+string(filepath.Separator)) {
		t.Fatalf("copy %s is not under %s", debPath, e.tempRoot)
	}
}

func TestAppDebHashMismatchStopsBeforePkexec(t *testing.T) {
	e := newDebEnv(t)
	if err := os.WriteFile(e.artifact, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, tx, err := e.place()
	if errpkg.Code(err) != "E_APP_SIGNATURE" || len(e.calls) != 0 || e.tempEntries() != 0 {
		t.Fatalf("err=%v calls=%d temp=%d", err, len(e.calls), e.tempEntries())
	}
	e.assertNextInstallStarts(tx)
}

func TestAppDebInstallCommitsAfterProbe(t *testing.T) {
	e := newDebEnv(t)
	e.onRun = func(string, []string) { e.installed, e.version = true, "0.2.0" }
	p, tx, err := e.place()
	if err != nil {
		t.Fatal(err)
	}
	if p.Location != "dpkg:festival-app" || p.Executable != e.exe || e.writes != 1 || e.tempEntries() != 0 {
		t.Fatalf("%+v writes=%d temp=%d", p, e.writes, e.tempEntries())
	}
	if len(e.calls) != 1 || strings.Join(e.calls[0][:4], " ") != "/usr/bin/pkexec /usr/bin/apt-get install --yes" {
		t.Fatalf("calls = %v", e.calls)
	}
	e.assertNextInstallStarts(tx)
}

func TestAppDebVersionMismatchAfterInstall(t *testing.T) {
	e := newDebEnv(t)
	e.onRun = func(string, []string) { e.installed, e.version = true, "0.1.7" }
	_, tx, err := e.place()
	if errpkg.Code(err) != "E_APP_INSTALLER_FAILED" || e.writes != 0 || !strings.Contains(err.Error(), "0.1.7") {
		t.Fatalf("err=%v writes=%d", err, e.writes)
	}
	e.assertNextInstallStarts(tx)
}

func TestAppDebElevationFailureKeepsCopyForManualInstall(t *testing.T) {
	e := newDebEnv(t)
	e.res = appCommandResult{ExitCode: 126}
	_, tx, err := e.place()
	if errpkg.Code(err) != "E_APP_ELEVATION" {
		t.Fatalf("err = %v", err)
	}
	const marker = "sudo apt-get install --yes "
	i := strings.Index(err.Error(), marker)
	if i < 0 {
		t.Fatalf("no manual hint: %v", err)
	}
	path := strings.TrimSuffix(strings.Fields(err.Error()[i+len(marker):])[0], ",")
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("the path in the hint does not exist: %v", statErr)
	}
	e.assertNextInstallStarts(tx)
}

func TestAppDebAptFailureDoesNotBlockNextInstall(t *testing.T) {
	e := newDebEnv(t)
	e.res = appCommandResult{ExitCode: 100, Stderr: "E: Some warning\nE: Unable to locate package libwebkit2gtk-4.1-0\n"}
	_, tx, err := e.place()
	if errpkg.Code(err) != "E_APP_INSTALLER_FAILED" || !strings.HasSuffix(err.Error(), "E: Unable to locate package libwebkit2gtk-4.1-0") || e.tempEntries() != 0 {
		t.Fatalf("err=%v temp=%d", err, e.tempEntries())
	}
	e.assertNextInstallStarts(tx)
}

func TestAppDebMissingPkexecNeverCopies(t *testing.T) {
	e := newDebEnv(t)
	e.pkexec = false
	_, tx, err := e.place()
	if errpkg.Code(err) != "E_APP_ELEVATION" || !strings.Contains(err.Error(), "pkexec is not installed") || e.tempEntries() != 0 || len(e.calls) != 0 {
		t.Fatalf("err=%v temp=%d calls=%d", err, e.tempEntries(), len(e.calls))
	}
	e.assertNextInstallStarts(tx)
}
