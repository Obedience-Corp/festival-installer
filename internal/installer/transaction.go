package installer

import (
	"context"
	"database/sql"
	"os"
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

type Transaction struct {
	db         *sql.DB
	stagingDir string
	release    lock.Release

	mu        sync.Mutex
	staged    []StagedFile
	placed    []string
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
	staging, err := os.MkdirTemp(home, "staging-")
	if err != nil {
		_ = rel()
		return nil, errpkg.Wrap("E_INSTALL_STAGING", err, "create staging dir")
	}
	return &Transaction{db: db, stagingDir: staging, release: rel}, nil
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

	owned := make([]receipts.OwnedFile, 0, len(t.staged))
	for _, f := range t.staged {
		if err := artifacts.AtomicMove(ctx, f.StagedPath, f.DestPath); err != nil {
			t.removePlaced()
			return receipts.Receipt{}, err
		}
		t.placed = append(t.placed, f.DestPath)
		if f.Mode != 0 {
			if err := os.Chmod(f.DestPath, f.Mode); err != nil {
				t.removePlaced()
				return receipts.Receipt{}, errpkg.Wrap("E_INSTALL_CHMOD", err, "chmod "+f.DestPath)
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
		Metadata:    map[string]string{"manifest_url": info.ManifestURL},
	}
	if err := receipts.Write(ctx, t.db, r); err != nil {
		t.removePlaced()
		return receipts.Receipt{}, err
	}

	t.committed = true
	t.finish()
	return r, nil
}

func (t *Transaction) Rollback(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.done {
		return nil
	}
	t.removePlaced()
	t.finish()
	return nil
}

func (t *Transaction) removePlaced() {
	for _, p := range t.placed {
		_ = os.Remove(p)
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
