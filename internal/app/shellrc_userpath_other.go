//go:build !windows

package app

import (
	"context"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

func planWindowsUserPath(context.Context, string, string) (ShellRCPlan, error) {
	return ShellRCPlan{}, errpkg.New("E_SHELL_UNSUPPORTED", "Windows user Path is unavailable on this platform")
}

func applyWindowsUserPath(context.Context, ShellRCPlan) error {
	return errpkg.New("E_SHELL_UNSUPPORTED", "Windows user Path is unavailable on this platform")
}
