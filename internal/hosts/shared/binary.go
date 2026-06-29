package shared

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Obedience-Corp/obey-installer/internal/artifacts"
	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
	"github.com/Obedience-Corp/obey-installer/internal/state"
	"github.com/Obedience-Corp/obey-installer/internal/state/receipts"
)

const binMode = os.FileMode(0o755)

func ActivateBinary(ctx context.Context, stagedPath, name string) (receipts.OwnedFile, error) {
	if err := ctx.Err(); err != nil {
		return receipts.OwnedFile{}, errpkg.Wrap("E_HOST_CTX", err, "context cancelled before activate")
	}
	if strings.TrimSpace(name) == "" || strings.ContainsAny(name, `/\`) {
		return receipts.OwnedFile{}, ErrEmptyName
	}

	binDir, err := state.BinDir(ctx)
	if err != nil {
		return receipts.OwnedFile{}, err
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return receipts.OwnedFile{}, errpkg.Wrap("E_HOST_MKDIR", err, "create managed bin dir")
	}

	digest, err := hashFile(stagedPath)
	if err != nil {
		return receipts.OwnedFile{}, err
	}

	dst := filepath.Join(binDir, name)
	if err := artifacts.AtomicMove(ctx, stagedPath, dst); err != nil {
		return receipts.OwnedFile{}, err
	}
	if err := os.Chmod(dst, binMode); err != nil {
		_ = os.Remove(dst)
		return receipts.OwnedFile{}, errpkg.Wrap("E_HOST_CHMOD", err, "chmod "+dst)
	}
	return receipts.OwnedFile{Path: dst, Hash: digest, Mode: binMode}, nil
}

func RemoveByRecord(ctx context.Context, file receipts.OwnedFile) error {
	if err := ctx.Err(); err != nil {
		return errpkg.Wrap("E_HOST_CTX", err, "context cancelled before remove")
	}
	if err := os.Remove(file.Path); err != nil && !os.IsNotExist(err) {
		return errpkg.Wrap("E_HOST_REMOVE", err, "remove "+file.Path)
	}
	return nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", errpkg.Wrap("E_HOST_OPEN", err, "open "+path)
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", errpkg.Wrap("E_HOST_HASH", err, "hash "+path)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
