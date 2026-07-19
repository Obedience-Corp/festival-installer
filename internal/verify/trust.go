package verify

import "crypto/ed25519"

// pinnedKeys is the build-time trust root for package metadata signatures.
// Empty until marketplace signing keys are embedded (issue #8 / OI0007).
// With PolicyRefuseByDefault (live default), empty keys mean:
//   - unsigned manifests require --allow-unverified
//   - signed manifests fail key lookup until a matching key is pinned
var pinnedKeys = map[string]ed25519.PublicKey{}

func PinnedKeyStore() KeyStore {
	return NewStaticStore(pinnedKeys)
}

func HasPinnedKeys() bool {
	return len(pinnedKeys) > 0
}
