package fest

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/hosts/shared"
	"github.com/Obedience-Corp/festival-installer/internal/metadata"
	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
)

const (
	FeatureMarketplaceExtensionSource = "marketplace_extension_source_v1"
	extensionManifestFile             = "extension.yml"
)

func extensionsRoot() (string, error) {
	if v := strings.TrimSpace(os.Getenv("FEST_CONFIG_DIR")); v != "" {
		if !filepath.IsAbs(v) {
			return "", errpkg.New("E_FEST_HOME_NOT_ABS", "FEST_CONFIG_DIR must be an absolute path")
		}
		return filepath.Join(v, "marketplace", "extensions"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errpkg.Wrap("E_FEST_HOME_USERHOME", err, "cannot resolve $HOME")
	}
	return filepath.Join(home, ".obey", "fest", "marketplace", "extensions"), nil
}

func TargetPath(entry metadata.InstallEntry) (string, error) {
	if entry.Kind != "extension_dir" {
		return "", errpkg.New("E_BAD_ENTRY_KIND", "fest extension activation requires kind extension_dir")
	}
	if entry.ExtensionName == "" {
		return "", errpkg.New("E_EXTENSION_NAME_EMPTY", "install entry missing extension_name")
	}
	if err := shared.ValidateSegment(entry.ExtensionName); err != nil {
		return "", err
	}
	root, err := extensionsRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, entry.ExtensionName), nil
}

func ActivateExtension(ctx context.Context, staged string, entry metadata.InstallEntry, hostFeatures []string) ([]receipts.OwnedFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, errpkg.Wrap("E_EXTENSION_CTX", err, "context cancelled")
	}
	if !slices.Contains(hostFeatures, FeatureMarketplaceExtensionSource) {
		return nil, errpkg.New("E_HOST_FEATURE_MISSING", "fest host release does not expose "+FeatureMarketplaceExtensionSource)
	}
	if _, err := os.Stat(filepath.Join(staged, extensionManifestFile)); err != nil {
		return nil, errpkg.New("E_EXTENSION_MANIFEST_MISSING", "staged extension is missing "+extensionManifestFile)
	}
	dst, err := TargetPath(entry)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(dst); err == nil {
		return nil, errpkg.New("E_EXTENSION_EXISTS", "extension dir already exists: "+dst)
	}

	if err := shared.CopyTreeSafe(staged, dst); err != nil {
		_ = os.RemoveAll(dst)
		return nil, err
	}
	records, err := shared.FileRecordsForTree(dst)
	if err != nil {
		_ = os.RemoveAll(dst)
		return nil, err
	}
	return records, nil
}

func RemoveExtension(ctx context.Context, owned []receipts.OwnedFile) error {
	if err := ctx.Err(); err != nil {
		return errpkg.Wrap("E_EXTENSION_CTX", err, "context cancelled")
	}
	root, err := extensionsRoot()
	if err != nil {
		return err
	}
	return shared.RemoveOwnedAndPrune(owned, root)
}
