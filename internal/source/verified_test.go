package source_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/source"
	"github.com/Obedience-Corp/festival-installer/internal/verify"
)

func TestParseVerifiedMarketplace(t *testing.T) {
	ctx := context.Background()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	ks := verify.NewStaticStore(map[string]ed25519.PublicKey{"k1": pub})

	valid := readFixture(t, "valid.json")
	good := verify.Signature{KeyID: "k1", Algorithm: verify.AlgorithmEd25519,
		Bytes: ed25519.Sign(priv, valid)}

	tests := []struct {
		name    string
		doc     []byte
		sig     verify.Signature
		wantErr error
	}{
		{name: "tampered bytes", doc: append(append([]byte{}, valid...), ' '),
			sig: good, wantErr: verify.ErrSignatureInvalid},
		{name: "unknown key id", doc: valid,
			sig:     verify.Signature{KeyID: "nope", Algorithm: verify.AlgorithmEd25519, Bytes: good.Bytes},
			wantErr: verify.ErrKeyNotFound},
		{name: "unsupported algorithm", doc: valid,
			sig:     verify.Signature{KeyID: "k1", Algorithm: "rsa", Bytes: good.Bytes},
			wantErr: verify.ErrUnsupportedAlgorithm},
		{name: "valid", doc: valid, sig: good, wantErr: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, err := source.ParseVerifiedMarketplace(ctx, ks, tc.doc, tc.sig)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("ParseVerifiedMarketplace: got %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseVerifiedMarketplace: %v", err)
			}
			if len(m.Packages) != 2 {
				t.Fatalf("packages: got %d want 2", len(m.Packages))
			}
		})
	}
}

func TestParseVerifiedMarketplace_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	ks := verify.NewStaticStore(map[string]ed25519.PublicKey{"k1": pub})

	valid := readFixture(t, "valid.json")
	sig := verify.Signature{KeyID: "k1", Algorithm: verify.AlgorithmEd25519,
		Bytes: ed25519.Sign(priv, valid)}

	_, err = source.ParseVerifiedMarketplace(ctx, ks, valid, sig)
	if err == nil {
		t.Fatal("ParseVerifiedMarketplace: expected error for cancelled context, got nil")
	}
	if code := errpkg.Code(err); code != "E_SIG_CTX" {
		t.Fatalf("ParseVerifiedMarketplace: got code %q, want E_SIG_CTX", code)
	}
}
