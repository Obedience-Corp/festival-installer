package metadata

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/verify"
)

func validManifest(t *testing.T) []byte {
	t.Helper()
	return read(t, "..", "..", "testdata", "metadata", "manifest", "valid.json")
}

func signedKeyStore(t *testing.T, raw []byte) (verify.KeyStore, verify.Signature) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	ks := verify.NewStaticStore(map[string]ed25519.PublicKey{"test-key": pub})
	sig := verify.Signature{KeyID: "test-key", Algorithm: verify.AlgorithmEd25519, Bytes: ed25519.Sign(priv, raw)}
	return ks, sig
}

func TestIngestManifest_TamperedSignedRefused(t *testing.T) {
	original := validManifest(t)
	ks, sig := signedKeyStore(t, original)
	tampered := append(append([]byte{}, original...), '\n')

	if _, err := ParseManifest(context.Background(), tampered); err != nil {
		t.Fatalf("baseline: the unverified parse path should accept the tampered manifest, got %v", err)
	}

	_, err := IngestManifest(context.Background(), ks, tampered, &sig, IngestOptions{})
	if err == nil {
		t.Fatal("expected IngestManifest to refuse a tampered signed manifest")
	}
	if !errors.Is(err, verify.ErrSignatureInvalid) {
		t.Fatalf("expected ErrSignatureInvalid, got %v", err)
	}
}

func TestIngestManifest_ValidSignedAccepted(t *testing.T) {
	original := validManifest(t)
	ks, sig := signedKeyStore(t, original)

	m, err := IngestManifest(context.Background(), ks, original, &sig, IngestOptions{})
	if err != nil {
		t.Fatalf("valid signed manifest should be accepted: %v", err)
	}
	if m.ID != "obedience-corp/festival" {
		t.Fatalf("unexpected manifest: %+v", m)
	}
}

func TestIngestManifest_UnknownKeyRefused(t *testing.T) {
	original := validManifest(t)
	_, sig := signedKeyStore(t, original)
	empty := verify.PinnedKeyStore()

	_, err := IngestManifest(context.Background(), empty, original, &sig, IngestOptions{})
	if !errors.Is(err, verify.ErrKeyNotFound) {
		t.Fatalf("expected ErrKeyNotFound for an untrusted signing key, got %v", err)
	}
}

func TestIngestManifest_UnsignedWarnAllow(t *testing.T) {
	original := validManifest(t)
	var warn bytes.Buffer

	m, err := IngestManifest(context.Background(), verify.PinnedKeyStore(), original, nil, IngestOptions{
		Policy:      PolicyWarnAllow,
		WarnWriter:  &warn,
		SourceLabel: "official-obey/festival",
	})
	if err != nil {
		t.Fatalf("unsigned manifest under WarnAllow should be accepted: %v", err)
	}
	if m.ID == "" {
		t.Fatal("expected a parsed manifest")
	}
	out := warn.String()
	if !strings.Contains(out, "UNVERIFIED") || !strings.Contains(out, "official-obey/festival") {
		t.Fatalf("expected a loud unverified warning naming the source, got %q", out)
	}
}

func TestIngestManifest_UnsignedRefuseByDefault(t *testing.T) {
	original := validManifest(t)

	_, err := IngestManifest(context.Background(), verify.PinnedKeyStore(), original, nil, IngestOptions{
		Policy: PolicyRefuseByDefault,
	})
	if !errors.Is(err, ErrUnverifiedRefused) {
		t.Fatalf("expected ErrUnverifiedRefused under refuse-by-default, got %v", err)
	}

	m, err := IngestManifest(context.Background(), verify.PinnedKeyStore(), original, nil, IngestOptions{
		Policy:          PolicyRefuseByDefault,
		AllowUnverified: true,
	})
	if err != nil {
		t.Fatalf("--allow-unverified should override refuse-by-default: %v", err)
	}
	if m.ID == "" {
		t.Fatal("expected a parsed manifest with the override")
	}
}
