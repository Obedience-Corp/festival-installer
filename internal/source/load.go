package source

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
	"github.com/Obedience-Corp/obey-installer/internal/metadata"
)

var ErrPackageNotFound = errpkg.New("E_PKG_NOT_FOUND", "package not found in source")

func LoadPackageManifest(ctx context.Context, sourceName, packageID string) (metadata.PackageManifest, error) {
	if err := ctx.Err(); err != nil {
		return metadata.PackageManifest{}, errpkg.Wrap("E_PKG_CTX", err, "context cancelled before load")
	}
	dest, err := CloneDir(ctx, sourceName)
	if err != nil {
		return metadata.PackageManifest{}, err
	}
	mp, err := LoadMarketplace(ctx, dest)
	if err != nil {
		return metadata.PackageManifest{}, err
	}
	for _, ref := range mp.Packages {
		if ref.ID != packageID {
			continue
		}
		manifestPath, err := safeManifestPath(dest, ref.ManifestPath)
		if err != nil {
			return metadata.PackageManifest{}, err
		}
		raw, err := os.ReadFile(manifestPath)
		if err != nil {
			return metadata.PackageManifest{}, errpkg.Wrap("E_PKG_MANIFEST_READ", err, "read "+ref.ManifestPath)
		}
		return metadata.ParseManifest(ctx, raw)
	}
	return metadata.PackageManifest{}, errpkg.Wrap("E_PKG_NOT_FOUND", ErrPackageNotFound, packageID+" in source "+sourceName)
}

func safeManifestPath(dest, manifestPath string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(manifestPath))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", errpkg.Wrap("E_PKG_MANIFEST_PATH", ErrNotInCache, manifestPath)
	}
	target := filepath.Join(dest, clean)
	rel, err := filepath.Rel(dest, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", errpkg.Wrap("E_PKG_MANIFEST_PATH", ErrNotInCache, manifestPath)
	}
	return target, nil
}
