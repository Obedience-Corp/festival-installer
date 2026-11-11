package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Obedience-Corp/festival-installer/internal/artifacts"
	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/hosts/shared"
	"github.com/Obedience-Corp/festival-installer/internal/installer"
	"github.com/Obedience-Corp/festival-installer/internal/metadata"
)

const (
	debPackage     = "festival-app"
	debLocation    = "dpkg:festival-app"
	pkexecPath     = "/usr/bin/pkexec"
	aptGetPath     = "/usr/bin/apt-get"
	debAptTimeout  = 9 * time.Minute
	debElevateHint = "Install the AppImage instead with: festival install app --format appimage. Or install the package yourself with: sudo apt-get install --yes %s, then run festival install app to record it."

	shPath          = "/bin/sh"
	debCopyPrefix   = "festival-app-deb-"
	debCopyStale    = 24 * time.Hour
	debExitMismatch = 97 // the script's exit code when the package hash differs
)

// debInstallScript is the one thing pkexec runs. It is a constant: the package
// path, the expected sha256 and the apt-get path arrive as positional arguments
// and are never spliced into the text. The package is copied into a root-owned
// 0700 directory, hashed there, and installed from there, so the bytes that are
// checked are the bytes root installs. The directory is removed on every exit.
const debInstallScript = `set -eu
src=$1
want=$2
apt=$3
dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT HUP INT TERM
chmod 700 "$dir"
cp -- "$src" "$dir/package.deb"
got=$(sha256sum "$dir/package.deb")
got=${got%% *}
if [ "$got" != "$want" ]; then
  echo "package sha256 differs from the verified value" >&2
  exit 97
fi
"$apt" install --yes "$dir/package.deb"
`

// shellQuote quotes s for a POSIX shell only when it needs it.
func shellQuote(s string) string {
	safe := s != ""
	for _, r := range s {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_./:@%+=,-", r)
		if !ok {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// pruneStaleDebCopies removes kept manual-install copies older than a day.
func pruneStaleDebCopies(root string, now time.Time) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), debCopyPrefix) {
			continue
		}
		if info, err := e.Info(); err == nil && now.Sub(info.ModTime()) > debCopyStale {
			_ = os.RemoveAll(filepath.Join(root, e.Name()))
		}
	}
}

type debPlacer struct {
	run        appCommandRunner                              // pkexec apt-get
	probe      func(ctx context.Context) (debInstall, error) // production: probeDeb with its runner
	exists     func(string) bool
	tempRoot   string // os.TempDir() in production
	executable string // "/usr/bin/festival-app" in production; tests point it at a temp file
}

// debInstallArgv is the whole elevated command: pkexec runs the fixed script
// with the verified download, its signed sha256 and the apt-get path.
func debInstallArgv(debPath, sha256 string) []string {
	return []string{pkexecPath, shPath, "-c", debInstallScript, "festival-app-install", debPath, strings.ToLower(sha256), aptGetPath}
}

// debElevationError maps a pkexec start failure or exit code to the D008 errors.
// Exit 0 with no start error returns nil.
func debElevationError(exitCode int, startErr error, debPath string) error {
	hint := " " + fmt.Sprintf(debElevateHint, shellQuote(debPath))
	switch {
	case startErr != nil && (errors.Is(startErr, exec.ErrNotFound) || errors.Is(startErr, fs.ErrNotExist)):
		return errpkg.New("E_APP_ELEVATION", "pkexec is not installed."+hint)
	case startErr != nil:
		return errpkg.Wrap("E_APP_INSTALLER_FAILED", startErr, "could not start pkexec")
	case exitCode == 126:
		return errpkg.New("E_APP_ELEVATION", "The password prompt was dismissed."+hint)
	case exitCode == 127:
		return errpkg.New("E_APP_ELEVATION", "No password prompt is available in this session."+hint)
	case exitCode != 0:
		return errpkg.New("E_APP_INSTALLER_FAILED", fmt.Sprintf("apt-get exited %d", exitCode))
	}
	return nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func (d debPlacer) discover(ctx context.Context, existing *appReceipt) (appCopy, error) {
	inst, err := d.probe(ctx)
	if err != nil {
		return appCopy{}, errpkg.Wrap("E_APP_PROBE", err, "ask dpkg about "+debPackage)
	}
	if !inst.Installed {
		return appCopy{}, nil
	}
	return appCopy{
		Found: true, Location: debLocation, Version: inst.Version,
		FromReceipt:  existing != nil && existing.Format == metadata.ArtifactKindDeb,
		UpdatedByApp: existing != nil && installer.VersionLess(existing.Version, inst.Version),
	}, nil
}

// copyDeb copies the verified package into a fresh world-readable directory so
// root and the _apt user can read it. It returns the directory and the copy.
func (d debPlacer) copyDeb(ctx context.Context, req appPlaceRequest) (dir, dst string, err error) {
	rel := req.Release
	pruneStaleDebCopies(d.tempRoot, time.Now())
	dir, err = os.MkdirTemp(d.tempRoot, debCopyPrefix)
	if err != nil {
		return "", "", errpkg.Wrap("E_APP_INSTALLER_FAILED", err, "create a directory for the package")
	}
	fail := func(e error) (string, string, error) { _ = os.RemoveAll(dir); return "", "", e }
	name := filepath.Base(rel.Artifact.Filename)
	if shared.ValidateSegment(name) != nil || !strings.HasSuffix(name, ".deb") {
		name = "festival-app.deb"
	}
	dst = filepath.Join(dir, name)
	src, err := os.Open(req.Artifact.Path)
	if err != nil {
		return fail(errpkg.Wrap("E_APP_INSTALLER_FAILED", err, "open the verified package"))
	}
	defer func() { _ = src.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return fail(errpkg.Wrap("E_APP_INSTALLER_FAILED", err, "create the package copy"))
	}
	if _, err := io.Copy(out, src); err != nil {
		_ = out.Close()
		return fail(errpkg.Wrap("E_APP_INSTALLER_FAILED", err, "copy the package"))
	}
	if err := out.Close(); err != nil {
		return fail(errpkg.Wrap("E_APP_INSTALLER_FAILED", err, "close the package copy"))
	}
	if err := os.Chmod(dst, 0o644); err != nil {
		return fail(errpkg.Wrap("E_APP_INSTALLER_FAILED", err, "chmod the package copy"))
	}
	if err := artifacts.VerifySHA256(ctx, dst, rel.Artifact.Sha256); err != nil {
		if ctx.Err() != nil {
			return fail(ctx.Err())
		}
		return fail(errpkg.Wrap("E_APP_SIGNATURE", err, "the verified package changed while it was copied"))
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		return fail(errpkg.Wrap("E_APP_INSTALLER_FAILED", err, "chmod the package directory"))
	}
	return dir, dst, nil
}

func (d debPlacer) place(ctx context.Context, txn appTxn, req appPlaceRequest) (appPlacement, error) {
	rel := req.Release
	if !d.exists(pkexecPath) {
		// The downloaded file is removed when this call returns, so the manual
		// hint must name a kept copy, as it does for the other elevation failures.
		_, kept, err := d.copyDeb(ctx, req)
		if err != nil {
			return appPlacement{}, err
		}
		return appPlacement{}, debElevationError(-1, exec.ErrNotFound, kept)
	}
	err := txn.RecordExternal(ctx, installer.AppPlacement{PackageID: AppPackageID, Version: rel.Version, Format: metadata.ArtifactKindDeb, ArtifactSHA256: rel.Artifact.Sha256, Dest: debLocation})
	if err != nil {
		return appPlacement{}, err
	}
	argv := debInstallArgv(req.Artifact.Path, rel.Artifact.Sha256)
	res, err := d.run(ctx, debAptTimeout, "", req.Output, argv[0], argv[1:]...)
	switch {
	case err != nil && errpkg.Code(err) == "E_INSTALL_CTX":
		return appPlacement{}, err
	case err != nil || res.ExitCode == 126 || res.ExitCode == 127:
		// The download is removed when this call returns, so the manual hint
		// names a verified copy that is kept for the user's own command.
		_, kept, cerr := d.copyDeb(ctx, req)
		if cerr != nil {
			return appPlacement{}, cerr
		}
		return appPlacement{}, debElevationError(res.ExitCode, err, kept)
	case res.ExitCode == debExitMismatch:
		return appPlacement{}, errpkg.New("E_APP_SIGNATURE", "the Festival App package changed after it was verified; nothing was installed")
	case res.ExitCode != 0:
		return appPlacement{}, errpkg.New("E_APP_INSTALLER_FAILED", fmt.Sprintf("apt-get exited %d: %s", res.ExitCode, lastLine(res.Stderr)))
	}

	inst, err := d.probe(ctx)
	if err != nil {
		return appPlacement{}, errpkg.Wrap("E_APP_PROBE", err, "ask dpkg about "+debPackage)
	}
	if !inst.Installed || inst.Version != rel.Version {
		got := "not installed"
		if inst.Installed {
			got = inst.Version
		}
		return appPlacement{}, errpkg.New("E_APP_INSTALLER_FAILED", "apt-get finished but dpkg lists "+debPackage+" "+got)
	}
	info, err := os.Stat(d.executable)
	if err != nil || !info.Mode().IsRegular() {
		return appPlacement{}, errpkg.New("E_APP_INSTALLER_FAILED", "apt-get finished but "+d.executable+" is missing")
	}
	placement := appPlacement{Location: debLocation, Executable: d.executable, Mode: info.Mode().Perm()}
	if _, err := txn.Commit(ctx, func(ctx context.Context, db *sql.DB) error { return req.WriteReceipt(ctx, db, placement) }); err != nil {
		return appPlacement{}, err
	}
	return placement, nil
}

const debRemoveHint = " Or remove it yourself with: sudo apt-get remove --yes festival-app, then run festival uninstall app to clear the record."

func debRemoveArgv() []string {
	return []string{pkexecPath, aptGetPath, "remove", "--yes", debPackage}
}

// debRemoveElevationError maps a pkexec start failure or exit code for uninstall.
func debRemoveElevationError(exitCode int, startErr error) error {
	switch {
	case startErr != nil && (errors.Is(startErr, exec.ErrNotFound) || errors.Is(startErr, fs.ErrNotExist)):
		return errpkg.New("E_APP_ELEVATION", "pkexec is not installed."+debRemoveHint)
	case startErr != nil:
		return errpkg.Wrap("E_APP_INSTALLER_FAILED", startErr, "could not start pkexec")
	case exitCode == 126:
		return errpkg.New("E_APP_ELEVATION", "The password prompt was dismissed."+debRemoveHint)
	case exitCode == 127:
		return errpkg.New("E_APP_ELEVATION", "No password prompt is available in this session."+debRemoveHint)
	}
	return nil
}

func (d debPlacer) remove(ctx context.Context, _ appReceipt, out io.Writer) (bool, []string, error) {
	inst, err := d.probe(ctx)
	if err != nil {
		return false, nil, errpkg.Wrap("E_APP_PROBE", err, "ask dpkg about "+debPackage)
	}
	if !inst.Installed {
		return false, []string{removedOutsideNote2}, nil
	}
	if !d.exists(pkexecPath) {
		return false, nil, debRemoveElevationError(-1, exec.ErrNotFound)
	}
	argv := debRemoveArgv()
	res, err := d.run(ctx, debAptTimeout, "", out, argv[0], argv[1:]...)
	if err != nil {
		if errpkg.Code(err) == "E_INSTALL_CTX" {
			return false, nil, err
		}
		return false, nil, debRemoveElevationError(-1, err)
	}
	if e := debRemoveElevationError(res.ExitCode, nil); e != nil {
		return false, nil, e
	}
	if res.ExitCode != 0 {
		return false, nil, errpkg.New("E_APP_INSTALLER_FAILED", fmt.Sprintf("apt-get exited %d: %s", res.ExitCode, lastLine(res.Stderr)))
	}
	after, err := d.probe(ctx)
	if err != nil {
		return false, nil, errpkg.Wrap("E_APP_PROBE", err, "ask dpkg about "+debPackage)
	}
	if after.Installed {
		return false, nil, errpkg.New("E_APP_INSTALLER_FAILED", "apt-get finished but dpkg still lists "+debPackage+" "+after.Version)
	}
	return true, nil, nil
}
