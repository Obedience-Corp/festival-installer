//go:build unix

package lock

import (
	"context"
	"time"

	"golang.org/x/sys/unix"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

func tryAcquire(fd int) (bool, error) {
	flk := unix.Flock_t{
		Type:   unix.F_WRLCK,
		Whence: 0,
		Start:  0,
		Len:    0,
	}
	if err := unix.FcntlFlock(uintptr(fd), unix.F_SETLK, &flk); err != nil {
		if err == unix.EAGAIN || err == unix.EACCES {
			return false, nil
		}
		return false, errpkg.Wrap("E_LOCK_FCNTL", err, "fcntl F_SETLK")
	}
	return true, nil
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
	flk := unix.Flock_t{
		Type:   unix.F_UNLCK,
		Whence: 0,
		Start:  0,
		Len:    0,
	}
	if err := unix.FcntlFlock(uintptr(fd), unix.F_SETLK, &flk); err != nil {
		return errpkg.Wrap("E_LOCK_FCNTL", err, "fcntl F_UNLCK")
	}
	return nil
}
