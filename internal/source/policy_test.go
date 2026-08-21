package source

import (
	"os"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/metadata"
	"github.com/Obedience-Corp/festival-installer/internal/verify"
)

func TestDefaultVerifyOptions_RefuseByDefault(t *testing.T) {
	vo := DefaultVerifyOptions(nil, false)
	if vo.Policy != metadata.PolicyRefuseByDefault {
		t.Fatalf("default policy = %v, want PolicyRefuseByDefault (VER-01)", vo.Policy)
	}
	if vo.AllowUnverified {
		t.Fatal("default AllowUnverified must be false")
	}
	if !verify.HasPinnedKeys() {
		t.Fatal("production verification requires a pinned marketplace trust root")
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

func TestDefaultVerifyOptions_NilWriterFallsBackToStderr(t *testing.T) {
	vo := DefaultVerifyOptions(nil, false)
	if vo.WarnWriter != os.Stderr {
		t.Fatal("CLI nil writer must remain stderr; TUI callers pass an explicit writer")
	}
}
