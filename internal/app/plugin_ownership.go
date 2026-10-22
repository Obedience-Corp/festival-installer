package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"github.com/Obedience-Corp/festival-installer/internal/artifacts"
	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/state"
	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
)

func pluginReceipt(ctx context.Context, packageID string) (receipts.Receipt, bool, error) {
	home, err := state.Home(ctx)
	if err != nil {
		return receipts.Receipt{}, false, err
	}
	db, exists, err := state.OpenDBIfExists(ctx, home)
	if err != nil || !exists {
		return receipts.Receipt{}, false, err
	}
	defer func() { _ = db.Close(ctx) }()
	receipt, err := receipts.Get(ctx, db.Raw(), packageID)
	if errors.Is(err, receipts.ErrNotFound) {
		return receipts.Receipt{}, false, nil
	}
	return receipt, err == nil, err
}

// checkPluginDestination never interprets a receipt row as permission to overwrite
// bytes the user replaced. Missing owned files can be repaired; foreign files stay.
func checkPluginDestination(ctx context.Context, path string, receipt receipts.Receipt) error {
	if err := ctx.Err(); err != nil {
		return errpkg.Wrap("E_PLUGIN_CTX", err, "check plugin ownership")
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errpkg.Wrap("E_PLUGIN_OWNERSHIP", err, "inspect "+path)
	}
	if !info.Mode().IsRegular() {
		return foreignPluginPath(path)
	}
	for _, file := range receipt.OwnedFiles {
		if filepath.Clean(file.Path) != filepath.Clean(path) {
			continue
		}
		if file.Hash == "" {
			return foreignPluginPath(path)
		}
		hash, err := artifacts.SHA256(ctx, path)
		if err != nil {
			return err
		}
		if hash == file.Hash {
			return nil
		}
		return foreignPluginPath(path)
	}
	return foreignPluginPath(path)
}

func foreignPluginPath(path string) error {
	return errpkg.New("E_PLUGIN_FOREIGN", "preserved "+path+": it is not the unchanged receipt-owned plugin file; update it through its existing owner or move it aside before retrying")
}

func pluginReceiptIntact(ctx context.Context, receipt receipts.Receipt) (bool, error) {
	if len(receipt.OwnedFiles) == 0 {
		return false, nil
	}
	intact := true
	for _, file := range receipt.OwnedFiles {
		if err := checkPluginDestination(ctx, file.Path, receipt); err != nil {
			return false, err
		}
		info, err := os.Stat(file.Path)
		if errors.Is(err, os.ErrNotExist) {
			intact = false
			continue
		}
		if err != nil {
			return false, errpkg.Wrap("E_PLUGIN_OWNERSHIP", err, "inspect receipt file")
		}
		// Windows does not preserve Unix execute bits; receipt hashes remain authoritative there.
		if runtime.GOOS != "windows" && info.Mode().Perm() != file.Mode.Perm() {
			intact = false
		}
	}
	return intact, nil
}
