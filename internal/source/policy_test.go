package source

import (
	"testing"

	"github.com/Obedience-Corp/obey-installer/internal/metadata"
	"github.com/Obedience-Corp/obey-installer/internal/verify"
)

func TestDefaultVerifyOptions_RefuseByDefault(t *testing.T) {
	vo := DefaultVerifyOptions(nil, false)
	if vo.Policy != metadata.PolicyRefuseByDefault {
		t.Fatalf("default policy = %v, want PolicyRefuseByDefault (VER-01)", vo.Policy)
	}
	if vo.AllowUnverified {
		t.Fatal("default AllowUnverified must be false")
	}
	// Empty trust root is intentional until marketplace keys are pinned (#8).
	// Unsigned installs require --allow-unverified; signed installs need keys.
	if verify.HasPinnedKeys() {
		t.Log("pinned keys present — signed installs can verify against trust root")
	}
}

func TestDefaultVerifyOptions_AllowUnverifiedFlag(t *testing.T) {
	vo := DefaultVerifyOptions(nil, true)
	if !vo.AllowUnverified {
		t.Fatal("AllowUnverified=true must propagate")
	}
	if vo.Policy != metadata.PolicyRefuseByDefault {
		t.Fatal("policy stays refuse-by-default even with allow flag (flag is the escape)")
	}
}
