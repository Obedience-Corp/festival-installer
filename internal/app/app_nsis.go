package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/installer"
	"github.com/Obedience-Corp/festival-installer/internal/metadata"
)

const (
	nsisMainBinary   = "festival-app.exe"
	nsisSetupTimeout = 10 * time.Minute
)

type nsisPlatform struct {
	readReg      func() (nsisRegistration, bool, error) // production: readNSISRegistration(nsisUninstallKey)
	run          appCommandRunner
	localAppData string // %LOCALAPPDATA%
}

// nsisPolicyErrnos are the OS errors Windows returns when an application
// control policy (Smart App Control) refuses to start an unsigned binary.
var nsisPolicyErrnos = map[syscall.Errno]bool{225: true, 786: true, 1260: true, 4551: true}

// nsisInstallArgs: a silent install, plus /UPDATE over an existing install,
// the flag the App's own updater adds.
func nsisInstallArgs(update bool) []string {
	if update {
		return []string{"/S", "/UPDATE"}
	}
	return []string{"/S"}
}

func nsisExitError(code int) error {
	if code == 0 {
		return nil
	}
	msg := fmt.Sprintf("the Festival App installer exited %d", code)
	if code == 1 || code == 2 {
		msg += " (it stopped before finishing, for example because a newer version is installed or the App could not be closed)"
	}
	return errpkg.New("E_APP_INSTALLER_FAILED", msg)
}

func nsisStartError(err error) error {
	var errno syscall.Errno
	if errors.As(err, &errno) && nsisPolicyErrnos[errno] {
		return errpkg.Wrap("E_APP_INSTALLER_FAILED", err, fmt.Sprintf("Windows blocked the Festival App installer (%v). Smart App Control or an application control policy blocks unsigned installers, and festival cannot install the App while it is on.", err))
	}
	return errpkg.Wrap("E_APP_INSTALLER_FAILED", err, "could not start the Festival App installer")
}

func sameInstallDir(a, b string) bool {
	return strings.EqualFold(strings.TrimRight(a, `\/`), strings.TrimRight(b, `\/`))
}

func (p nsisPlatform) format(_ context.Context, requested string, existing *appReceipt) (string, error) {
	if requested != "" {
		return "", errpkg.New("E_APP_FORMAT_UNSUPPORTED", "--format applies to Linux only")
	}
	if existing != nil && existing.Format != metadata.ArtifactKindNSIS {
		return "", errpkg.New("E_APP_FORMAT_UNSUPPORTED", "the existing Festival App receipt is format "+existing.Format+", not "+metadata.ArtifactKindNSIS)
	}
	return metadata.ArtifactKindNSIS, nil
}

func (p nsisPlatform) discover(_ context.Context, _ string, existing *appReceipt, _ appRelease) (appCopy, error) {
	reg, ok, err := p.readReg()
	if err != nil {
		return appCopy{}, err
	}
	if !ok {
		return appCopy{}, nil
	}
	if reg.MainBinaryName != "" && !strings.EqualFold(reg.MainBinaryName, nsisMainBinary) {
		return appCopy{}, foreignAppError(reg.InstallLocation, "registered main binary "+reg.MainBinaryName)
	}
	if reg.InstallLocation == "" || !filepath.IsAbs(reg.InstallLocation) {
		return appCopy{}, foreignAppError(nsisUninstallKey, "no absolute InstallLocation")
	}
	return appCopy{
		Found: true, Location: reg.InstallLocation, Version: reg.DisplayVersion,
		FromReceipt:  existing != nil && sameInstallDir(existing.Location, reg.InstallLocation),
		UpdatedByApp: existing != nil && installer.VersionLess(existing.Version, reg.DisplayVersion),
	}, nil
}

func (p nsisPlatform) target(_ string, cp appCopy) (string, error) {
	if cp.Found {
		return cp.Location, nil
	}
	return filepath.Join(p.localAppData, "Festival App"), nil
}

func (p nsisPlatform) place(ctx context.Context, txn appTxn, req appPlaceRequest) (appPlacement, error) {
	rel, cp := req.Release, req.Copy
	dest, err := p.target(rel.Format, cp)
	if err != nil {
		return appPlacement{}, err
	}
	err = txn.RecordExternal(ctx, installer.AppPlacement{PackageID: AppPackageID, Version: rel.Version, Format: metadata.ArtifactKindNSIS, ArtifactSHA256: rel.Artifact.Sha256, Dest: dest})
	if err != nil {
		return appPlacement{}, err
	}
	setup := req.Artifact.Path
	if !strings.HasSuffix(strings.ToLower(setup), ".exe") {
		renamed := filepath.Join(req.Artifact.Dir, "festival-app-setup.exe")
		if err := os.Rename(setup, renamed); err != nil {
			return appPlacement{}, errpkg.Wrap("E_APP_INSTALLER_FAILED", err, "name the installer with an .exe extension")
		}
		setup = renamed
	}
	res, err := p.run(ctx, nsisSetupTimeout, "", req.Output, setup, nsisInstallArgs(cp.Found)...)
	if err != nil {
		if errpkg.Code(err) == "E_INSTALL_CTX" {
			return appPlacement{}, err
		}
		return appPlacement{}, nsisStartError(err)
	}
	if err := nsisExitError(res.ExitCode); err != nil {
		return appPlacement{}, err
	}
	reg, ok, err := p.readReg()
	if err != nil {
		return appPlacement{}, err
	}
	exeStat, statErr := os.Stat(filepath.Join(reg.InstallLocation, nsisMainBinary))
	switch {
	case !ok:
		return appPlacement{}, nsisPostCheck("it did not register Festival App")
	case reg.DisplayVersion != rel.Version:
		return appPlacement{}, nsisPostCheck("it registered version " + reg.DisplayVersion + " instead of " + rel.Version)
	case statErr != nil || !exeStat.Mode().IsRegular():
		return appPlacement{}, nsisPostCheck(nsisMainBinary + " is missing from " + reg.InstallLocation)
	}
	exe, err := appMainExecutable(metadata.ArtifactKindNSIS, reg.InstallLocation)
	if err != nil {
		return appPlacement{}, err
	}
	placement := appPlacement{Location: reg.InstallLocation, Executable: exe, Mode: exeStat.Mode().Perm()}
	if _, err := txn.Commit(ctx, func(ctx context.Context, db *sql.DB) error { return req.WriteReceipt(ctx, db, placement) }); err != nil {
		return appPlacement{}, err
	}
	return placement, nil
}

func nsisPostCheck(what string) error {
	return errpkg.New("E_APP_INSTALLER_FAILED", "the Festival App installer exited 0 but "+what)
}
