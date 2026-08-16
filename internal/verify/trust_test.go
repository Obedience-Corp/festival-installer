package verify_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/verify"
)

func TestOfficialMarketplaceKeyPinned(t *testing.T) {
	if !verify.HasPinnedKeys() {
		t.Fatal("expected at least one pinned marketplace key")
	}
	got, err := verify.PinnedKeyStore().KeyByID(context.Background(), verify.OfficialMarketplaceKeyID)
	if err != nil {
		t.Fatalf("KeyByID: %v", err)
	}
	want, err := base64.StdEncoding.DecodeString("B6DUhrEgXcXGIWThyI1oe5k/iWg8h2pMLuXFx8QtOQw=")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("pinned official marketplace public key mismatch")
	}
}
