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
	if m.ID != "obedience-corp/festival" || m.Class != "product" {
		t.Fatalf("unexpected manifest header: %+v", m)
	}
	if len(m.ProvidesBinaries) != 2 {
		t.Fatalf("provides_binaries: got %d, want 2", len(m.ProvidesBinaries))
	}
	if len(m.Releases) != 1 {
		t.Fatalf("releases: got %d, want 1", len(m.Releases))
	}
	rel := m.Releases[0]
	if rel.Version != "0.2.10" || rel.Channel != "stable" {
		t.Fatalf("release header: %+v", rel)
	}
	if rel.Components["camp"] != "0.2.11" || rel.Components["fest"] != "0.4.5" {
		t.Fatalf("components: %+v", rel.Components)
	}
	if rel.Dependencies == nil || len(rel.Dependencies) != 0 {
		t.Fatalf("dependencies should be empty non-nil: %+v", rel.Dependencies)
	}
	if len(rel.Artifacts) != 1 || rel.Artifacts[0].Kind != "suite-archive" || rel.Artifacts[0].Arch != "all" {
		t.Fatalf("artifact: %+v", rel.Artifacts)
	}
	if len(rel.Install.Entries) != 2 || rel.Install.Entries[0].ExecutableName != "camp" {
		t.Fatalf("install entries: %+v", rel.Install.Entries)
	}
}

func TestParseManifest_InvalidVariants(t *testing.T) {
	cases := []string{
		"bad_version.json",
		"missing_release_field.json",
		"bad_arch.json",
		"bad_class.json",
		"unknown_field.json",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			raw := read(t, "..", "..", "testdata", "metadata", "manifest", name)
			_, err := metadata.ParseManifest(context.Background(), raw)
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
