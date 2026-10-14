//go:build !windows

package app

import (
	"os"
	"path/filepath"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

func powerShellProfile(string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errpkg.Wrap("E_SHELL_RC_HOME", err, "cannot resolve $HOME")
	}
	return filepath.Join(home, ".config", "powershell", "profile.ps1"), nil
}
