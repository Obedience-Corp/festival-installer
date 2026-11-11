package installer

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"time"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/metadata"
	"github.com/Obedience-Corp/festival-installer/internal/state/lock"
)

var (
	exchangeFn             = exchange
	linkFn                 = os.Link
	errExchangeUnsupported = errors.New("atomic exchange unsupported on this volume")
)

// AppPlacement describes one App placement or external install.
type AppPlacement struct {
	PackageID      string
	Version        string
	Format         string // Prepare: app-bundle or appimage; RecordExternal: nsis or deb
	ArtifactSHA256 string
	// Dest: Prepare takes the absolute target ("/Applications/Festival App.app",
	// "/home/u/.local/bin/festival-app"). RecordExternal stores it as given
	// ("dpkg:festival-app" or the NSIS install dir).
	Dest string
}

// AppTxn is one App placement under the installer lock.
type AppTxn struct {
	db         *sql.DB
	home       string
	release    lock.Release
	intent     AppIntent
	journaled  bool
	external   bool
	committed  bool
	done       bool
	afterPhase func(point string) error // test hook; nil in production
}

// BeginApp takes the installer lock and reconciles both journals.
func BeginApp(ctx context.Context, db *sql.DB, home string) (*AppTxn, error) {
	if err := ctx.Err(); err != nil {
		return nil, errpkg.Wrap("E_INSTALL_CTX", err, "context cancelled before begin")
	}
	rel, err := lockAndReconcile(ctx, db, home, defaultLockTimeout)
	if err != nil {
		return nil, err
	}
	return &AppTxn{db: db, home: home, release: rel}, nil
}

func (t *AppTxn) hook(point string) error {
	if t.afterPhase != nil {
		return t.afterPhase(point)
	}
	return nil
}

func (t *AppTxn) setPhase(phase AppPhase, point string) error {
	t.intent.Phase = phase
	if err := writeAppIntent(t.home, t.intent); err != nil {
		return err
	}
	return t.hook(point)
}

func (t *AppTxn) usable() error {
	if t.done || t.committed {
		return ErrAlreadyCommitted
	}
	return nil
}

// Prepare journals phase staging, creates the stage dir and returns the payload
// path the caller must fill: extract the bundle so its root lands exactly
// there, or copy the AppImage there with mode 0755.
func (t *AppTxn) Prepare(ctx context.Context, p AppPlacement) (string, error) {
	if err := t.usable(); err != nil {
		return "", err
	}
	if t.journaled {
		return "", errpkg.New("E_INSTALL_STAGING", "this App transaction already started")
	}
	if p.Format != metadata.ArtifactKindAppBundle && p.Format != metadata.ArtifactKindAppImage {
		return "", errpkg.New("E_INSTALL_STAGING", "format "+p.Format+" is not placed beside its target")
	}
	if !filepath.IsAbs(p.Dest) || filepath.Clean(p.Dest) != p.Dest || filepath.Dir(p.Dest) == p.Dest {
		return "", errpkg.New("E_INSTALL_STAGING", "App target must be an absolute clean path: "+p.Dest)
	}
	id, err := newAppIntentID()
	if err != nil {
		return "", err
	}
	staged := stagedSiblingPath(p.Dest, id)
	backup := ""
	if pathExists(p.Dest) {
		backup = oldSiblingPath(p.Dest, id)
	}
	for _, path := range []string{staged, oldSiblingPath(p.Dest, id)} {
		if pathExists(path) {
			return "", errpkg.New("E_INSTALL_STAGING", "stage path already exists: "+path)
		}
	}
	if err := os.MkdirAll(filepath.Dir(p.Dest), 0o755); err != nil {
		return "", errpkg.Wrap("E_INSTALL_STAGING", err, "create "+filepath.Dir(p.Dest))
	}
	t.intent = AppIntent{
		Schema: 1, ID: id, PackageID: p.PackageID, Version: p.Version, Format: p.Format,
		ArtifactSHA256: p.ArtifactSHA256, Dest: p.Dest, Staged: staged,
		Payload: filepath.Join(staged, filepath.Base(p.Dest)), Backup: backup,
		StartedAt: time.Now().UTC(),
	}
	t.journaled = true
	if err := t.setPhase(AppPhaseStaging, "staging-written"); err != nil {
		return "", err
	}
	if err := os.Mkdir(staged, 0o755); err != nil {
		return "", errpkg.Wrap("E_INSTALL_STAGING", err, "create stage dir "+staged)
	}
	return t.intent.Payload, nil
}

// MarkStaged records the payload's identity once the caller has filled it.
func (t *AppTxn) MarkStaged(ctx context.Context) error {
	if err := t.usable(); err != nil {
		return err
	}
	if t.external || !t.journaled || t.intent.Phase != AppPhaseStaging {
		return errpkg.New("E_INSTALL_STAGING", "MarkStaged needs a prepared payload")
	}
	dev, ino, err := fileID(t.intent.Payload)
	if err != nil {
		return errpkg.Wrap("E_INSTALL_STAGING", err, "payload missing at "+t.intent.Payload)
	}
	t.intent.PayloadDev, t.intent.PayloadIno = dev, ino
	return t.setPhase(AppPhaseStaged, "staged-written")
}

// Swap puts the staged payload at the target. A crash at any point leaves one
// complete copy at the target; ReconcileApp finishes or reverses the swap.
func (t *AppTxn) Swap(ctx context.Context) error {
	if err := t.usable(); err != nil {
		return err
	}
	if t.external || t.intent.Phase != AppPhaseStaged {
		return errpkg.New("E_INSTALL_STAGING", "Swap needs a staged payload")
	}
	if err := ctx.Err(); err != nil {
		return errpkg.Wrap("E_INSTALL_CTX", err, "context cancelled before swap")
	}
	if err := t.setPhase(AppPhaseSwapping, "swapping-written"); err != nil {
		return err
	}
	if err := t.place(); err != nil {
		return err
	}
	return t.setPhase(AppPhaseSwapped, "swapped-written")
}

func (t *AppTxn) place() error {
	j := t.intent
	info, err := os.Lstat(j.Dest)
	switch {
	case os.IsNotExist(err):
		if j.Backup != "" {
			return errpkg.New("E_INSTALL_STAGING", j.Dest+" disappeared during the install")
		}
		return renameErr(j.Payload, j.Dest)
	case err != nil:
		return errpkg.Wrap("E_INSTALL_STAGING", err, "inspect "+j.Dest)
	case j.Backup == "":
		return errpkg.New("E_INSTALL_STAGING", j.Dest+" appeared during the install")
	case info.IsDir():
		return t.swapDirectory()
	case info.Mode().IsRegular():
		return t.swapFile()
	}
	return errpkg.New("E_INSTALL_STAGING", j.Dest+" is neither a directory nor a regular file")
}

func (t *AppTxn) swapDirectory() error {
	j := t.intent
	err := exchangeFn(j.Payload, j.Dest)
	if err == nil {
		if herr := t.hook("after-exchange"); herr != nil {
			return herr
		}
		return renameErr(j.Payload, j.Backup)
	}
	if !errors.Is(err, errExchangeUnsupported) {
		return errpkg.Wrap("E_INSTALL_STAGING", err, "exchange "+j.Payload+" and "+j.Dest)
	}
	return t.twoRenames()
}

func (t *AppTxn) swapFile() error {
	j := t.intent
	if err := linkFn(j.Dest, j.Backup); err != nil {
		return t.twoRenames()
	}
	if herr := t.hook("after-link"); herr != nil {
		return herr
	}
	return renameErr(j.Payload, j.Dest)
}

func (t *AppTxn) twoRenames() error {
	j := t.intent
	if err := renameErr(j.Dest, j.Backup); err != nil {
		return err
	}
	if herr := t.hook("after-first-rename"); herr != nil {
		return herr
	}
	return renameErr(j.Payload, j.Dest)
}

func renameErr(from, to string) error {
	if err := os.Rename(from, to); err != nil {
		return errpkg.Wrap("E_INSTALL_STAGING", err, "rename "+from+" to "+to)
	}
	return nil
}

// Commit writes the receipt through writeReceipt and finishes the placement. The
// writer must use the given *sql.DB directly, never state.WithHomeLock. It
// returns the old copy's path when that copy could not be removed; the journal
// then stays in phase committed and the next run retries.
func (t *AppTxn) Commit(ctx context.Context, writeReceipt func(context.Context, *sql.DB) error) (string, error) {
	if err := t.usable(); err != nil {
		return "", err
	}
	if !t.journaled {
		return "", errpkg.New("E_INSTALL_STAGING", "nothing to commit")
	}
	if t.external {
		if err := writeReceipt(ctx, t.db); err != nil {
			return "", err
		}
		t.committed = true
		return "", clearAppIntent(t.home)
	}
	if t.intent.Phase != AppPhaseSwapped {
		return "", errpkg.New("E_INSTALL_STAGING", "Commit needs a completed swap")
	}
	if err := writeReceipt(ctx, t.db); err != nil {
		if rerr := ReconcileApp(ctx, t.home, nil); rerr != nil {
			return "", errors.Join(err, rerr)
		}
		return "", err
	}
	if err := t.hook("receipt-written"); err != nil {
		return "", err
	}
	if err := t.setPhase(AppPhaseCommitted, "committed-written"); err != nil {
		return "", err
	}
	t.committed = true
	return t.finishCommit()
}

func (t *AppTxn) finishCommit() (string, error) {
	j := t.intent
	leftover := ""
	if j.Backup != "" {
		if err := os.RemoveAll(j.Backup); err != nil && pathExists(j.Backup) {
			leftover = j.Backup
		}
	}
	_ = os.RemoveAll(j.Staged)
	if leftover != "" {
		return leftover, nil
	}
	return "", clearAppIntent(t.home)
}

// RecordExternal journals an install owned by an OS installer (nsis, deb). It
// creates nothing on disk; Dest is stored verbatim and never read as a path.
func (t *AppTxn) RecordExternal(ctx context.Context, p AppPlacement) error {
	if err := t.usable(); err != nil {
		return err
	}
	if t.journaled {
		return errpkg.New("E_INSTALL_STAGING", "this App transaction already started")
	}
	id, err := newAppIntentID()
	if err != nil {
		return err
	}
	t.intent = AppIntent{
		Schema: 1, ID: id, PackageID: p.PackageID, Version: p.Version, Format: p.Format,
		ArtifactSHA256: p.ArtifactSHA256, Dest: p.Dest, Phase: AppPhaseExternal, StartedAt: time.Now().UTC(),
	}
	t.journaled, t.external = true, true
	return writeAppIntent(t.home, t.intent)
}

// Close reverses an uncommitted placement, then releases the lock. Calling it
// again does nothing.
func (t *AppTxn) Close(ctx context.Context) error {
	if t.done {
		return nil
	}
	t.done = true
	var err error
	if t.journaled && !t.committed {
		err = ReconcileApp(ctx, t.home, nil)
	}
	if t.release != nil {
		if rerr := t.release(); rerr != nil && err == nil {
			err = rerr
		}
	}
	return err
}
