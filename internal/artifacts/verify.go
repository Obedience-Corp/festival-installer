package artifacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"strings"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
)

func VerifySHA256(ctx context.Context, path, want string) error {
	if err := ctx.Err(); err != nil {
		return errpkg.Wrap("E_ARTIFACT_CTX", err, "context cancelled before verify")
	}
	f, err := os.Open(path)
	if err != nil {
		return errpkg.Wrap("E_ARTIFACT_OPEN", err, "open "+path)
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return errpkg.Wrap("E_ARTIFACT_HASH", err, "hash "+path)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, want) {
		return errpkg.Wrap("E_ARTIFACT_SHA256", ErrChecksumMismatch, "got "+got+" want "+want)
	}
	return nil
}
