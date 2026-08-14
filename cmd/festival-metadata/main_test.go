package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/verify"
)

func TestGenerateSignVerifyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	privatePath := filepath.Join(dir, "private.key")
	manifestPath := filepath.Join(dir, "manifest.json")
	fixture, err := os.ReadFile(filepath.Join("..", "..", "testdata", "metadata", "manifest", "valid.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, fixture, 0o644); err != nil {
		t.Fatal(err)
	}

	var publicOut bytes.Buffer
	if err := run([]string{"generate-key", "--private-key", privatePath}, &publicOut, &bytes.Buffer{}); err != nil {
		t.Fatalf("generate-key: %v", err)
	}
	info, err := os.Stat(privatePath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("private key mode: got %o, want 600", got)
	}

	if err := run([]string{"sign", "--private-key", privatePath, "--key-id", "official-2026-01", manifestPath}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("sign: %v", err)
	}
	canonical, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	wantCanonical, err := verify.CanonicalizeJSON(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonical, wantCanonical) {
		t.Fatalf("canonical manifest mismatch\n got: %s\nwant: %s", canonical, wantCanonical)
	}

	if err := run([]string{"verify", "--public-key", strings.TrimSpace(publicOut.String()), manifestPath}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("verify: %v", err)
	}

	tampered := bytes.Replace(canonical, []byte(`"display_name":"Festival"`), []byte(`"display_name":"Festivbl"`), 1)
	if bytes.Equal(tampered, canonical) {
		t.Fatal("test fixture did not contain the tamper target")
	}
	if err := os.WriteFile(manifestPath, tampered, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"verify", "--public-key", strings.TrimSpace(publicOut.String()), manifestPath}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("tampered manifest unexpectedly verified")
	}
}

func TestSignRejectsLoosePrivateKeyPermissions(t *testing.T) {
	dir := t.TempDir()
	privatePath := filepath.Join(dir, "private.key")
	manifestPath := filepath.Join(dir, "manifest.json")
	var publicOut bytes.Buffer
	if err := run([]string{"generate-key", "--private-key", privatePath}, &publicOut, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(privatePath, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, []byte(`{"a":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"sign", "--private-key", privatePath, "--key-id", "official-2026-01", manifestPath}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("sign accepted a group/world-readable private key")
	}
}
