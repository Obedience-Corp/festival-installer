//go:build windows

package app

import (
	"context"
	"os"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

func hostAppPlatform(context.Context) (appPlatform, error) {
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		return nil, errpkg.New("E_APP_PLATFORM", "LOCALAPPDATA is not set; cannot find where Festival App installs")
	}
	return nsisPlatform{
		readReg:      func() (nsisRegistration, bool, error) { return readNSISRegistration(nsisUninstallKey) },
		run:          runAppCommand,
		localAppData: local,
	}, nil
}
