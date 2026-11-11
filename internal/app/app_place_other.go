//go:build !darwin && !windows

package app

import (
	"context"
	"runtime"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

func hostAppPlatform(context.Context) (appPlatform, error) {
	return nil, errpkg.New("E_APP_PLATFORM", "Festival App installs are not supported on "+runtime.GOOS+" yet")
}
