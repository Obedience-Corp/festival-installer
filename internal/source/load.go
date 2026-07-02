package source

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
	"github.com/Obedience-Corp/obey-installer/internal/metadata"
	"github.com/Obedience-Corp/obey-installer/internal/verify"
)

var ErrPackageNotFound = errpkg.New("E_PKG_NOT_FOUND", "package not found in source")

type VerifyOptions struct {
	KeyStore        verify.KeyStore
	Policy          metadata.Policy
	AllowUnverified bool
	WarnWriter      io.Writer
}

func DefaultVerifyOptions(warnWriter io.Writer, allowUnverified bool) VerifyOptions {
	return VerifyOptions{
		KeyStore:        verify.PinnedKeyStore(),
		Policy:          metadata.PolicyWarnAllow,
		AllowUnverified: allowUnverified,
		WarnWriter:      warnWriter,
	}
}

func LoadPackageManifest(ctx context.Context, sourceName, packageID string, vo VerifyOptions) (metadata.PackageManifest, error) {
	if err := ctx.Err(); err != nil {
		return metadata.PackageManifest{}, errpkg.Wrap("E_PKG_CTX", err, "context cancelled before load")
	}
	if vo.KeyStore == nil {
		vo.KeyStore = verify.PinnedKeyStore()
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
		sig, err := loadDetachedSignature(manifestPath, ref.ManifestPath)
		if err != nil {
			return metadata.PackageManifest{}, err
		}
		return metadata.IngestManifest(ctx, vo.KeyStore, raw, sig, metadata.IngestOptions{
			Policy:          vo.Policy,
			AllowUnverified: vo.AllowUnverified,
			WarnWriter:      vo.WarnWriter,
			SourceLabel:     sourceName + "/" + packageID,
		})
	}
	return metadata.PackageManifest{}, errpkg.Wrap("E_PKG_NOT_FOUND", ErrPackageNotFound, packageID+" in source "+sourceName)
}

func loadDetachedSignature(manifestPath, relPath string) (*verify.Signature, error) {
	sigRaw, err := os.ReadFile(manifestPath + ".sig")
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errpkg.Wrap("E_PKG_SIG_READ", err, "read "+relPath+".sig")
	}
	sig, err := verify.ParseDetachedSignature(sigRaw)
	if err != nil {
		return nil, err
	}
	return &sig, nil
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
