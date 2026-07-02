package verify

import "crypto/ed25519"

var pinnedKeys = map[string]ed25519.PublicKey{}

func PinnedKeyStore() KeyStore {
	return NewStaticStore(pinnedKeys)
}
