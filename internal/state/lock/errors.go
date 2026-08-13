package lock

import errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"

var (
	ErrLockTimeout         = errpkg.New("E_LOCK_TIMEOUT", "timeout acquiring lock")
	ErrUnsupportedPlatform = errpkg.New("E_LOCK_UNSUPPORTED", "lock manager not supported on this platform")
)
