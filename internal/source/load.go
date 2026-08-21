package source

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/metadata"
	"github.com/Obedience-Corp/festival-installer/internal/verify"
)

var ErrPackageNotFound = errpkg.New("E_PKG_NOT_FOUND", "package not found in source")

var ErrPackageIDMismatch = errpkg.New("E_PKG_ID_MISMATCH", "manifest id does not match the requested package id")

type VerifyOptions struct {
	KeyStore        verify.KeyStore
	Policy          metadata.Policy
	AllowUnverified bool
	WarnWriter      io.Writer
	// SourceLabel names the source being verified, for the refusal message and
	// the unverified-content warning (metadata.IngestOptions.SourceLabel).
	// Empty falls back to the bare "source" label.
	SourceLabel string
}

// DefaultVerifyOptions is the live-path policy for install/update/plugin.
//
// Policy is RefuseByDefault: unsigned package metadata is refused unless
// allowUnverified is true (CLI --allow-unverified). When a detached .sig is
// present, IngestManifest verifies it against the trust root (PinnedKeyStore).
// Official marketplace metadata must use a matching pinned key. See VER-01.
func DefaultVerifyOptions(warnWriter io.Writer, allowUnverified bool) VerifyOptions {
	if warnWriter == nil {
		warnWriter = os.Stderr
	}
	return VerifyOptions{
		KeyStore:        verify.PinnedKeyStore(),
		Policy:          metadata.PolicyRefuseByDefault,
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
	return loadPackageManifestFromDir(ctx, dest, sourceName, packageID, vo)
}

func loadPackageManifestFromDir(ctx context.Context, dest, sourceName, packageID string, vo VerifyOptions) (metadata.PackageManifest, error) {
	mp, err := LoadMarketplace(ctx, dest, vo)
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
		sig, err := loadDetachedSignature(manifestPath, ref.ManifestPath, "E_PKG_SIG_READ")
		if err != nil {
			return metadata.PackageManifest{}, err
		}
		manifest, err := metadata.IngestManifest(ctx, vo.KeyStore, raw, sig, metadata.IngestOptions{
			Policy:          vo.Policy,
			AllowUnverified: vo.AllowUnverified,
			WarnWriter:      vo.WarnWriter,
			SourceLabel:     sourceName + "/" + packageID,
		})
		if err != nil {
			return metadata.PackageManifest{}, err
		}
		if manifest.ID != packageID {
			return metadata.PackageManifest{}, errpkg.Wrap("E_PKG_ID_MISMATCH", ErrPackageIDMismatch,
				"index entry "+packageID+" in source "+sourceName+" resolved to a manifest for "+manifest.ID)
		}
		return manifest, nil
	}
	return metadata.PackageManifest{}, errpkg.Wrap("E_PKG_NOT_FOUND", ErrPackageNotFound, packageID+" in source "+sourceName)
}

// loadDetachedSignature reads path+".sig". An absent signature is not an
// error: it returns (nil, nil) and leaves the decision to the caller's
// policy. errCode identifies the caller for an unreadable (but present)
// signature file, since package manifests and the marketplace document want
// distinct codes here.
func loadDetachedSignature(path, relPath, errCode string) (*verify.Signature, error) {
	sigRaw, err := os.ReadFile(path + ".sig")
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errpkg.Wrap(errCode, err, "read "+relPath+".sig")
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
