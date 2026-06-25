package fest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
	"github.com/Obedience-Corp/obey-installer/internal/metadata"
	"github.com/Obedience-Corp/obey-installer/internal/state/receipts"
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

	if err := copyTreeSafe(staged, dst); err != nil {
		_ = os.RemoveAll(dst)
		return nil, err
	}
	records, err := fileRecordsForTree(dst)
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
	var dirs []string
	for _, f := range owned {
		if err := os.Remove(f.Path); err != nil && !os.IsNotExist(err) {
			return errpkg.Wrap("E_EXTENSION_REMOVE", err, "removing "+f.Path)
		}
		dirs = append(dirs, filepath.Dir(f.Path))
	}
	root, err := extensionsRoot()
	if err != nil {
		return err
	}
	pruneEmptyDirs(dirs, root)
	return nil
}

func copyTreeSafe(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return errpkg.Wrap("E_EXTENSION_WALK", err, "walk "+path)
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return errpkg.New("E_EXTENSION_UNSAFE", "symlink not permitted in extension tree: "+path)
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return errpkg.Wrap("E_EXTENSION_UNSAFE", err, "resolve "+path)
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return errpkg.New("E_EXTENSION_UNSAFE", "path escapes extension tree: "+rel)
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			return mkdirSafe(target)
		case d.Type().IsRegular():
			if err := mkdirSafe(filepath.Dir(target)); err != nil {
				return err
			}
			return copyFile(path, target)
		default:
			return errpkg.New("E_EXTENSION_UNSAFE", "irregular file not permitted: "+path)
		}
	})
}

func mkdirSafe(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return errpkg.Wrap("E_EXTENSION_MKDIR", err, "mkdir "+dir)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return errpkg.Wrap("E_EXTENSION_OPEN", err, "open "+src)
	}
	defer func() { _ = in.Close() }()
	info, err := in.Stat()
	if err != nil {
		return errpkg.Wrap("E_EXTENSION_STAT", err, "stat "+src)
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return errpkg.Wrap("E_EXTENSION_CREATE", err, "create "+dst)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return errpkg.Wrap("E_EXTENSION_WRITE", err, "write "+dst)
	}
	if err := out.Close(); err != nil {
		return errpkg.Wrap("E_EXTENSION_CLOSE", err, "close "+dst)
	}
	return nil
}

func fileRecordsForTree(root string) ([]receipts.OwnedFile, error) {
	var records []receipts.OwnedFile
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return errpkg.Wrap("E_EXTENSION_WALK", err, "walk "+path)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return errpkg.Wrap("E_EXTENSION_STAT", err, "stat "+path)
		}
		hash, err := hashFile(path)
		if err != nil {
			return err
		}
		records = append(records, receipts.OwnedFile{Path: path, Hash: hash, Mode: info.Mode().Perm()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return records, nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", errpkg.Wrap("E_EXTENSION_OPEN", err, "open "+path)
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", errpkg.Wrap("E_EXTENSION_HASH", err, "hash "+path)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func pruneEmptyDirs(dirs []string, root string) {
	slices.Sort(dirs)
	slices.Reverse(dirs)
	for _, dir := range dirs {
		rel, err := filepath.Rel(root, dir)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			continue
		}
		_ = os.Remove(dir)
	}
}
