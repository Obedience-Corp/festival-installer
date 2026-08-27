//go:build windows

package launch

import errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"

func replaceExec(string, []string, []string) error {
	return errpkg.New("E_LAUNCH_REPLACE", "restart festival after the package upgrade")
}
