package source

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
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

func TestLoadPackageManifestFromDir_UnsignedRefusedByDefault(t *testing.T) {
	dir := t.TempDir()
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "metadata", "manifest", "valid.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	index := `{
  "id": "obedience-corp/official",
  "name": "Test",
  "schema_version": "1",
  "namespace": "obedience-corp",
  "packages": [{"id":"obedience-corp/festival","display_name":"F","class":"tool","channels":["stable"],"manifest_path":"package.json"}]
}`
	if err := os.WriteFile(filepath.Join(dir, manifestFilename), []byte(index), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = loadPackageManifestFromDir(context.Background(), dir, "official-obey", "obedience-corp/festival",
		DefaultVerifyOptions(nil, false))
	if err == nil {
		t.Fatal("unsigned manifest must refuse under default policy")
	}
}

func TestLoadPackageManifestFromDir_UnsignedAllowedWithFlag(t *testing.T) {
	dir := t.TempDir()
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "metadata", "manifest", "valid.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	index := `{
  "id": "obedience-corp/official",
  "name": "Test",
  "schema_version": "1",
  "namespace": "obedience-corp",
  "packages": [{"id":"obedience-corp/festival","display_name":"F","class":"tool","channels":["stable"],"manifest_path":"package.json"}]
}`
	if err := os.WriteFile(filepath.Join(dir, manifestFilename), []byte(index), 0o600); err != nil {
		t.Fatal(err)
	}
	var warn bytes.Buffer
	m, err := loadPackageManifestFromDir(context.Background(), dir, "official-obey", "obedience-corp/festival",
		DefaultVerifyOptions(&warn, true))
	if err != nil {
		t.Fatalf("allow-unverified should accept unsigned: %v", err)
	}
	if m.ID != "obedience-corp/festival" {
		t.Fatalf("id %q", m.ID)
	}
	if !strings.Contains(warn.String(), "UNVERIFIED") {
		t.Fatalf("expected loud warning, got %q", warn.String())
	}
}
