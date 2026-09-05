package state

import (
	"context"
	"time"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/state/lock"
)

// HomeLockTimeout is how long a writer waits for the installer home lock before
// giving up.
const HomeLockTimeout = 30 * time.Second

// WithHomeLock creates the installer home if needed, holds its file lock, opens
// the database and runs fn. Every writer takes this path so the TUI cannot race
// an install that is running at the same time.
func WithHomeLock(ctx context.Context, fn func(ctx context.Context, db *DB) error) error {
	home, err := Home(ctx)
	if err != nil {
		return err
	}
	if err := EnsureHome(ctx, 0o700); err != nil {
		return err
	}
	fl, err := lock.NewFileLock(home)
	if err != nil {
		return err
	}
	release, err := fl.Acquire(ctx, HomeLockTimeout)
	if err != nil {
		return errpkg.Wrap("E_LOCK_ACQUIRE", err, "acquire installer lock")
	}
	defer func() { _ = release() }()

	db, err := OpenDB(ctx, home)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close(ctx) }()

	return fn(ctx, db)
}
