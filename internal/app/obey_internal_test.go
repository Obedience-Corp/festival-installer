package app

import (
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/release"
)

func TestDeclaredBinaries_PrefersPluralThenSingularThenSegment(t *testing.T) {
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
		{
			name:      "package id last segment when neither is declared",
			src:       release.Source{},
			packageID: "obedience-corp/obey",
			want:      []string{"obey"},
		},
		{
			name:      "unqualified package id is its own segment",
			src:       release.Source{},
			packageID: "obey",
			want:      []string{"obey"},
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

func TestDeclaredBinaries_ErrorsWhenNothingIsDeclarable(t *testing.T) {
	cases := map[string]string{
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
