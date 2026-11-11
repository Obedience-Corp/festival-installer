package app

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"

	"github.com/Obedience-Corp/festival-installer/internal/artifacts"
	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/installer"
	"github.com/Obedience-Corp/festival-installer/internal/metadata"
)

const appBundleName = "Festival App.app"

type bundlePlatform struct {
	systemApps, userApps string                                  // "/Applications", "$HOME/Applications"
	uid                  int                                     // os.Getuid()
	owner                func(path string) (int, error)          // darwin: os.Stat(path).Sys().(*syscall.Stat_t).Uid
	writable             func(path string) bool                  // darwin: unix.Access(path, unix.W_OK) == nil
	info                 func(bundle string) (bundleInfo, error) // readBundleInfo
	codesign             func(ctx context.Context, bundle string) error
}

// newBundlePlatform builds the production bundle platform; the darwin file
// supplies the OS probes.
func newBundlePlatform(systemApps, userApps string, uid int, owner func(string) (int, error), writable func(string) bool, codesign func(context.Context, string) error) bundlePlatform {
	return bundlePlatform{systemApps: systemApps, userApps: userApps, uid: uid, owner: owner, writable: writable, info: readBundleInfo, codesign: codesign}
}

// ownedByThisAccount: owner(bundle) == uid, writable(bundle) and writable(filepath.Dir(bundle)).
func (b bundlePlatform) ownedByThisAccount(bundle string) bool {
	uid, err := b.owner(bundle)
	return err == nil && uid == b.uid && b.writable(bundle) && b.writable(filepath.Dir(bundle))
}

func (b bundlePlatform) format(_ context.Context, requested string, existing *appReceipt) (string, error) {
	if requested != "" {
		return "", errpkg.New("E_APP_FORMAT_UNSUPPORTED", "--format applies to Linux only")
	}
	if existing != nil && existing.Format != metadata.ArtifactKindAppBundle {
		return "", errpkg.New("E_APP_FORMAT_UNSUPPORTED", "the existing Festival App receipt is format "+existing.Format+", not "+metadata.ArtifactKindAppBundle)
	}
	return metadata.ArtifactKindAppBundle, nil
}

func (b bundlePlatform) candidates(existing *appReceipt) []string {
	var out []string
	if existing != nil && filepath.IsAbs(existing.Location) && filepath.Clean(existing.Location) == existing.Location && filepath.Base(existing.Location) == appBundleName {
		out = append(out, existing.Location)
	}
	return append(out, filepath.Join(b.systemApps, appBundleName), filepath.Join(b.userApps, appBundleName))
}

func (b bundlePlatform) discover(_ context.Context, _ string, existing *appReceipt, _ appRelease) (appCopy, error) {
	var other string
	seen := map[string]bool{}
	for _, path := range b.candidates(existing) {
		if seen[path] {
			continue
		}
		seen[path] = true
		fi, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return appCopy{}, errpkg.Wrap("E_APP_PROBE", err, "inspect "+path)
		}
		if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
			return appCopy{}, foreignAppError(path, "not an app bundle directory")
		}
		info, err := b.info(path)
		if err != nil {
			return appCopy{}, foreignAppError(path, "no readable Info.plist")
		}
		if info.ID != AppBundleID {
			return appCopy{}, foreignAppError(path, "bundle id "+info.ID)
		}
		if !b.ownedByThisAccount(path) {
			if other == "" {
				other = path
			}
			continue
		}
		return appCopy{
			Found: true, FromReceipt: existing != nil && path == existing.Location, Location: path, Version: info.ShortVersion,
			UpdatedByApp: existing != nil && installer.VersionLess(existing.Version, info.ShortVersion), OtherAccount: other,
		}, nil
	}
	return appCopy{OtherAccount: other}, nil
}

func (b bundlePlatform) target(_ string, cp appCopy) (string, error) {
	switch {
	case cp.Found:
		return cp.Location, nil
	case cp.OtherAccount != "":
		return filepath.Join(b.userApps, appBundleName), nil
	case b.writable(b.systemApps):
		return filepath.Join(b.systemApps, appBundleName), nil
	}
	return filepath.Join(b.userApps, appBundleName), nil
}

func (b bundlePlatform) tildePath(p string) string {
	home := filepath.Dir(b.userApps)
	if rest, ok := strings.CutPrefix(p, home+string(filepath.Separator)); ok {
		return "~/" + rest
	}
	return p
}

func bundleInvalid(msg string) error { return errpkg.New("E_APP_BUNDLE_INVALID", msg) }

func (b bundlePlatform) place(ctx context.Context, txn appTxn, req appPlaceRequest) (appPlacement, error) {
	rel, cp := req.Release, req.Copy
	dest, err := b.target(rel.Format, cp)
	if err != nil {
		return appPlacement{}, err
	}
	var notes []string
	if !cp.Found && filepath.Dir(dest) == b.userApps {
		if err := os.MkdirAll(b.userApps, 0o755); err != nil {
			return appPlacement{}, errpkg.Wrap("E_INSTALL_STAGING", err, "create "+b.userApps)
		}
		if cp.OtherAccount != "" {
			notes = append(notes, "Festival App in "+filepath.Dir(cp.OtherAccount)+" belongs to another account; installed for this account in "+b.tildePath(dest))
		} else {
			notes = append(notes, "Installed to ~/Applications because this account cannot write to "+b.systemApps+".")
		}
	}
	payload, err := txn.Prepare(ctx, installer.AppPlacement{PackageID: AppPackageID, Version: rel.Version, Format: metadata.ArtifactKindAppBundle, ArtifactSHA256: rel.Artifact.Sha256, Dest: dest})
	if err != nil {
		return appPlacement{}, err
	}
	stage := filepath.Dir(payload)
	if err := artifacts.ExtractTarGz(ctx, req.Artifact.Path, stage); err != nil {
		return appPlacement{}, err
	}
	if err := b.checkStaged(ctx, stage, payload, dest, rel); err != nil {
		return appPlacement{}, err
	}
	if err := txn.MarkStaged(ctx); err != nil {
		return appPlacement{}, err
	}
	if err := txn.Swap(ctx); err != nil {
		if os.IsPermission(err) || strings.Contains(err.Error(), "operation not permitted") {
			return appPlacement{}, errpkg.Wrap(errpkg.Code(err), err, "macOS blocked changing "+dest+". Allow your terminal under System Settings > Privacy & Security > App Management, then run the command again.")
		}
		return appPlacement{}, err
	}
	exe, err := appMainExecutable(metadata.ArtifactKindAppBundle, dest)
	if err != nil {
		return appPlacement{}, err
	}
	var mode os.FileMode = 0o755
	if fi, err := os.Stat(exe); err == nil {
		mode = fi.Mode().Perm()
	}
	placement := appPlacement{Location: dest, Executable: exe, Mode: mode, Notes: notes}
	leftover, err := txn.Commit(ctx, func(ctx context.Context, db *sql.DB) error { return req.WriteReceipt(ctx, db, placement) })
	if err != nil {
		return appPlacement{}, err
	}
	if leftover != "" {
		placement.Notes = append(placement.Notes, "Could not remove the previous copy at "+leftover+"; festival retries on its next run.")
	}
	return placement, nil
}

// checkStaged requires exactly one root named like the target, matching the
// signed manifest, and a passing codesign verification.
func (b bundlePlatform) checkStaged(ctx context.Context, stage, payload, dest string, rel appRelease) error {
	entries, err := os.ReadDir(stage)
	if err != nil {
		return errpkg.Wrap("E_APP_BUNDLE_INVALID", err, "read the extracted archive")
	}
	if len(entries) != 1 || !entries[0].IsDir() || entries[0].Name() != filepath.Base(dest) {
		return bundleInvalid("the archive must hold exactly one root named " + filepath.Base(dest))
	}
	info, err := b.info(payload)
	if err != nil {
		return errpkg.Wrap("E_APP_BUNDLE_INVALID", err, "read the new bundle's Info.plist")
	}
	if info.ID != rel.BundleID || info.ShortVersion != rel.Version || info.Executable != "festival-app" {
		return bundleInvalid("the new bundle (" + info.ID + " " + info.ShortVersion + " " + info.Executable + ") does not match the signed manifest (" + rel.BundleID + " " + rel.Version + " festival-app)")
	}
	if err := b.codesign(ctx, payload); err != nil {
		if errpkg.Code(err) == "E_APP_BUNDLE_INVALID" {
			return err
		}
		return errpkg.Wrap("E_APP_BUNDLE_INVALID", err, "codesign verification failed")
	}
	return nil
}
