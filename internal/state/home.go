package state

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
)

const homeEnvVar = "OBEY_INSTALLER_HOME"

func Home(ctx context.Context) (string, error) {
	if v := strings.TrimSpace(os.Getenv(homeEnvVar)); v != "" {
		if !filepath.IsAbs(v) {
			return "", errpkg.New("E_HOME_NOT_ABS", homeEnvVar+" must be an absolute path")
		}
		return v, nil
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", errpkg.Wrap("E_HOME_USERHOME", err, "cannot resolve $HOME")
	}
	return filepath.Join(userHome, ".obey", "installer"), nil
}

func EnsureHome(ctx context.Context, mode os.FileMode) error {
	path, err := Home(ctx)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(path, mode); err != nil {
		return errpkg.Wrap("E_HOME_MKDIR", err, "cannot create installer home")
	}
	if err := os.Chmod(path, mode); err != nil {
		return errpkg.Wrap("E_HOME_CHMOD", err, "cannot chmod installer home")
	}
	return nil
}
