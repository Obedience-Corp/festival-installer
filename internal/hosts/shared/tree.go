package shared

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
	"github.com/Obedience-Corp/obey-installer/internal/state/receipts"
)

func CopyTreeSafe(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return errpkg.Wrap("E_TREE_WALK", err, "walk "+path)
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return errpkg.New("E_TREE_UNSAFE", "symlink not permitted in tree: "+path)
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return errpkg.Wrap("E_TREE_UNSAFE", err, "resolve "+path)
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return errpkg.New("E_TREE_UNSAFE", "path escapes tree: "+rel)
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			return mkdirTree(target)
		case d.Type().IsRegular():
			if err := mkdirTree(filepath.Dir(target)); err != nil {
				return err
			}
			return copyTreeFile(path, target)
		default:
			return errpkg.New("E_TREE_UNSAFE", "irregular file not permitted: "+path)
		}
	})
}

func mkdirTree(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return errpkg.Wrap("E_TREE_MKDIR", err, "mkdir "+dir)
	}
	return nil
}

func copyTreeFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return errpkg.Wrap("E_TREE_OPEN", err, "open "+src)
	}
	defer func() { _ = in.Close() }()
	info, err := in.Stat()
	if err != nil {
		return errpkg.Wrap("E_TREE_STAT", err, "stat "+src)
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return errpkg.Wrap("E_TREE_CREATE", err, "create "+dst)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return errpkg.Wrap("E_TREE_WRITE", err, "write "+dst)
	}
	if err := out.Close(); err != nil {
		return errpkg.Wrap("E_TREE_CLOSE", err, "close "+dst)
	}
	return nil
}

func FileRecordsForTree(root string) ([]receipts.OwnedFile, error) {
	var records []receipts.OwnedFile
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return errpkg.Wrap("E_TREE_WALK", err, "walk "+path)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return errpkg.Wrap("E_TREE_STAT", err, "stat "+path)
		}
		hash, err := hashTreeFile(path)
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

func hashTreeFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", errpkg.Wrap("E_TREE_OPEN", err, "open "+path)
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", errpkg.Wrap("E_TREE_HASH", err, "hash "+path)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func RemoveOwnedAndPrune(owned []receipts.OwnedFile, pruneRoot string) error {
	var dirs []string
	for _, f := range owned {
		if err := os.Remove(f.Path); err != nil && !os.IsNotExist(err) {
			return errpkg.Wrap("E_TREE_REMOVE", err, "removing "+f.Path)
		}
		dirs = append(dirs, filepath.Dir(f.Path))
	}
	slices.Sort(dirs)
	slices.Reverse(dirs)
	for _, dir := range dirs {
		rel, err := filepath.Rel(pruneRoot, dir)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			continue
		}
		_ = os.Remove(dir)
	}
	return nil
}
