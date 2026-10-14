package app

import (
	"path/filepath"

	"golang.org/x/sys/windows"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

// Use the Windows Documents known folder because PowerShell follows folder
// redirection when it resolves $PROFILE. CurrentUserAllHosts runs in every
// PowerShell host, including the ordinary console and Windows Terminal.
func powerShellProfile(shell string) (string, error) {
	documents, err := windows.KnownFolderPath(windows.FOLDERID_Documents, windows.KF_FLAG_DEFAULT)
	if err != nil {
		return "", errpkg.Wrap("E_SHELL_RC_DOCUMENTS", err, "resolve the Windows Documents folder for the PowerShell profile")
	}
	dir := "WindowsPowerShell"
	if shell == "pwsh" {
		dir = "PowerShell"
	}
	return filepath.Join(documents, dir, "profile.ps1"), nil
}
