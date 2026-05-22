//go:build !unix

package lock

import (
	"context"
	"time"
)

func tryAcquire(fd int) (bool, error) {
	return false, ErrUnsupportedPlatform
}

func waitAcquire(ctx context.Context, fd int, timeout time.Duration) error {
	return ErrUnsupportedPlatform
}

func release(fd int) error {
	return ErrUnsupportedPlatform
}
