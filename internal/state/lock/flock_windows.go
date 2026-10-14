//go:build windows

package lock

import (
	"context"
	"errors"
	"syscall"
	"time"

	"golang.org/x/sys/windows"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

func tryAcquire(fd int) (bool, error) {
	var overlapped windows.Overlapped
	err := windows.LockFileEx(windows.Handle(fd),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0, &overlapped)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, syscall.Errno(33)) || errors.Is(err, syscall.Errno(167)) {
		return false, nil
	}
	return false, errpkg.Wrap("E_LOCK_FILE", err, "LockFileEx")
}

func waitAcquire(ctx context.Context, fd int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	delay := 5 * time.Millisecond
	const maxDelay = 200 * time.Millisecond
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		got, err := tryAcquire(fd)
		if err != nil {
			return err
		}
		if got {
			return nil
		}
		if time.Now().After(deadline) {
			return ErrLockTimeout
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
		}
	}
}

func release(fd int) error {
	var overlapped windows.Overlapped
	if err := windows.UnlockFileEx(windows.Handle(fd), 0, 1, 0, &overlapped); err != nil {
		return errpkg.Wrap("E_LOCK_FILE", err, "UnlockFileEx")
	}
	return nil
}
