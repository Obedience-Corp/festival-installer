package installer

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/Obedience-Corp/obey-installer/internal/artifacts"
	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
	"github.com/Obedience-Corp/obey-installer/internal/state/lock"
	"github.com/Obedience-Corp/obey-installer/internal/state/receipts"
)

const defaultLockTimeout = 30 * time.Second

type StagedFile struct {
	StagedPath string
	DestPath   string
	Sha256     string
	Mode       os.FileMode
}

type ReceiptInfo struct {
	PackageID   string
	Version     string
	Channel     string
	Source      string
	ManifestURL string
}

type placedFile struct {
	dest   string
	backup string
}

type Transaction struct {
	db         *sql.DB
	home       string
	stagingDir string
	release    lock.Release

	mu        sync.Mutex
	staged    []StagedFile
	placed    []placedFile
	journalID string
	committed bool
	done      bool
}

func Begin(ctx context.Context, db *sql.DB, home string) (*Transaction, error) {
	return BeginWithTimeout(ctx, db, home, defaultLockTimeout)
}

func BeginWithTimeout(ctx context.Context, db *sql.DB, home string, lockTimeout time.Duration) (*Transaction, error) {
	if err := ctx.Err(); err != nil {
		return nil, errpkg.Wrap("E_INSTALL_CTX", err, "context cancelled before begin")
	}
	fl, err := lock.NewFileLock(home)
	if err != nil {
		return nil, err
	}
	rel, err := fl.Acquire(ctx, lockTimeout)
	if err != nil {
		return nil, err
	}
	// Reconcile only while holding the install lock. Otherwise a second
	// installer can roll back a live transaction's journal before it waits for
	// the lock.
	if err := Reconcile(ctx, home, db); err != nil {
		_ = rel()
		return nil, err
	}
	staging, err := os.MkdirTemp(home, "staging-")
	if err != nil {
		_ = rel()
		return nil, errpkg.Wrap("E_INSTALL_STAGING", err, "create staging dir")
	}
	return &Transaction{db: db, home: home, stagingDir: staging, release: rel}, nil
}

func (t *Transaction) StagingDir() string { return t.stagingDir }

func (t *Transaction) Stage(ctx context.Context, f StagedFile) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.committed || t.done {
		return ErrAlreadyCommitted
	}
	if f.Sha256 != "" {
		if err := artifacts.VerifySHA256(ctx, f.StagedPath, f.Sha256); err != nil {
			return err
		}
	}
	t.staged = append(t.staged, f)
	return nil
}

func (t *Transaction) Commit(ctx context.Context, info ReceiptInfo) (receipts.Receipt, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.committed || t.done {
		return receipts.Receipt{}, ErrAlreadyCommitted
	}
	if len(t.staged) == 0 {
		return receipts.Receipt{}, ErrNotStaged
	}

	journalID, err := newJournalID()
	if err != nil {
		return receipts.Receipt{}, err
	}
	t.journalID = journalID
	// Persist intent before placement so a kill mid-loop is recoverable (TXN-01).
	j := Journal{
		ID:        journalID,
		PackageID: info.PackageID,
		Version:   info.Version,
		Channel:   info.Channel,
		Source:    info.Source,
		StartedAt: time.Now().UTC(),
		Placed:    nil,
	}
	if err := writeJournal(t.home, j); err != nil {
		return receipts.Receipt{}, err
	}

	owned := make([]receipts.OwnedFile, 0, len(t.staged))
	for i, f := range t.staged {
		backup, err := t.backupPath(journalID, i, f.DestPath)
		if err != nil {
			return receipts.Receipt{}, t.abortActivation(ctx, err)
		}
		// Record the destination and planned pre-image path before any rename
		// or activate. removePlacement treats "backup path set but missing" as
		// not-started so a kill here cannot destroy a live binary.
		j.Placed = append(j.Placed, JournalPlace{Dest: f.DestPath, Backup: backup})
		if err := writeJournal(t.home, j); err != nil {
			return receipts.Receipt{}, t.abortActivation(ctx, err)
		}
		if err := t.backupExisting(f.DestPath, backup); err != nil {
			return receipts.Receipt{}, t.abortActivation(ctx, err)
		}
		if err := artifacts.AtomicMove(ctx, f.StagedPath, f.DestPath); err != nil {
			return receipts.Receipt{}, t.abortActivation(ctx, err)
		}
		t.placed = append(t.placed, placedFile{dest: f.DestPath, backup: backup})
		if f.Mode != 0 {
			if err := os.Chmod(f.DestPath, f.Mode); err != nil {
				return receipts.Receipt{}, t.abortActivation(ctx, errpkg.Wrap("E_INSTALL_CHMOD", err, "chmod "+f.DestPath))
			}
		}
		owned = append(owned, receipts.OwnedFile{Path: f.DestPath, Hash: f.Sha256, Mode: f.Mode})
	}

	r := receipts.Receipt{
		PackageID:   info.PackageID,
		Version:     info.Version,
		Source:      info.Source,
		Channel:     info.Channel,
		InstalledAt: time.Now().UTC(),
		OwnedFiles:  owned,
		Metadata: map[string]string{
			"manifest_url":       info.ManifestURL,
			"install_journal_id": j.ID,
		},
	}
	if err := receipts.Write(ctx, t.db, r); err != nil {
		return receipts.Receipt{}, t.abortActivation(ctx, err)
	}

	// Receipt is durable. Best-effort journal clear; stale journals with a
	// matching receipt are cleared on next Reconcile without reverse.
	_ = clearJournal(t.home)
	clearIntentBackups(t.home, j.ID)

	t.committed = true
	t.finish()
	return r, nil
}

// abortActivation reverses via the durable journal (not best-effort in-memory
// rollback). If reverse fails, the journal is retained for the next Begin.
func (t *Transaction) abortActivation(ctx context.Context, cause error) error {
	t.placed = nil
	if err := Reconcile(ctx, t.home, nil); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

func (t *Transaction) Rollback(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.done {
		return nil
	}
	if !t.committed {
		// Prefer journal recovery when an intent remains (failed Commit that
		// could not fully reverse). Do not wipe staging backups that may still
		// be referenced — pre-images live under intent-backups/, not staging.
		if j, ok, err := readJournal(t.home); err == nil && ok {
			if rerr := Reconcile(ctx, t.home, nil); rerr != nil {
				// Keep journal; still release lock/staging so the process can exit.
				t.finish()
				return rerr
			}
			_ = j
		} else {
			t.removePlaced()
			_ = clearJournal(t.home)
			if t.journalID != "" {
				clearIntentBackups(t.home, t.journalID)
			}
		}
	}
	t.finish()
	return nil
}

func (t *Transaction) backupPath(journalID string, i int, dest string) (string, error) {
	if _, err := os.Lstat(dest); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", errpkg.Wrap("E_INSTALL_BACKUP", err, "inspect existing "+dest)
	}
	dir := intentBackupDir(t.home, journalID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", errpkg.Wrap("E_INSTALL_BACKUP", err, "create intent backup dir")
	}
	return filepath.Join(dir, "backup-"+strconv.Itoa(i)), nil
}

func (t *Transaction) backupExisting(dest, backup string) error {
	if backup == "" {
		return nil
	}
	if err := os.Rename(dest, backup); err != nil {
		return errpkg.Wrap("E_INSTALL_BACKUP", err, "back up existing "+dest)
	}
	return nil
}

func (t *Transaction) removePlaced() {
	for i := len(t.placed) - 1; i >= 0; i-- {
		p := t.placed[i]
		_ = removePlacement(JournalPlace{Dest: p.dest, Backup: p.backup})
	}
	t.placed = nil
}

func (t *Transaction) finish() {
	_ = os.RemoveAll(t.stagingDir)
	if t.release != nil {
		_ = t.release()
		t.release = nil
	}
	t.done = true
}
