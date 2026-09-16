package app

import (
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
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
