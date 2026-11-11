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
)

type debPlacer struct {
	run        appCommandRunner                              // pkexec apt-get
	probe      func(ctx context.Context) (debInstall, error) // production: probeDeb with its runner
	exists     func(string) bool
	tempRoot   string // os.TempDir() in production
	executable string // "/usr/bin/festival-app" in production; tests point it at a temp file
}

func debInstallArgv(debPath string) []string {
	return []string{pkexecPath, aptGetPath, "install", "--yes", debPath}
}

// debElevationError maps a pkexec start failure or exit code to the D008 errors.
// Exit 0 with no start error returns nil.
func debElevationError(exitCode int, startErr error, debPath string) error {
	hint := " " + fmt.Sprintf(debElevateHint, debPath)
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
	dir, err = os.MkdirTemp(d.tempRoot, "festival-app-deb-")
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
		return appPlacement{}, debElevationError(-1, exec.ErrNotFound, req.Artifact.Path)
	}
	err := txn.RecordExternal(ctx, installer.AppPlacement{PackageID: AppPackageID, Version: rel.Version, Format: metadata.ArtifactKindDeb, ArtifactSHA256: rel.Artifact.Sha256, Dest: debLocation})
	if err != nil {
		return appPlacement{}, err
	}
	dir, dst, err := d.copyDeb(ctx, req)
	if err != nil {
		return appPlacement{}, err
	}
	argv := debInstallArgv(dst)
	res, err := d.run(ctx, debAptTimeout, "", req.Output, argv[0], argv[1:]...)
	switch {
	case err != nil && errpkg.Code(err) == "E_INSTALL_CTX":
		_ = os.RemoveAll(dir)
		return appPlacement{}, err
	case err != nil:
		return appPlacement{}, debElevationError(-1, err, dst) // the copy stays for the manual install
	case res.ExitCode == 126 || res.ExitCode == 127:
		return appPlacement{}, debElevationError(res.ExitCode, nil, dst)
	case res.ExitCode != 0:
		_ = os.RemoveAll(dir)
		return appPlacement{}, errpkg.New("E_APP_INSTALLER_FAILED", fmt.Sprintf("apt-get exited %d: %s", res.ExitCode, lastLine(res.Stderr)))
	}
	_ = os.RemoveAll(dir)

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
