//go:build !darwin && !linux && !windows

package app

import (
	"context"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

func listProcessesOS(context.Context) ([]processInfo, error) {
	return nil, errpkg.New("E_APP_PLATFORM", "Festival App is not supported on this operating system")
}
