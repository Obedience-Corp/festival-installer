package installer

import (
	"context"
	"database/sql"
	"errors"
	"os"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func sameFile(a, b string) bool {
	ad, ai, err := fileID(a)
	if err != nil {
		return false
	}
	bd, bi, err := fileID(b)
	return err == nil && ad == bd && ai == bi
}

func isAppPayload(j AppIntent, path string) bool {
	if j.PayloadIno == 0 {
		return false
	}
	d, i, err := fileID(path)
	return err == nil && d == j.PayloadDev && i == j.PayloadIno
}

func reconcileFail(cause error, what string) error {
	return errpkg.Wrap("E_INSTALL_RECONCILE", cause, what)
}

// ReconcileApp reverses an incomplete App placement recorded in app-intent.json.
// The caller must hold the installer lock. It never touches install-intent.json
// or intent-backups/.
func ReconcileApp(ctx context.Context, home string, db *sql.DB) error {
	if err := ctx.Err(); err != nil {
		return errpkg.Wrap("E_INSTALL_CTX", err, "context cancelled before app reconcile")
	}
	j, ok, err := readAppIntent(home)
	if err != nil || !ok {
		return err
	}
	if err := validateAppIntent(j); err != nil {
		return err
	}
	switch j.Phase {
	case AppPhaseExternal:
		return clearAppIntent(home)
	case AppPhaseStaging, AppPhaseStaged:
		return finishAppReconcile(home, j)
	case AppPhaseCommitted:
		if j.Backup != "" {
			if err := os.RemoveAll(j.Backup); err != nil {
				return reconcileFail(err, "remove old App copy "+j.Backup)
			}
		}
		return finishAppReconcile(home, j)
	}
	return reconcileSwap(ctx, home, db, j)
}

func finishAppReconcile(home string, j AppIntent) error {
	if j.Staged != "" {
		if err := os.RemoveAll(j.Staged); err != nil {
			return reconcileFail(err, "remove App stage "+j.Staged)
		}
	}
	return clearAppIntent(home)
}

// reconcileSwap handles phases swapping and swapped by what is on disk, so a
// crash during reconcile converges on the next run.
func reconcileSwap(ctx context.Context, home string, db *sql.DB, j AppIntent) error {
	destExists := pathExists(j.Dest)
	isNew := destExists && isAppPayload(j, j.Dest)

	// The darwin exchange stopped before the old copy left the payload path.
	if isNew && j.Backup != "" && pathExists(j.Payload) && !pathExists(j.Backup) {
		if err := os.Rename(j.Payload, j.Backup); err != nil {
			return reconcileFail(err, "move old App copy to "+j.Backup)
		}
	}
	if !destExists {
		if j.Backup != "" && pathExists(j.Backup) {
			if err := os.Rename(j.Backup, j.Dest); err != nil {
				return reconcileFail(err, "restore "+j.Dest)
			}
		}
		return finishAppReconcile(home, j)
	}
	if isNew {
		return reconcileNew(ctx, home, db, j)
	}
	return reconcileOld(home, j)
}

func reconcileNew(ctx context.Context, home string, db *sql.DB, j AppIntent) error {
	committed, err := appReceiptCommitted(ctx, db, j)
	if err != nil {
		return err
	}
	if committed {
		if j.Backup != "" {
			if err := os.RemoveAll(j.Backup); err != nil {
				return reconcileFail(err, "remove old App copy "+j.Backup)
			}
		}
		return finishAppReconcile(home, j)
	}
	if j.Backup == "" {
		if err := os.RemoveAll(j.Dest); err != nil {
			return reconcileFail(err, "remove incomplete App install "+j.Dest)
		}
		return finishAppReconcile(home, j)
	}
	info, err := os.Lstat(j.Backup)
	if err != nil {
		return errpkg.New("E_INSTALL_RECONCILE", "old App copy "+j.Backup+" is missing while "+j.Dest+" holds the new copy; left both alone")
	}
	if info.Mode().IsRegular() {
		if err := os.Rename(j.Backup, j.Dest); err != nil {
			return reconcileFail(err, "restore "+j.Dest)
		}
		return finishAppReconcile(home, j)
	}
	if err := restoreDirectory(j); err != nil {
		return err
	}
	return finishAppReconcile(home, j)
}

// restoreDirectory puts the old directory copy back at Dest.
func restoreDirectory(j AppIntent) error {
	err := exchangeFn(j.Backup, j.Dest)
	if err == nil {
		if herr := appReconcileStep("restore-after-exchange"); herr != nil {
			return herr
		}
		if err := os.RemoveAll(j.Backup); err != nil {
			return reconcileFail(err, "remove new App copy "+j.Backup)
		}
		return nil
	}
	if !errors.Is(err, errExchangeUnsupported) {
		return reconcileFail(err, "exchange "+j.Backup+" and "+j.Dest)
	}
	if err := os.Rename(j.Dest, j.Payload); err != nil {
		return reconcileFail(err, "set aside new App copy "+j.Dest)
	}
	if herr := appReconcileStep("restore-after-first-rename"); herr != nil {
		return herr
	}
	if err := os.Rename(j.Backup, j.Dest); err != nil {
		return reconcileFail(err, "restore "+j.Dest)
	}
	return nil
}

// reconcileOld handles a Dest that is not the new copy: the swap never
// happened, a file swap stopped between link and rename, or a restore
// exchanged the copies back and stopped before removing the new one.
func reconcileOld(home string, j AppIntent) error {
	if err := os.RemoveAll(j.Staged); err != nil {
		return reconcileFail(err, "remove App stage "+j.Staged)
	}
	if j.Backup != "" && pathExists(j.Backup) {
		switch {
		case sameFile(j.Backup, j.Dest):
			if err := os.Remove(j.Backup); err != nil {
				return reconcileFail(err, "remove link "+j.Backup)
			}
		case isAppPayload(j, j.Backup):
			if err := os.RemoveAll(j.Backup); err != nil {
				return reconcileFail(err, "remove new App copy "+j.Backup)
			}
		default:
			return errpkg.New("E_INSTALL_RECONCILE", j.Dest+" is not the new App copy and "+j.Backup+" is some other copy; left both alone")
		}
	}
	return clearAppIntent(home)
}
