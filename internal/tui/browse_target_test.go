package tui

import (
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/app"
)

func TestBrowseInstallTargetPreservesProductIdentity(t *testing.T) {
	for _, tc := range []struct {
		id, class, want string
	}{
		{"obedience-corp/obey", "product", "obey"},
		{"obedience-corp/festival", "product", "festival"},
		{"obedience-corp/festival", "bundle", "festival"},
		{"obedience-corp/camp", "product", "festival"},
		{"obedience-corp/fest", "product", "festival"},
		{"acme/camp-example", "plugin", "camp-example"},
		{"acme/fest-example", "plugin", "fest-example"},
		{"acme/other-product", "product", "other-product"},
		{"acme/festival-helper", "plugin", "festival-helper"},
		{"obey", "product", "obey"},
	} {
		t.Run(tc.id+"/"+tc.class, func(t *testing.T) {
			if got := browseInstallTarget(app.BrowseEntry{ID: tc.id, Class: tc.class}); got != tc.want {
				t.Fatalf("catalog selection %s [%s] dispatches %q, want %q", tc.id, tc.class, got, tc.want)
			}
		})
	}
}
