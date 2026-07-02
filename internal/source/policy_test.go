package source

import (
	"testing"

	"github.com/Obedience-Corp/obey-installer/internal/metadata"
	"github.com/Obedience-Corp/obey-installer/internal/verify"
)

func TestDefaultVerifyOptions_RefuseByDefaultRequiresPinnedKeys(t *testing.T) {
	vo := DefaultVerifyOptions(nil, false)
	if vo.Policy == metadata.PolicyRefuseByDefault && !verify.HasPinnedKeys() {
		t.Fatal("DefaultVerifyOptions defaults to PolicyRefuseByDefault but internal/verify has no pinned keys (see issue #8); pin at least one verification key before flipping the default, or every install will refuse")
	}
}
