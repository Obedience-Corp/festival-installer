package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/hosts/shared"
	"github.com/Obedience-Corp/festival-installer/internal/shellsetup"
	"github.com/Obedience-Corp/festival-installer/internal/source"
	"github.com/Obedience-Corp/festival-installer/internal/state"
	"github.com/Obedience-Corp/festival-installer/internal/state/lock"
	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
)

const uninstallLockTimeout = 30 * time.Second

// ErrOutsideManagedBin is returned when a receipt file escapes the managed bin dir.
var ErrOutsideManagedBin = errpkg.New("E_UNINSTALL_PATH", "receipt file resolves outside the managed bin dir")

// UninstallTarget removes festival suite or a plugin by CLI selector. The
// second value is a human warning, empty when nothing needs saying.
func UninstallTarget(ctx context.Context, target string) (UninstallResult, string, error) {
	res, warning, err := uninstallTarget(ctx, target)
	if err == nil && len(res.Removed) > 0 && (res.Package == FestivalPackageID || res.Package == ObeyPackageID) {
		warning = appendWarning(warning, IntegrationNote(integrationAfterInstall(ctx, false)))
	}
	return res, warning, err
}

func uninstallTarget(ctx context.Context, target string) (UninstallResult, string, error) {
	packageID := FestivalPackageID
	if strings.Contains(target, "/") {
		selected, err := ParsePluginTarget(target)
		if err != nil {
			return UninstallResult{}, "", err
		}
		packageID = selected.PackageID
	} else if host, name, ok := PluginHost(target); ok {
		// Uninstall resolves the package id from local marketplace metadata
		// only to find the receipt key; it does not install anything, so the
		// live-path default (no override) is the right, unsurprising choice.
		id, err := ResolvePluginPackageID(ctx, host, name, source.DefaultVerifyOptions(nil, false))
		if err != nil {
			return UninstallResult{}, "", err
		}
		packageID = id
	} else {
		switch target {
		case "festival", "camp", "fest":
		case "obey":
			return uninstallObey(ctx)
		default:
			return UninstallResult{}, "", errpkg.New("E_UNINSTALL_TARGET", "unknown uninstall target "+target+" (expected festival, camp, fest, obey, or a camp-*/fest-* plugin)")
		}
	}
	res, err := UninstallPackage(ctx, packageID)
	return res, "", err
}

// uninstallObey removes the obey product and, only for an install this hub
// owns, removes its user bridge hooks and unregisters its user service first.
//
// The receipt decides. Without one there is nothing to remove, so there is
// nothing to unregister either: an obey that reached the managed bin dir by
// another route, or one whose receipt was lost, still belongs to the user, and
// stopping its daemon and deleting its unit while reporting "nothing to
// uninstall" destroys state the caller never asked about. With a receipt the
// order is unregister first, or the supervisor is left pointing at a path that
// is about to vanish, and a failed unregister is reported rather than dropped:
// the unit may outlive the binary and the user is the one who has to clear it.
func uninstallObey(ctx context.Context) (UninstallResult, string, error) {
	if err := ctx.Err(); err != nil {
		return UninstallResult{}, "", errpkg.Wrap("E_UNINSTALL_CTX", err, "context cancelled")
	}
	if os.Geteuid() == 0 {
		return UninstallResult{}, "", errpkg.New("E_UNINSTALL_ROOT", "run festival uninstall obey as your normal user without sudo so provider hooks and the managed binary are removed under the same identity")
	}
	_, found, err := readReceipt(ctx, ObeyPackageID)
	if err != nil {
		return UninstallResult{}, "", err
	}
	if !found {
		res, uerr := UninstallPackage(ctx, ObeyPackageID)
		return res, "", uerr
	}
	warning := ""
	obeyPath, perr := obeyServicePath(ctx)
	if perr != nil {
		warning = "could not locate the managed obey to remove its provider hooks and service: " + perr.Error() +
			". Provider hooks and the service may need manual removal."
	} else {
		// Hooks embed this exact binary path. Remove them while the receipt-owned
		// executable still exists; resolving another Obey on PATH would remove
		// that installation's hooks instead.
		if _, err := shellsetup.Run(ctx, obeyPath, "bridge", "uninstall", "all", "--scope", "user"); err != nil {
			warning = "obey bridge uninstall failed: " + err.Error() +
				". Provider hooks may remain; reinstall Obey and run obey bridge uninstall all --scope user to retry."
		}
		if verr := runServiceVerb(ctx, obeyPath, serviceVerbUninstall); verr != nil {
			warning = appendWarning(warning, "obey service uninstall failed: "+verr.Error()+
				". The receipt-owned files were still removed; the service unit may remain registered.")
		}
	}
	res, uerr := UninstallPackage(ctx, ObeyPackageID)
	return res, warning, uerr
}

// UninstallPackage removes only receipt-owned files for packageID.
func UninstallPackage(ctx context.Context, packageID string) (UninstallResult, error) {
	if err := ctx.Err(); err != nil {
		return UninstallResult{}, errpkg.Wrap("E_UNINSTALL_CTX", err, "context cancelled")
	}
	origin, _ := DetectSuite(ctx)
	if packageID == FestivalPackageID && origin.Kind == OriginPackage {
		home, herr := state.Home(ctx)
		if herr == nil {
			if _, ok, _ := state.OpenDBIfExists(ctx, home); !ok && !origin.Dual {
				note := origin.Remove
				if note == "" {
					note = "use your package manager to remove the suite"
				}
				note += "; festival uninstall cannot remove package-owned files"
				return UninstallResult{Package: packageID, Note: note}, nil
			}
		}
	}
	if packageID == FestivalPackageID && origin.Kind == OriginLeftover && !managedToolchain(ctx) {
		note := "leftover binaries at " + origin.Prefix + "; festival uninstall cannot remove them; see " + docsInstall
		return UninstallResult{Package: packageID, Note: note}, nil
	}

	if packageID == FestivalPackageID {
		placement, selfPath, err := ResolveSelf(ctx)
		if err != nil {
			return UninstallResult{}, err
		}
		switch placement {
		case SelfManaged:
			return UninstallResult{}, errpkg.New("E_UNINSTALL_SELF",
				"the running festival binary at "+selfPath+" cannot delete itself. "+
					"Run uninstall from a festival binary outside the managed bin dir, "+
					"or delete the managed files manually after this process exits.")
		case SelfUnknown:
			return UninstallResult{}, errpkg.New("E_UNINSTALL_SELF",
				"could not determine whether the running festival binary at "+selfPath+
					" is the managed one; retry once the underlying error clears.")
		}
	}
	home, err := state.Home(ctx)
	if err != nil {
		return UninstallResult{}, err
	}
	binDir, err := state.BinDir(ctx)
	if err != nil {
		return UninstallResult{}, err
	}

	fl, err := lock.NewFileLock(home)
	if err != nil {
		return UninstallResult{}, err
	}
	rel, err := fl.Acquire(ctx, uninstallLockTimeout)
	if err != nil {
		return UninstallResult{}, err
	}
	defer func() { _ = rel() }()

	db, err := state.OpenDB(ctx, home)
	if err != nil {
		return UninstallResult{}, err
	}
	defer func() { _ = db.Close(ctx) }()

	rec, err := receipts.Get(ctx, db.Raw(), packageID)
	if errors.Is(err, receipts.ErrNotFound) {
		return UninstallResult{Package: packageID, Note: "not installed (no receipt); nothing to uninstall"}, nil
	}
	if err != nil {
		return UninstallResult{}, err
	}

	var pluginRoot string
	if packageID != FestivalPackageID {
		pluginName := packageID
		if i := strings.LastIndex(pluginName, "/"); i >= 0 {
			pluginName = pluginName[i+1:]
		}
		pluginRoot, err = pluginRuntimeRoot(pluginName)
		if err != nil {
			return UninstallResult{}, err
		}
	}

	// Validate the complete receipt before removing anything. A corrupt path
	// must not turn uninstall into a partial destructive operation.
	for _, f := range rec.OwnedFiles {
		if err := assertWithinManagedRoots(f.Path, binDir, pluginRoot); err != nil {
			return UninstallResult{}, err
		}
	}

	var removed []string
	for _, f := range rec.OwnedFiles {
		if err := shared.RemoveByRecord(ctx, f); err != nil {
			return UninstallResult{}, err
		}
		removed = append(removed, f.Path)
	}
	if pluginRoot != "" {
		pruneEmptyPluginDirs(rec.OwnedFiles, pluginRoot)
	}

	if err := receipts.Delete(ctx, db.Raw(), packageID); err != nil {
		return UninstallResult{}, err
	}
	return UninstallResult{Package: packageID, Removed: removed}, nil
}

func assertWithinManagedRoots(path, binDir, pluginRoot string) error {
	if resolvePath(filepath.Dir(path)) == resolvePath(binDir) {
		return nil
	}
	if pluginRoot != "" {
		rel, err := filepath.Rel(resolvePath(pluginRoot), resolvePath(path))
		if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return nil
		}
	}
	if pluginRoot == "" {
		return errpkg.Wrap("E_UNINSTALL_PATH", ErrOutsideManagedBin, path)
	}
	return errpkg.New("E_UNINSTALL_PATH", "receipt file resolves outside the managed bin and plugin asset directories: "+path)
}

func pruneEmptyPluginDirs(files []receipts.OwnedFile, root string) {
	for _, f := range files {
		dir := filepath.Dir(f.Path)
		for dir != root {
			rel, err := filepath.Rel(root, dir)
			if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
				break
			}
			if err := os.Remove(dir); err != nil {
				break
			}
			dir = filepath.Dir(dir)
		}
	}
	_ = os.Remove(root)
}
