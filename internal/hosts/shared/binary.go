package shared

import (
	"context"
	"os"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
	"github.com/Obedience-Corp/obey-installer/internal/state/receipts"
)

func RemoveByRecord(ctx context.Context, file receipts.OwnedFile) error {
	if err := ctx.Err(); err != nil {
		return errpkg.Wrap("E_HOST_CTX", err, "context cancelled before remove")
	}
	if err := os.Remove(file.Path); err != nil && !os.IsNotExist(err) {
		return errpkg.Wrap("E_HOST_REMOVE", err, "remove "+file.Path)
	}
	return nil
}
