package metadata_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Obedience-Corp/obey-installer/internal/metadata"
)

func read(t *testing.T, parts ...string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(parts...))
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Join(parts...), err)
	}
	return b
}

func TestParseSource_Valid(t *testing.T) {
	raw := read(t, "..", "..", "testdata", "metadata", "source", "valid.json")
	s, err := metadata.ParseSource(context.Background(), raw)
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	if s.ID != "official-obey" || s.TTLSeconds != 3600 {
		t.Fatalf("unexpected source: %+v", s)
	}
}

func TestParseSource_InvalidVariants(t *testing.T) {
	cases := []string{"missing_required.json", "wrong_type.json", "unknown_field.json"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			raw := read(t, "..", "..", "testdata", "metadata", "source", name)
			_, err := metadata.ParseSource(context.Background(), raw)
			if !errors.Is(err, metadata.ErrSchemaInvalid) {
				t.Fatalf("expected ErrSchemaInvalid, got %v", err)
			}
			var se *metadata.SchemaError
			if !errors.As(err, &se) {
				t.Fatalf("expected *SchemaError, got %T", err)
			}
		})
	}
}

func TestParseIndex_Valid(t *testing.T) {
	raw := read(t, "..", "..", "testdata", "metadata", "index", "valid.json")
	idx, err := metadata.ParseIndex(context.Background(), raw)
	if err != nil {
		t.Fatalf("ParseIndex: %v", err)
	}
	if len(idx.Packages) != 2 {
		t.Fatalf("packages count: got %d, want 2", len(idx.Packages))
	}
}

func TestParseIndex_EmptyChannelsRejected(t *testing.T) {
	raw := read(t, "..", "..", "testdata", "metadata", "index", "empty_channels.json")
	_, err := metadata.ParseIndex(context.Background(), raw)
	if !errors.Is(err, metadata.ErrSchemaInvalid) {
		t.Fatalf("expected ErrSchemaInvalid, got %v", err)
	}
	var se *metadata.SchemaError
	if !errors.As(err, &se) {
		t.Fatalf("expected *SchemaError")
	}
	if se.Field() == "" {
		t.Fatalf("expected non-empty Field(), got empty")
	}
}

func TestParseManifest_Valid(t *testing.T) {
	raw := read(t, "..", "..", "testdata", "metadata", "manifest", "valid.json")
	m, err := metadata.ParseManifest(context.Background(), raw)
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if m.Version != "1.2.3" || len(m.Artifacts) != 1 {
		t.Fatalf("unexpected manifest: %+v", m)
	}
}

func TestParseManifest_BadVersionPattern(t *testing.T) {
	raw := read(t, "..", "..", "testdata", "metadata", "manifest", "bad_version.json")
	_, err := metadata.ParseManifest(context.Background(), raw)
	if !errors.Is(err, metadata.ErrSchemaInvalid) {
		t.Fatalf("expected ErrSchemaInvalid, got %v", err)
	}
}
