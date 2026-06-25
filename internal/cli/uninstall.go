package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
	"github.com/Obedience-Corp/obey-installer/internal/hosts/shared"
	"github.com/Obedience-Corp/obey-installer/internal/jsonout"
	"github.com/Obedience-Corp/obey-installer/internal/state"
	"github.com/Obedience-Corp/obey-installer/internal/state/lock"
	"github.com/Obedience-Corp/obey-installer/internal/state/receipts"
)

const uninstallLockTimeout = 30 * time.Second

var ErrOutsideManagedBin = errpkg.New("E_UNINSTALL_PATH", "receipt file resolves outside the managed bin dir")

type uninstallResult struct {
	Package string   `json:"package"`
	Removed []string `json:"removed,omitempty"`
	Note    string   `json:"note,omitempty"`
}

func NewUninstallCommand() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "uninstall <festival|camp|fest>",
		Short: "Remove the installer-managed festival suite (receipt-owned files only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := args[0]
			packageID := festivalPackageID
			if host, name, ok := pluginHost(target); ok {
				id, err := resolvePluginPackageID(cmd.Context(), host, name)
				if err != nil {
					return err
				}
				packageID = id
			} else {
				switch target {
				case "festival", "camp", "fest":
				default:
					return errpkg.New("E_UNINSTALL_TARGET", "unknown uninstall target "+target+" (expected festival, camp, fest, or a camp-*/fest-* plugin)")
				}
			}
			res, err := uninstallPackage(cmd.Context(), packageID)
			if err != nil {
				return err
			}
			if asJSON {
				return jsonout.Print(cmd.OutOrStdout(), res)
			}
			return renderUninstallResult(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON output")
	return cmd
}

func uninstallPackage(ctx context.Context, packageID string) (uninstallResult, error) {
	if err := ctx.Err(); err != nil {
		return uninstallResult{}, errpkg.Wrap("E_UNINSTALL_CTX", err, "context cancelled")
	}
	home, err := state.Home(ctx)
	if err != nil {
		return uninstallResult{}, err
	}
	binDir, err := state.BinDir(ctx)
	if err != nil {
		return uninstallResult{}, err
	}

	fl, err := lock.NewFileLock(home)
	if err != nil {
		return uninstallResult{}, err
	}
	rel, err := fl.Acquire(ctx, uninstallLockTimeout)
	if err != nil {
		return uninstallResult{}, err
	}
	defer func() { _ = rel() }()

	db, err := state.OpenDB(ctx, home)
	if err != nil {
		return uninstallResult{}, err
	}
	defer func() { _ = db.Close(ctx) }()

	rec, err := receipts.Get(ctx, db.Raw(), packageID)
	if errors.Is(err, receipts.ErrNotFound) {
		return uninstallResult{Package: packageID, Note: "not installed (no receipt); nothing to uninstall"}, nil
	}
	if err != nil {
		return uninstallResult{}, err
	}

	var removed []string
	for _, f := range rec.OwnedFiles {
		if err := assertWithinManagedBin(f.Path, binDir); err != nil {
			return uninstallResult{}, err
		}
		if err := shared.RemoveByRecord(ctx, f); err != nil {
			return uninstallResult{}, err
		}
		removed = append(removed, f.Path)
	}

	if err := receipts.Delete(ctx, db.Raw(), packageID); err != nil {
		return uninstallResult{}, err
	}
	return uninstallResult{Package: packageID, Removed: removed}, nil
}

func assertWithinManagedBin(path, binDir string) error {
	if resolvePath(filepath.Dir(path)) != resolvePath(binDir) {
		return errpkg.Wrap("E_UNINSTALL_PATH", ErrOutsideManagedBin, path)
	}
	return nil
}

func renderUninstallResult(w io.Writer, res uninstallResult) error {
	if res.Note != "" {
		_, err := fmt.Fprintln(w, res.Note)
		return err
	}
	if _, err := fmt.Fprintf(w, "uninstalled %s\n", res.Package); err != nil {
		return err
	}
	for _, f := range res.Removed {
		if _, err := fmt.Fprintf(w, "  removed %s\n", f); err != nil {
			return err
		}
	}
	return nil
}
