package lock

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

// Lock represents a process-level mutual-exclusion primitive backed by an
// advisory file lock. Implementations must be safe for concurrent Acquire
// calls from different processes; in-process callers must serialize via
// their own sync.Mutex if they need re-entrancy semantics.
type Lock interface {
	Acquire(ctx context.Context, timeout time.Duration) (Release, error)
}

// Release returns the lock to its unheld state. Implementations must be
// idempotent: callers may invoke Release multiple times safely (e.g. defer
// plus explicit unlock on the happy path).
type Release func() error

type FileLock struct {
	path string
}

func NewFileLock(home string) (*FileLock, error) {
	if home == "" {
		return nil, errpkg.New("E_LOCK_HOME", "home directory required")
	}
	locksDir := filepath.Join(home, "locks")
	if err := os.MkdirAll(locksDir, 0o700); err != nil {
		return nil, errpkg.Wrap("E_LOCK_MKDIR", err, "create locks dir")
	}
	if err := os.Chmod(locksDir, 0o700); err != nil {
		return nil, errpkg.Wrap("E_LOCK_CHMOD", err, "chmod locks dir")
	}
	return &FileLock{path: filepath.Join(locksDir, "installer.lock")}, nil
}

func (l *FileLock) Acquire(ctx context.Context, timeout time.Duration) (Release, error) {
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, errpkg.Wrap("E_LOCK_OPEN", err, "open lock file")
	}
	if err := os.Chmod(l.path, 0o600); err != nil {
		_ = f.Close()
		return nil, errpkg.Wrap("E_LOCK_CHMOD", err, "chmod lock file")
	}

	if err := waitAcquire(ctx, int(f.Fd()), timeout); err != nil {
		_ = f.Close()
		return nil, err
	}

	var once sync.Once
	rel := Release(func() error {
		var releaseErr error
		once.Do(func() {
			if err := release(int(f.Fd())); err != nil {
				releaseErr = err
			}
			if err := f.Close(); err != nil && releaseErr == nil {
				releaseErr = errpkg.Wrap("E_LOCK_CLOSE", err, "close lock file")
			}
		})
		return releaseErr
	})
	return rel, nil
}
