package verify

import (
	"context"
	"crypto/ed25519"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

var ErrKeyNotFound = errpkg.New("E_KEY_NOT_FOUND", "verification key not found")

// KeyStore resolves verification keys by ID. Implementations may pin keys
// at build time, load them from disk, or fetch them from a network source.
type KeyStore interface {
	KeyByID(ctx context.Context, keyID string) (ed25519.PublicKey, error)
}

type staticStore struct {
	keys map[string]ed25519.PublicKey
}

func NewStaticStore(keys map[string]ed25519.PublicKey) KeyStore {
	copied := make(map[string]ed25519.PublicKey, len(keys))
	for k, v := range keys {
		copied[k] = append(ed25519.PublicKey(nil), v...)
	}
	return &staticStore{keys: copied}
}

func (s *staticStore) KeyByID(_ context.Context, keyID string) (ed25519.PublicKey, error) {
	k, ok := s.keys[keyID]
	if !ok {
		return nil, ErrKeyNotFound
	}
	return k, nil
}
