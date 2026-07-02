package verify

import "crypto/ed25519"

// TODO(#8): pinnedKeys must gain at least one real key before the installer's
// default verification policy flips from WarnAllow to RefuseByDefault; flipping
// the default with this map still empty makes every install refuse.
var pinnedKeys = map[string]ed25519.PublicKey{}

func PinnedKeyStore() KeyStore {
	return NewStaticStore(pinnedKeys)
}

func HasPinnedKeys() bool {
	return len(pinnedKeys) > 0
}
