package app

import (
	"strings"
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/metadata"
	"github.com/Obedience-Corp/festival-installer/internal/release"
)

func TestDeclaredBinaries_PrefersPluralThenSingular(t *testing.T) {
	cases := []struct {
		name      string
		src       release.Source
		packageID string
		want      []string
	}{
		{
			name:      "plural wins over singular",
			src:       release.Source{Binaries: []string{"obey", "ob"}, Binary: "obey"},
			packageID: "obedience-corp/obey",
			want:      []string{"obey", "ob"},
		},
		{
			name:      "singular when no plural",
			src:       release.Source{Binary: "fest-demo"},
			packageID: "acme/fest-demo",
			want:      []string{"fest-demo"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := declaredBinaries(&tc.src, tc.packageID)
			if err != nil {
				t.Fatalf("declaredBinaries: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// A release_source that declares no executable is a marketplace bug. The
// package id is never mined for a binary name, so even an id that reads like
// one errors instead of being installed.
func TestDeclaredBinaries_ErrorsWhenNothingIsDeclared(t *testing.T) {
	cases := map[string]string{
		"qualified package id":   "obedience-corp/obey",
		"unqualified package id": "obey",
		"empty package id":       "",
		"package id ends in sep": "obedience-corp/",
	}
	for name, packageID := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := declaredBinaries(&release.Source{}, packageID)
			if err == nil {
				t.Fatalf("expected an error, got %v", got)
			}
			if code := errpkg.Code(err); code != "E_PRODUCT_NO_BINARIES" {
				t.Fatalf("code = %q, want E_PRODUCT_NO_BINARIES", code)
			}
		})
	}
}

// The product path places binaries. A manifest that also declares a skill
// bundle or an extension is refused by name rather than half installed under a
// receipt that claims the whole release landed.
func TestManifestBinaries_RefusesEntriesItCannotPlace(t *testing.T) {
	binaries := metadata.Release{
		Version: "0.2.0",
		Install: metadata.Install{Entries: []metadata.InstallEntry{
			{Kind: "binary", Source: "obey"},
			{Kind: "binary", Source: "obey-ob", ExecutableName: "ob"},
		}},
	}
	got, err := manifestBinaries(binaries, "obedience-corp/obey")
	if err != nil {
		t.Fatalf("manifestBinaries: %v", err)
	}
	if len(got) != 2 || got[0] != "obey" || got[1] != "ob" {
		t.Fatalf("got %v, want [obey ob]", got)
	}

	cases := map[string]metadata.InstallEntry{
		"skill bundle": {Kind: "skill_bundle", Source: "skills", SkillSlug: "obey"},
		"extension":    {Kind: "extension", Source: "ext", ExtensionName: "obey"},
		"kindless":     {Source: "mystery"},
	}
	for name, entry := range cases {
		t.Run(name, func(t *testing.T) {
			rel := metadata.Release{
				Version: "0.2.0",
				Install: metadata.Install{Entries: []metadata.InstallEntry{
					{Kind: "binary", Source: "obey"},
					entry,
				}},
			}
			_, err := manifestBinaries(rel, "obedience-corp/obey")
			if err == nil {
				t.Fatal("expected a refusal rather than a silent drop")
			}
			if code := errpkg.Code(err); code != "E_PRODUCT_UNSUPPORTED_ENTRY" {
				t.Fatalf("code = %q, want E_PRODUCT_UNSUPPORTED_ENTRY", code)
			}
			if !strings.Contains(err.Error(), entryKindLabel(entry)) {
				t.Fatalf("error = %q, want it to name the kind it cannot place", err)
			}
		})
	}
}

// A bare-binary artifact is refused for what it is. The old message blamed the
// number of binaries, which is wrong for a product that declares exactly one.
func TestRequireArchive_NamesTheArtifactNotTheBinaryCount(t *testing.T) {
	single := productResolved{
		version:  "0.2.0",
		url:      "https://example.test/dl/obey-0.2.0-macOS-arm64",
		binaries: []string{"obey"},
	}
	err := requireArchive("obedience-corp/obey", single)
	if err == nil {
		t.Fatal("expected a bare binary to be refused")
	}
	if code := errpkg.Code(err); code != "E_PRODUCT_NOT_ARCHIVE" {
		t.Fatalf("code = %q, want E_PRODUCT_NOT_ARCHIVE", code)
	}
	if strings.Contains(err.Error(), "more than one binary") {
		t.Fatalf("error = %q, must not blame the binary count for a single-binary product", err)
	}
	if !strings.Contains(err.Error(), "obey-0.2.0-macOS-arm64") {
		t.Fatalf("error = %q, want it to name the artifact that was found", err)
	}

	archive := single
	archive.archive = true
	if err := requireArchive("obedience-corp/obey", archive); err != nil {
		t.Fatalf("an archive must be accepted: %v", err)
	}
}
