package metadata_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Obedience-Corp/obey-installer/internal/metadata"
	"github.com/Obedience-Corp/obey-installer/internal/verify"
)

func TestParseVerifiedSource_EndToEnd(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	ks := verify.NewStaticStore(map[string]ed25519.PublicKey{"k1": pub})

	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "metadata", "source", "valid.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	// Round-trip through canonical encoder so the signed bytes match what
	// downstream verifiers will check.
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	canonical, err := verify.Marshal(v)
	if err != nil {
		t.Fatalf("canonical Marshal: %v", err)
	}
	sig := verify.Signature{
		KeyID:     "k1",
		Algorithm: verify.AlgorithmEd25519,
		Bytes:     ed25519.Sign(priv, canonical),
	}

	src, err := metadata.ParseVerifiedSource(context.Background(), ks, canonical, sig)
	if err != nil {
		t.Fatalf("ParseVerifiedSource: %v", err)
	}
	if src.ID != "official-obey" {
		t.Fatalf("unexpected source: %+v", src)
	}

	// Tamper the canonical bytes; verification must fail.
	tampered := append([]byte(nil), canonical...)
	tampered[1] ^= 0x01
	_, err = metadata.ParseVerifiedSource(context.Background(), ks, tampered, sig)
	if !errors.Is(err, verify.ErrSignatureInvalid) {
		t.Fatalf("expected ErrSignatureInvalid for tampered doc, got %v", err)
	}
}

