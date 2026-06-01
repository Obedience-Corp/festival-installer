package source

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
	"github.com/Obedience-Corp/obey-installer/internal/state"
)

const marketplacesDirMode = 0o700

var ErrInvalidName = errpkg.New("E_SOURCE_NAME", "source name must not contain path separators or .. segments")

func MarketplacesDir(ctx context.Context) (string, error) {
	home, err := state.Home(ctx)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "marketplaces")
	if err := os.MkdirAll(dir, marketplacesDirMode); err != nil {
		return "", errpkg.Wrap("E_CACHE_MKDIR", err, "cannot create marketplaces dir")
	}
	if err := os.Chmod(dir, marketplacesDirMode); err != nil {
		return "", errpkg.Wrap("E_CACHE_CHMOD", err, "cannot chmod marketplaces dir")
	}
	return dir, nil
}

func CloneDir(ctx context.Context, name string) (string, error) {
	if err := validateName(name); err != nil {
		return "", err
	}
	md, err := MarketplacesDir(ctx)
	if err != nil {
		return "", err
	}
	return filepath.Join(md, name), nil
}

func validateName(name string) error {
	if name == "" || name == "." || name == ".." {
		return ErrInvalidName
	}
	if strings.ContainsRune(name, '/') || strings.ContainsRune(name, filepath.Separator) {
		return ErrInvalidName
	}
	return nil
}
