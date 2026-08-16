package artifacts

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

func AtomicMove(ctx context.Context, src, dst string) error {
	if err := ctx.Err(); err != nil {
		return errpkg.Wrap("E_ARTIFACT_CTX", err, "context cancelled before move")
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return errpkg.Wrap("E_ARTIFACT_MKDIR", err, "create parent of "+dst)
	}
	if err := os.Rename(src, dst); err != nil {
		if errors.Is(err, syscall.EXDEV) {
			return errpkg.Wrap("E_ARTIFACT_EXDEV", ErrCrossDevice, src+" -> "+dst)
		}
		return errpkg.Wrap("E_ARTIFACT_RENAME", err, "rename "+src+" -> "+dst)
	}
	return nil
}
