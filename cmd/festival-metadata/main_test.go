package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/metadata"
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

// genKeyAndSign writes doc to <dir>/<name>.json, generates a fresh key pair,
// and signs the document with keyID. sign rewrites the document to its
// canonical form on disk, so docPath is canonical JSON on return.
func genKeyAndSign(t *testing.T, dir, name string, doc []byte, keyID string) (docPath, sigPath, pubKey string) {
	t.Helper()
	privatePath := filepath.Join(dir, name+"-private.key")
	docPath = filepath.Join(dir, name+".json")
	if err := os.WriteFile(docPath, doc, 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	var publicOut bytes.Buffer
	if err := run([]string{"generate-key", "--private-key", privatePath}, &publicOut, &bytes.Buffer{}); err != nil {
		t.Fatalf("generate-key: %v", err)
	}
	if err := run([]string{"sign", "--private-key", privatePath, "--key-id", keyID, docPath}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("sign: %v", err)
	}
	return docPath, docPath + ".sig", strings.TrimSpace(publicOut.String())
}

func TestVerifyKind(t *testing.T) {
	manifestFixture, err := os.ReadFile(filepath.Join("..", "..", "testdata", "metadata", "manifest", "valid.json"))
	if err != nil {
		t.Fatal(err)
	}
	marketplaceDoc := []byte(`{"id":"ns/mp","name":"Test Marketplace","schema_version":"1","packages":[]}`)
	indexDoc := []byte(`{"source":"official-obey","updatedAt":"2026-08-11T00:00:00Z","packages":[{"id":"ns/p","channels":["stable"]}]}`)
	sourceDoc := []byte(`{"id":"ns/src","name":"Test Source","indexUrl":"https://example.com/index.json","keys":{},"ttlSeconds":3600}`)

	// Case 1: unknown kind is rejected before any file read.
	t.Run("unknown kind", func(t *testing.T) {
		dir := t.TempDir()
		missing := filepath.Join(dir, "does-not-exist.json")
		err := run([]string{"verify", "--pinned", "--kind", "bogus", missing}, &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected error for unknown --kind")
		}
		for _, want := range []string{"manifest", "marketplace", "index", "source"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error %q does not name valid kind %q", err, want)
			}
		}
	})

	// Case 2: wrong kind for the document produces a schema error, not a
	// signature error. The signature and key are correct; only the shape is
	// wrong.
	t.Run("wrong kind for the document", func(t *testing.T) {
		dir := t.TempDir()
		docPath, _, pub := genKeyAndSign(t, dir, "marketplace", marketplaceDoc, "k1")
		err := run([]string{"verify", "--public-key", pub, "--kind", "manifest", docPath}, &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected schema error verifying a marketplace document as manifest")
		}
		if errors.Is(err, verify.ErrSignatureInvalid) || errors.Is(err, verify.ErrKeyNotFound) {
			t.Fatalf("expected a schema error, got a signature/key error: %v", err)
		}
		if !errors.Is(err, metadata.ErrSchemaInvalid) {
			t.Fatalf("expected metadata.ErrSchemaInvalid, got %v", err)
		}
	})

	// Case 3: a missing .sig file names the signature path it looked for.
	t.Run("missing sig", func(t *testing.T) {
		dir := t.TempDir()
		docPath, sigPath, pub := genKeyAndSign(t, dir, "marketplace", marketplaceDoc, "k1")
		if err := os.Remove(sigPath); err != nil {
			t.Fatal(err)
		}
		err := run([]string{"verify", "--public-key", pub, "--kind", "marketplace", docPath}, &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected error for missing .sig")
		}
		if !strings.Contains(err.Error(), sigPath) {
			t.Fatalf("error %q does not mention signature path %q", err, sigPath)
		}
	})

	// Case 4: non-canonical input is refused, naming the offending path. The
	// canonical check runs before the signature is even read, so no valid
	// .sig is required to exercise it.
	t.Run("non-canonical input", func(t *testing.T) {
		dir := t.TempDir()
		prettyPath := filepath.Join(dir, "marketplace-pretty.json")
		pretty := []byte("{\n  \"id\": \"ns/mp\",\n  \"name\": \"Test Marketplace\",\n  \"schema_version\": \"1\",\n  \"packages\": []\n}\n")
		if err := os.WriteFile(prettyPath, pretty, 0o644); err != nil {
			t.Fatal(err)
		}
		err := run([]string{"verify", "--pinned", "--kind", "marketplace", prettyPath}, &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected non-canonical JSON error")
		}
		if !strings.Contains(err.Error(), "is not canonical JSON") {
			t.Fatalf("error %q does not report non-canonical JSON", err)
		}
		if !strings.Contains(err.Error(), prettyPath) {
			t.Fatalf("error %q does not name the offending path %q", err, prettyPath)
		}
	})

	// Case 5: unknown key id. --public-key builds its key store from
	// whatever key id is in the .sig, so that flow can never miss a lookup;
	// --pinned is the only way an unknown key id is reachable, by signing
	// with a key that was never registered in the pinned trust store.
	t.Run("unknown key id", func(t *testing.T) {
		dir := t.TempDir()
		docPath, _, _ := genKeyAndSign(t, dir, "marketplace", marketplaceDoc, "not-the-pinned-key")
		err := run([]string{"verify", "--pinned", "--kind", "marketplace", docPath}, &bytes.Buffer{}, &bytes.Buffer{})
		if !errors.Is(err, verify.ErrKeyNotFound) {
			t.Fatalf("expected verify.ErrKeyNotFound, got %v", err)
		}
	})

	// Case 6: happy path, one signed fixture per kind.
	t.Run("happy path", func(t *testing.T) {
		cases := []struct {
			kind string
			doc  []byte
		}{
			{"manifest", manifestFixture},
			{"marketplace", marketplaceDoc},
			{"index", indexDoc},
			{"source", sourceDoc},
		}
		for _, tc := range cases {
			t.Run(tc.kind, func(t *testing.T) {
				dir := t.TempDir()
				docPath, _, pub := genKeyAndSign(t, dir, tc.kind, tc.doc, "k1")
				var stdout bytes.Buffer
				if err := run([]string{"verify", "--public-key", pub, "--kind", tc.kind, docPath}, &stdout, &bytes.Buffer{}); err != nil {
					t.Fatalf("verify --kind %s: %v", tc.kind, err)
				}
				want := "verified " + docPath + " with k1\n"
				if stdout.String() != want {
					t.Fatalf("stdout: got %q want %q", stdout.String(), want)
				}
			})
		}
	})
}
