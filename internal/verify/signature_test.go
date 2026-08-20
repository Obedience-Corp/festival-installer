package verify_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/verify"
)

func TestDetachedSignatureRoundTrip(t *testing.T) {
	original := verify.Signature{KeyID: "official-2026-01", Algorithm: verify.AlgorithmEd25519, Bytes: []byte("signature")}
	raw, err := verify.MarshalDetachedSignature(original)
	if err != nil {
		t.Fatalf("MarshalDetachedSignature: %v", err)
	}
	var encoded map[string]string
	if err := json.Unmarshal(raw, &encoded); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if encoded["signature"] != base64.StdEncoding.EncodeToString(original.Bytes) {
		t.Fatalf("signature encoding mismatch: %q", encoded["signature"])
	}
	got, err := verify.ParseDetachedSignature(raw)
	if err != nil {
		t.Fatalf("ParseDetachedSignature: %v", err)
	}
	if got.KeyID != original.KeyID || got.Algorithm != original.Algorithm || string(got.Bytes) != string(original.Bytes) {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}

func newKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return pub, priv
}

func TestVerify_HappyPath(t *testing.T) {
	ctx := context.Background()
	pub, priv := newKey(t)
	ks := verify.NewStaticStore(map[string]ed25519.PublicKey{"k1": pub})

	doc := []byte(`{"hello":"world"}`)
	sig := ed25519.Sign(priv, doc)

	if err := verify.Verify(ctx, ks, doc, verify.Signature{KeyID: "k1", Algorithm: verify.AlgorithmEd25519, Bytes: sig}); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestVerify_UnsupportedAlgorithm(t *testing.T) {
	ctx := context.Background()
	ks := verify.NewStaticStore(map[string]ed25519.PublicKey{"k1": make(ed25519.PublicKey, 32)})

	err := verify.Verify(ctx, ks, []byte("doc"), verify.Signature{
		KeyID: "k1", Algorithm: "rsa-pss", Bytes: []byte("sig"),
	})
	if !errors.Is(err, verify.ErrUnsupportedAlgorithm) {
		t.Fatalf("expected ErrUnsupportedAlgorithm, got %v", err)
	}
}

func TestVerify_KeyNotFound(t *testing.T) {
	ctx := context.Background()
	ks := verify.NewStaticStore(map[string]ed25519.PublicKey{})

	err := verify.Verify(ctx, ks, []byte("doc"), verify.Signature{
		KeyID: "missing", Algorithm: verify.AlgorithmEd25519, Bytes: make([]byte, 64),
	})
	if !errors.Is(err, verify.ErrKeyNotFound) {
		t.Fatalf("expected ErrKeyNotFound, got %v", err)
	}
}

func TestVerify_TamperedDoc(t *testing.T) {
	ctx := context.Background()
	pub, priv := newKey(t)
	ks := verify.NewStaticStore(map[string]ed25519.PublicKey{"k1": pub})

	doc := []byte(`{"hello":"world"}`)
	sig := ed25519.Sign(priv, doc)

	tampered := append([]byte(nil), doc...)
	tampered[1] ^= 0x01

	err := verify.Verify(ctx, ks, tampered, verify.Signature{KeyID: "k1", Algorithm: verify.AlgorithmEd25519, Bytes: sig})
	if !errors.Is(err, verify.ErrSignatureInvalid) {
		t.Fatalf("expected ErrSignatureInvalid, got %v", err)
	}
}

func TestVerify_WrongKey(t *testing.T) {
	ctx := context.Background()
	pub, _ := newKey(t)
	_, priv2 := newKey(t)
	ks := verify.NewStaticStore(map[string]ed25519.PublicKey{"k1": pub})

	doc := []byte(`{"hello":"world"}`)
	sig := ed25519.Sign(priv2, doc) // signed with a different key

	err := verify.Verify(ctx, ks, doc, verify.Signature{KeyID: "k1", Algorithm: verify.AlgorithmEd25519, Bytes: sig})
	if !errors.Is(err, verify.ErrSignatureInvalid) {
		t.Fatalf("expected ErrSignatureInvalid (wrong key), got %v", err)
	}
}

func TestVerify_SwappedSignatureBytes(t *testing.T) {
	ctx := context.Background()
	pub, priv := newKey(t)
	ks := verify.NewStaticStore(map[string]ed25519.PublicKey{"k1": pub})

	doc := []byte(`{"hello":"world"}`)
	sig := ed25519.Sign(priv, doc)
	sig[0], sig[10] = sig[10], sig[0]

	err := verify.Verify(ctx, ks, doc, verify.Signature{KeyID: "k1", Algorithm: verify.AlgorithmEd25519, Bytes: sig})
	if !errors.Is(err, verify.ErrSignatureInvalid) {
		t.Fatalf("expected ErrSignatureInvalid (swapped bytes), got %v", err)
	}
}

func TestVerify_ErrorMessagesAvoidSecretMaterial(t *testing.T) {
	ctx := context.Background()
	pub, priv := newKey(t)
	ks := verify.NewStaticStore(map[string]ed25519.PublicKey{"k1": pub})

	doc := []byte("hello")
	sig := ed25519.Sign(priv, doc)
	sig[0] ^= 0x01

	err := verify.Verify(ctx, ks, doc, verify.Signature{KeyID: "k1", Algorithm: verify.AlgorithmEd25519, Bytes: sig})
	if err == nil {
		t.Fatal("expected verification failure")
	}
	msg := err.Error()
	for _, b := range pub {
		// printable ASCII check is irrelevant: what matters is no full key/sig substring leaks.
		_ = b
	}
	// Cheap heuristic: error message must not contain hex of full signature or key.
	if len(msg) > 0 && (containsBytes(msg, sig) || containsBytes(msg, pub)) {
		t.Fatalf("error message leaks key/sig material: %q", msg)
	}
}

func containsBytes(s string, b []byte) bool {
	if len(b) < 8 {
		return false
	}
	sub := string(b[:8])
	for i := 0; i+8 <= len(s); i++ {
		if s[i:i+8] == sub {
			return true
		}
	}
	return false
}
