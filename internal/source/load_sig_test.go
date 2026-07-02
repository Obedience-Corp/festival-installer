package source

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDetachedSignature_Absent(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	sig, err := loadDetachedSignature(manifestPath, "manifest.json")
	if err != nil {
		t.Fatalf("absent signature should not error: %v", err)
	}
	if sig != nil {
		t.Fatalf("expected nil signature when .sig is absent, got %+v", sig)
	}
}

func TestLoadDetachedSignature_Present(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	sigJSON := `{"key_id":"root-1","algorithm":"ed25519","signature":"` + base64.StdEncoding.EncodeToString([]byte("sig")) + `"}`
	if err := os.WriteFile(manifestPath+".sig", []byte(sigJSON), 0o600); err != nil {
		t.Fatalf("write sig: %v", err)
	}
	sig, err := loadDetachedSignature(manifestPath, "manifest.json")
	if err != nil {
		t.Fatalf("present signature: %v", err)
	}
	if sig == nil || sig.KeyID != "root-1" {
		t.Fatalf("expected parsed signature, got %+v", sig)
	}
}

func TestLoadDetachedSignature_Malformed(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(manifestPath+".sig", []byte("not json"), 0o600); err != nil {
		t.Fatalf("write sig: %v", err)
	}
	if _, err := loadDetachedSignature(manifestPath, "manifest.json"); err == nil {
		t.Fatal("expected an error for a malformed .sig file")
	}
}
