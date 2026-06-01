package source_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Obedience-Corp/obey-installer/internal/source"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "marketplace", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return raw
}

func TestParseMarketplace_Valid(t *testing.T) {
	ctx := context.Background()
	m, err := source.ParseMarketplace(ctx, readFixture(t, "valid.json"))
	if err != nil {
		t.Fatalf("ParseMarketplace: %v", err)
	}
	if m.ID != "obedience-corp/official" {
		t.Fatalf("id: got %q", m.ID)
	}
	if len(m.Packages) != 2 {
		t.Fatalf("packages: got %d want 2", len(m.Packages))
	}
	if m.Packages[0].Class != "tool" || m.Packages[1].Class != "plugin" {
		t.Fatalf("classes: %q %q", m.Packages[0].Class, m.Packages[1].Class)
	}
}

func TestParseMarketplace_InvalidVariants(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		file  string
		field string
	}{
		{"missing_required.json", ""},
		{"wrong_type.json", "/schema_version"},
		{"unknown_field.json", ""},
		{"bad_class.json", "/packages/0/class"},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			_, err := source.ParseMarketplace(ctx, readFixture(t, tc.file))
			if !errors.Is(err, source.ErrManifestInvalid) {
				t.Fatalf("expected ErrManifestInvalid, got %v", err)
			}
			if tc.field != "" {
				var merr *source.ManifestError
				if !errors.As(err, &merr) {
					t.Fatalf("expected *ManifestError, got %T", err)
				}
				if merr.Field() != tc.field {
					t.Fatalf("field: got %q want %q", merr.Field(), tc.field)
				}
			}
		})
	}
}

func TestLoadMarketplace_Valid(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "obey-marketplace.json"), readFixture(t, "valid.json"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	m, err := source.LoadMarketplace(ctx, dir)
	if err != nil {
		t.Fatalf("LoadMarketplace: %v", err)
	}
	if len(m.Packages) != 2 {
		t.Fatalf("packages: got %d want 2", len(m.Packages))
	}
}

func TestLoadMarketplace_MissingFile(t *testing.T) {
	ctx := context.Background()
	if _, err := source.LoadMarketplace(ctx, t.TempDir()); !errors.Is(err, source.ErrNotAMarketplace) {
		t.Fatalf("expected ErrNotAMarketplace, got %v", err)
	}
}
