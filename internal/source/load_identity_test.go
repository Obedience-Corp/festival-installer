package source

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/metadata"
	"github.com/Obedience-Corp/festival-installer/internal/verify"
)

func signedManifestSourceDir(t *testing.T, indexPackageID string) (string, verify.KeyStore) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "metadata", "manifest", "valid.json"))
	if err != nil {
		t.Fatalf("read manifest fixture: %v", err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	ks := verify.NewStaticStore(map[string]ed25519.PublicKey{"test-key": pub})

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), raw, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	sigJSON := `{"key_id":"test-key","algorithm":"ed25519","signature":"` +
		base64.StdEncoding.EncodeToString(ed25519.Sign(priv, raw)) + `"}`
	if err := os.WriteFile(filepath.Join(dir, "package.json.sig"), []byte(sigJSON), 0o600); err != nil {
		t.Fatalf("write sig: %v", err)
	}

	index := `{
  "id": "obedience-corp/official",
  "name": "Test Marketplace",
  "description": "test",
  "homepage": "https://example.com",
  "schema_version": "1",
  "namespace": "obedience-corp",
  "packages": [
    {
      "id": "` + indexPackageID + `",
      "display_name": "Test Package",
      "class": "tool",
      "summary": "test package",
      "host_runtimes": ["fest-cli"],
      "channels": ["latest"],
      "manifest_path": "package.json"
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(dir, manifestFilename), []byte(index), 0o600); err != nil {
		t.Fatalf("write index: %v", err)
	}
	return dir, ks
}

func TestLoadPackageManifestFromDir_RefusesIDMismatchDespiteValidSignature(t *testing.T) {
	dir, ks := signedManifestSourceDir(t, "obedience-corp/impostor")

	_, err := loadPackageManifestFromDir(context.Background(), dir, "obedience-corp/official", "obedience-corp/impostor",
		VerifyOptions{KeyStore: ks, Policy: metadata.PolicyRefuseByDefault})
	if err == nil {
		t.Fatal("expected refusal when a signed manifest's id does not match the requested package id")
	}
	if errors.Is(err, verify.ErrSignatureInvalid) {
		t.Fatalf("the signature must verify; the refusal has to come from the id binding, got %v", err)
	}
	if !errors.Is(err, ErrPackageIDMismatch) {
		t.Fatalf("expected ErrPackageIDMismatch, got %v", err)
	}
}

func TestLoadPackageManifestFromDir_AcceptsMatchingSignedManifest(t *testing.T) {
	dir, ks := signedManifestSourceDir(t, "obedience-corp/festival")

	m, err := loadPackageManifestFromDir(context.Background(), dir, "obedience-corp/official", "obedience-corp/festival",
		VerifyOptions{KeyStore: ks, Policy: metadata.PolicyRefuseByDefault})
	if err != nil {
		t.Fatalf("matching signed manifest should be accepted: %v", err)
	}
	if m.ID != "obedience-corp/festival" {
		t.Fatalf("unexpected manifest id: %q", m.ID)
	}
}
