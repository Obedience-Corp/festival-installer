package verify

import (
	"context"
	"crypto/ed25519"
	"fmt"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
)

const AlgorithmEd25519 = "ed25519"

var (
	ErrUnsupportedAlgorithm = errpkg.New("E_SIG_ALG", "signature algorithm not supported")
	ErrSignatureInvalid     = errpkg.New("E_SIG_INVALID", "signature failed verification")
)

// Signature carries the metadata required to verify a signed document.
// Bytes must be the raw signature; format is algorithm-specific.
type Signature struct {
	KeyID     string
	Algorithm string
	Bytes     []byte
}

// Verify checks that sig.Bytes is a valid signature over doc, where doc is
// expected to already be the canonical byte form produced by Marshal. The
// caller is responsible for canonicalization; Verify does not re-canonicalize.
func Verify(ctx context.Context, ks KeyStore, doc []byte, sig Signature) error {
	if err := ctx.Err(); err != nil {
		return errpkg.Wrap("E_SIG_CTX", err, "context cancelled")
	}
	if sig.Algorithm != AlgorithmEd25519 {
		return errpkg.Wrap("E_SIG_ALG", ErrUnsupportedAlgorithm,
			fmt.Sprintf("algorithm %q (key=%s, doc_len=%d)", sig.Algorithm, sig.KeyID, len(doc)))
	}
	key, err := ks.KeyByID(ctx, sig.KeyID)
	if err != nil {
		return err
	}
	if !ed25519.Verify(key, doc, sig.Bytes) {
		return errpkg.Wrap("E_SIG_INVALID", ErrSignatureInvalid,
			fmt.Sprintf("key=%s doc_len=%d", sig.KeyID, len(doc)))
	}
	return nil
}
