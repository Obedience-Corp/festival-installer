package verify

import (
	"crypto/ed25519"
	"encoding/base64"
)

// OfficialMarketplaceKeyID identifies the current official metadata signing key.
const OfficialMarketplaceKeyID = "obedience-marketplace-2026-01"

// pinnedKeys is the build-time trust root for package metadata signatures.
var pinnedKeys = map[string]ed25519.PublicKey{
	OfficialMarketplaceKeyID: mustPublicKey("B6DUhrEgXcXGIWThyI1oe5k/iWg8h2pMLuXFx8QtOQw="),
}

func mustPublicKey(encoded string) ed25519.PublicKey {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		panic("invalid embedded marketplace public key encoding")
	}
	if len(raw) != ed25519.PublicKeySize {
		panic("invalid embedded marketplace public key length")
	}
	return ed25519.PublicKey(raw)
}

func PinnedKeyStore() KeyStore {
	return NewStaticStore(pinnedKeys)
}

func HasPinnedKeys() bool {
	return len(pinnedKeys) > 0
}
