package app

import (
	"context"
	"io"
	"os"
	"runtime"
	"strings"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/installer"
	"github.com/Obedience-Corp/festival-installer/internal/state"
)

const notInstalledUninstallNote = "Festival App is not installed through festival; nothing to uninstall."

// UninstallApp removes Festival App from the location its receipt records
// (D006, D007). out receives system uninstaller output; nil discards it.
func UninstallApp(ctx context.Context, out io.Writer) (AppResult, error) {
	if err := ctx.Err(); err != nil {
		return AppResult{}, errpkg.Wrap("E_UNINSTALL_CTX", err, "context cancelled")
	}
	if runtime.GOOS != "windows" && os.Geteuid() == 0 {
		return AppResult{}, errpkg.New("E_APP_ELEVATION", "run festival uninstall app as your normal user without sudo")
	}
	none := AppResult{Package: AppPackageID, Notes: []string{notInstalledUninstallNote}}
	home, err := state.Home(ctx)
	if err != nil {
		return AppResult{}, err
	}
	db, ok, err := state.OpenDBIfExists(ctx, home)
	if err != nil {
		return AppResult{}, err
	}
	if !ok {
		return none, nil
	}
	defer func() { _ = db.Close(ctx) }()
	txn, err := installer.BeginApp(ctx, db.Raw(), home)
	if err != nil {
		return AppResult{}, err
	}
	defer func() { _ = txn.Close(ctx) }()
	store := dbAppReceiptStore{db: db.Raw()}
	rec, err := store.Read(ctx)
	if err != nil {
		return AppResult{}, err
	}
	if rec == nil {
		return none, nil
	}
	platform, err := appPlatformFor(ctx)
	if err != nil {
		return AppResult{}, err
	}
	scope, err := appRunScopeFor(rec.Format, rec.Location)
	if err != nil {
		return AppResult{}, err
	}
	if err := refuseRunningApp(ctx, scope); err != nil {
		return AppResult{}, err
	}
	removed, notes, err := platform.remove(ctx, *rec, out)
	if err != nil {
		return AppResult{}, err
	}
	if err := store.Delete(ctx); err != nil {
		return AppResult{}, errpkg.Wrap("E_APP_RECEIPT", err, "delete the Festival App receipt")
	}
	return AppResult{
		Package: AppPackageID, PreviousVersion: rec.Version, Format: rec.Format, Location: rec.Location,
		Adopted: rec.Adopted, Changed: removed, Notes: notes,
	}, nil
}

func (r AppResult) uninstallResult() UninstallResult {
	res := UninstallResult{Package: AppPackageID}
	if r.Changed {
		res.Removed = []string{r.Location}
	}
	if len(r.Notes) > 0 {
		note := strings.Join(r.Notes, "\n")
		if r.Changed {
			note = "uninstalled " + AppPackageID + "\n  removed " + r.Location + "\n" + note
		}
		res.Note = note
	}
	return res
}
