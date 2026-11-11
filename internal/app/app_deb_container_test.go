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
	realExec  bool   // run the elevated script for real, with a stub apt-get
	aptStub   string // the stub's path
	beforeRun func() // runs just before the elevated command
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
			if e.beforeRun != nil {
				e.beforeRun()
			}
			if e.onRun != nil {
				e.onRun(name, args)
			}
			if e.realExec {
				real := append([]string{}, args[1:]...)
				real[len(real)-1] = e.aptStub // the last positional argument is the apt-get path
				return runAppCommand(context.Background(), time.Minute, "", nil, args[0], real...)
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

func TestAppDebInstallRunsTheFixedScriptWithVerifiedArguments(t *testing.T) {
	e := newDebEnv(t)
	e.onRun = func(string, []string) { e.installed, e.version = true, "0.2.0" }
	_, tx, err := e.place()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Close(context.Background()) }()
	got := e.calls[0]
	want := []string{"/usr/bin/pkexec", "/bin/sh", "-c", debInstallScript, "festival-app-install", e.artifact, e.rel.Artifact.Sha256, "/usr/bin/apt-get"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("elevated command = %q\nwant %q", got, want)
	}
	if e.tempEntries() != 0 {
		t.Fatal("the happy path must not leave a user-owned copy for root to read")
	}
}

// writeAptStub makes a stand-in apt-get that records the file it was asked to install.
func (e *debEnv) writeAptStub(t *testing.T) (marker string) {
	t.Helper()
	dir := t.TempDir()
	marker = filepath.Join(dir, "apt-ran")
	e.aptStub = filepath.Join(dir, "apt-get")
	script := "#!/bin/sh\nset -eu\n[ \"$1 $2\" = \"install --yes\" ]\nstat -c '%a' \"$(dirname \"$3\")\" > " + marker + "\ncat \"$3\" >> " + marker + "\n"
	if err := os.WriteFile(e.aptStub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	e.realExec = true
	return marker
}

func TestAppDebElevatedScriptInstallsFromARootOwnedCopy(t *testing.T) {
	e := newDebEnv(t)
	marker := e.writeAptStub(t)
	scratch := t.TempDir()
	t.Setenv("TMPDIR", scratch)
	e.onRun = func(string, []string) { e.installed, e.version = true, "0.2.0" }
	_, tx, err := e.place()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Close(context.Background()) }()
	got, rerr := os.ReadFile(marker)
	if rerr != nil || string(got) != "700\ndeb payload" {
		t.Fatalf("apt-get saw %q (%v), want a 0700 directory and the verified bytes", got, rerr)
	}
	if entries, _ := os.ReadDir(scratch); len(entries) != 0 {
		t.Fatalf("the script left %d entries in its temp dir", len(entries))
	}
}

func TestAppDebSwapBeforeTheElevatedStepIsRefused(t *testing.T) {
	e := newDebEnv(t)
	marker := e.writeAptStub(t)
	scratch := t.TempDir()
	t.Setenv("TMPDIR", scratch)
	// The user-owned file is replaced after it was verified and before root reads it.
	e.beforeRun = func() {
		if err := os.WriteFile(e.artifact, []byte("different bytes"), 0o600); err != nil {
			t.Error(err)
		}
	}
	_, tx, err := e.place()
	if errpkg.Code(err) != "E_APP_SIGNATURE" || e.writes != 0 {
		t.Fatalf("err=%v writes=%d", err, e.writes)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatal("apt-get ran on bytes that did not match the signed hash")
	}
	if entries, _ := os.ReadDir(scratch); len(entries) != 0 {
		t.Fatalf("the script left %d entries behind", len(entries))
	}
	e.assertNextInstallStarts(tx)
}

func TestAppDebPrunesStaleKeptCopies(t *testing.T) {
	e := newDebEnv(t)
	for _, n := range []string{"festival-app-deb-old", "festival-app-deb-recent", "unrelated-old"} {
		if err := os.MkdirAll(filepath.Join(e.tempRoot, n), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-25 * time.Hour)
	for _, n := range []string{"festival-app-deb-old", "unrelated-old"} {
		if err := os.Chtimes(filepath.Join(e.tempRoot, n), old, old); err != nil {
			t.Fatal(err)
		}
	}
	e.res = appCommandResult{ExitCode: 126}
	_, tx, _ := e.place() // the dismissed prompt keeps a fresh copy and prunes the stale ones
	defer func() { _ = tx.Close(context.Background()) }()
	if _, err := os.Stat(filepath.Join(e.tempRoot, "festival-app-deb-old")); !os.IsNotExist(err) {
		t.Fatalf("stale kept copy not pruned: %v", err)
	}
	for _, n := range []string{"festival-app-deb-recent", "unrelated-old"} {
		if _, err := os.Stat(filepath.Join(e.tempRoot, n)); err != nil {
			t.Fatalf("%s must stay: %v", n, err)
		}
	}
}

func TestAppDebManualHintQuotesAPathWithSpaces(t *testing.T) {
	e := newDebEnv(t)
	spaced := filepath.Join(filepath.Dir(e.tempRoot), "tmp with space")
	if err := os.MkdirAll(spaced, 0o755); err != nil {
		t.Fatal(err)
	}
	e.d.tempRoot = spaced
	e.res = appCommandResult{ExitCode: 126}
	_, tx, err := e.place()
	defer func() { _ = tx.Close(context.Background()) }()
	if errpkg.Code(err) != "E_APP_ELEVATION" {
		t.Fatalf("err = %v", err)
	}
	entries, _ := os.ReadDir(spaced)
	if len(entries) != 1 {
		t.Fatalf("want one kept copy, got %d", len(entries))
	}
	files, _ := os.ReadDir(filepath.Join(spaced, entries[0].Name()))
	if len(files) != 1 {
		t.Fatalf("want one file in the kept copy, got %d", len(files))
	}
	quoted := "sudo apt-get install --yes '" + filepath.Join(spaced, entries[0].Name(), files[0].Name()) + "', then"
	if !strings.Contains(err.Error(), quoted) {
		t.Fatalf("hint does not quote the path:\n%v\nwant it to contain %s", err, quoted)
	}
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
	if len(e.calls) != 1 || strings.Join(e.calls[0][:3], " ") != "/usr/bin/pkexec /bin/sh -c" {
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

func TestAppDebMissingPkexecKeepsCopyForManualInstall(t *testing.T) {
	e := newDebEnv(t)
	e.pkexec = false
	_, tx, err := e.place()
	if errpkg.Code(err) != "E_APP_ELEVATION" || !strings.Contains(err.Error(), "pkexec is not installed") || len(e.calls) != 0 {
		t.Fatalf("err=%v calls=%d", err, len(e.calls))
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
