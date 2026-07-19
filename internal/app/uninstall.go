package app

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
	"github.com/Obedience-Corp/obey-installer/internal/hosts/shared"
	"github.com/Obedience-Corp/obey-installer/internal/state"
	"github.com/Obedience-Corp/obey-installer/internal/state/lock"
	"github.com/Obedience-Corp/obey-installer/internal/state/receipts"
)

const uninstallLockTimeout = 30 * time.Second

// ErrOutsideManagedBin is returned when a receipt file escapes the managed bin dir.
var ErrOutsideManagedBin = errpkg.New("E_UNINSTALL_PATH", "receipt file resolves outside the managed bin dir")

// UninstallTarget removes festival suite or a plugin by CLI selector.
func UninstallTarget(ctx context.Context, target string) (UninstallResult, error) {
	packageID := FestivalPackageID
	if host, name, ok := PluginHost(target); ok {
		id, err := ResolvePluginPackageID(ctx, host, name)
		if err != nil {
			return UninstallResult{}, err
		}
		packageID = id
	} else {
		switch target {
		case "festival", "camp", "fest":
		default:
			return UninstallResult{}, errpkg.New("E_UNINSTALL_TARGET", "unknown uninstall target "+target+" (expected festival, camp, fest, or a camp-*/fest-* plugin)")
		}
	}
	return UninstallPackage(ctx, packageID)
}

// UninstallPackage removes only receipt-owned files for packageID.
func UninstallPackage(ctx context.Context, packageID string) (UninstallResult, error) {
	if err := ctx.Err(); err != nil {
		return UninstallResult{}, errpkg.Wrap("E_UNINSTALL_CTX", err, "context cancelled")
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

	var removed []string
	for _, f := range rec.OwnedFiles {
		if err := assertWithinManagedBin(f.Path, binDir); err != nil {
			return UninstallResult{}, err
		}
		if err := shared.RemoveByRecord(ctx, f); err != nil {
			return UninstallResult{}, err
		}
		removed = append(removed, f.Path)
	}

	if err := receipts.Delete(ctx, db.Raw(), packageID); err != nil {
		return UninstallResult{}, err
	}
	return UninstallResult{Package: packageID, Removed: removed}, nil
}

func assertWithinManagedBin(path, binDir string) error {
	if resolvePath(filepath.Dir(path)) != resolvePath(binDir) {
		return errpkg.Wrap("E_UNINSTALL_PATH", ErrOutsideManagedBin, path)
	}
	return nil
}
