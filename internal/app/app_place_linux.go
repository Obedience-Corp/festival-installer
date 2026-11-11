//go:build linux

package app

import (
	"context"
	"os"
	"path/filepath"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/state"
)

func hostAppPlatform(ctx context.Context) (appPlatform, error) {
	home, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(home) {
		return nil, errpkg.New("E_APP_PLATFORM", "cannot find the home directory for ~/.local/bin")
	}
	festivalHome, err := state.Home(ctx)
	if err != nil {
		return nil, err
	}
	return linuxPlatform{
		deb: debPlacer{
			run:        runAppCommand,
			probe:      func(ctx context.Context) (debInstall, error) { return probeDeb(ctx, runDpkgQuery) },
			exists:     executableExists,
			tempRoot:   os.TempDir(),
			executable: "/usr/bin/festival-app",
		},
		appimage: appImagePlacer{run: runAppCommand, binDir: filepath.Join(home, ".local", "bin"), dataHome: xdgDataHome(os.Getenv, home), stageRoot: festivalHome},
		hasApt:   linuxPackageManager(executableExists) == "/usr/bin/apt-get",
	}, nil
}
