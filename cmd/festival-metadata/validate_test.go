package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/metadata"
)

func TestValidateKind(t *testing.T) {
	manifestFixture := readTestdata(t, "metadata", "manifest", "valid.json")
	marketplaceFixture := readTestdata(t, "marketplace", "valid.json")
	indexFixture := readTestdata(t, "metadata", "index", "valid.json")
	sourceFixture := readTestdata(t, "metadata", "source", "valid.json")

	t.Run("unknown kind", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "does-not-exist.json")
		err := run([]string{"validate", "--kind", "bogus", missing}, &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected error for unknown --kind")
		}
		for _, want := range []string{"manifest", "marketplace", "index", "source"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error %q does not name valid kind %q", err, want)
			}
		}
	})

	t.Run("missing document", func(t *testing.T) {
		err := run([]string{"validate"}, &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected error for missing document")
		}
		if !strings.Contains(err.Error(), "validate requires one document path") {
			t.Fatalf("error %q", err)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "does-not-exist.json")
		err := run([]string{"validate", missing}, &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected error for missing file")
		}
		if !strings.Contains(err.Error(), missing) {
			t.Fatalf("error %q does not name %q", err, missing)
		}
	})

	t.Run("schema invalid", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "unknown.json")
		writeFile(t, path, readTestdata(t, "metadata", "manifest", "unknown_field.json"))
		err := run([]string{"validate", "--kind", "manifest", path}, &bytes.Buffer{}, &bytes.Buffer{})
		if !errors.Is(err, metadata.ErrSchemaInvalid) {
			t.Fatalf("expected metadata.ErrSchemaInvalid, got %v", err)
		}
	})

	t.Run("wrong kind", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "marketplace.json")
		writeFile(t, path, marketplaceFixture)
		err := run([]string{"validate", "--kind", "manifest", path}, &bytes.Buffer{}, &bytes.Buffer{})
		if !errors.Is(err, metadata.ErrSchemaInvalid) {
			t.Fatalf("expected metadata.ErrSchemaInvalid, got %v", err)
		}
	})

	t.Run("non-canonical still validates", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "pretty.json")
		pretty := []byte("{\n  \"id\": \"ns/mp\",\n  \"name\": \"Test Marketplace\",\n  \"schema_version\": \"1\",\n  \"packages\": []\n}\n")
		writeFile(t, path, pretty)
		var stdout bytes.Buffer
		if err := run([]string{"validate", "--kind", "marketplace", path}, &stdout, &bytes.Buffer{}); err != nil {
			t.Fatalf("pretty JSON should validate without a signature: %v", err)
		}
		if !strings.Contains(stdout.String(), "validated") {
			t.Fatalf("stdout: %q", stdout.String())
		}
	})

	t.Run("empty published_at", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "empty-published-at.json")
		raw := bytes.Replace(
			manifestFixture,
			[]byte(`"published_at": "2026-06-08T00:00:00Z"`),
			[]byte(`"published_at": ""`),
			1,
		)
		if bytes.Equal(raw, manifestFixture) {
			t.Fatal("fixture did not contain published_at target")
		}
		writeFile(t, path, raw)
		err := run([]string{"validate", path}, &bytes.Buffer{}, &bytes.Buffer{})
		if !errors.Is(err, metadata.ErrSchemaInvalid) {
			t.Fatalf("expected schema invalid for empty published_at, got %v", err)
		}
		var se *metadata.SchemaError
		if !errors.As(err, &se) {
			t.Fatalf("expected *metadata.SchemaError, got %T", err)
		}
		if !strings.Contains(se.Field(), "published_at") {
			t.Fatalf("field %q does not name published_at", se.Field())
		}
	})

	t.Run("happy path", func(t *testing.T) {
		cases := []struct {
			kind string
			doc  []byte
		}{
			{"manifest", manifestFixture},
			{"marketplace", marketplaceFixture},
			{"index", indexFixture},
			{"source", sourceFixture},
		}
		for _, tc := range cases {
			t.Run(tc.kind, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), tc.kind+".json")
				writeFile(t, path, tc.doc)
				var stdout bytes.Buffer
				if err := run([]string{"validate", "--kind", tc.kind, path}, &stdout, &bytes.Buffer{}); err != nil {
					t.Fatalf("validate --kind %s: %v", tc.kind, err)
				}
				want := "validated " + path + " (" + tc.kind + ")\n"
				if stdout.String() != want {
					t.Fatalf("stdout: got %q want %q", stdout.String(), want)
				}
			})
		}
	})

	t.Run("default kind is manifest", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "manifest.json")
		writeFile(t, path, manifestFixture)
		if err := run([]string{"validate", path}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
			t.Fatalf("validate default kind: %v", err)
		}
	})

	t.Run("unsigned document does not need a signature", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "manifest.json")
		writeFile(t, path, manifestFixture)
		if err := run([]string{"validate", path}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
			t.Fatalf("validate unsigned: %v", err)
		}
		err := run([]string{"verify", "--pinned", path}, &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("verify should refuse an unsigned document")
		}
	})
}

func TestValidateByKind_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := validateByKind(ctx, "manifest", []byte(`{}`)); err == nil {
		t.Fatal("expected cancelled context error")
	}
}

func readTestdata(t *testing.T, parts ...string) []byte {
	t.Helper()
	path := filepath.Join(append([]string{"..", "..", "testdata"}, parts...)...)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
