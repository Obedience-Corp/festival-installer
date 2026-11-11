package metadata

import (
	"context"
	"errors"
	"testing"
)

func TestParseManifestAppKinds(t *testing.T) {
	raw := read(t, "..", "..", "testdata", "metadata", "manifest", "app_valid.json")
	m, err := ParseManifest(context.Background(), raw)
	if err != nil {
		t.Fatalf("app manifest rejected: %v", err)
	}
	rel := m.Releases[0]
	if len(rel.Artifacts) != 5 {
		t.Fatalf("want 5 artifacts, got %d", len(rel.Artifacts))
	}
	for _, a := range rel.Artifacts {
		if !IsAppArtifactKind(a.Kind) {
			t.Errorf("artifact kind %q is not an App kind", a.Kind)
		}
		if a.UpdaterSignature == "" {
			t.Errorf("artifact %s has no updater signature", a.Filename)
		}
	}
	e := rel.Install.Entries[0]
	if e.Kind != InstallKindDesktopApp || e.BundleID != "com.obediencecorp.festival-app" ||
		e.ProductName != "Festival App" || e.Source != "festival-app" {
		t.Fatalf("unexpected desktop_app entry: %+v", e)
	}
}

func TestParseManifestAppInvalidVariants(t *testing.T) {
	cases := []struct{ file, field string }{
		{"app_missing_bundle_id.json", "/releases/0/install/entries/0"},
		{"app_bundle_id_on_binary.json", "/releases/0/install/entries/0/bundle_id"},
		{"app_signature_on_suite_artifact.json", "/releases/0/artifacts/0/updater_signature"},
		{"app_bad_kind.json", "/releases/0/artifacts/3/kind"},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			raw := read(t, "..", "..", "testdata", "metadata", "manifest", tc.file)
			_, err := ParseManifest(context.Background(), raw)
			if !errors.Is(err, ErrSchemaInvalid) {
				t.Fatalf("expected ErrSchemaInvalid, got %v", err)
			}
			var se *SchemaError
			if !errors.As(err, &se) {
				t.Fatalf("expected *SchemaError, got %T", err)
			}
			if se.Field() != tc.field {
				t.Fatalf("field = %q, want %q (%v)", se.Field(), tc.field, err)
			}
		})
	}
}

func TestParseManifestSuiteFixtureUnchanged(t *testing.T) {
	raw := read(t, "..", "..", "testdata", "metadata", "manifest", "valid.json")
	m, err := ParseManifest(context.Background(), raw)
	if err != nil {
		t.Fatalf("suite manifest rejected: %v", err)
	}
	for _, a := range m.Releases[0].Artifacts {
		if IsAppArtifactKind(a.Kind) || a.UpdaterSignature != "" {
			t.Errorf("suite artifact %+v carries App data", a)
		}
	}
}
