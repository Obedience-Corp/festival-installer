package app

import (
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

func TestValidateAppFormat(t *testing.T) {
	cases := []struct {
		goos, format string
		want         string
	}{
		{"darwin", "app-bundle", ""},
		{"darwin", "nsis", "E_APP_FORMAT_UNSUPPORTED"},
		{"darwin", "deb", "E_APP_FORMAT_UNSUPPORTED"},
		{"darwin", "appimage", "E_APP_FORMAT_UNSUPPORTED"},
		{"windows", "nsis", ""},
		{"windows", "deb", "E_APP_FORMAT_UNSUPPORTED"},
		{"windows", "app-bundle", "E_APP_FORMAT_UNSUPPORTED"},
		{"linux", "deb", ""},
		{"linux", "appimage", ""},
		{"linux", "app-bundle", "E_APP_FORMAT_UNSUPPORTED"},
		{"linux", "nsis", "E_APP_FORMAT_UNSUPPORTED"},
		{"linux", "rpm", "E_APP_FORMAT_UNSUPPORTED"},
		{"linux", "", "E_APP_FORMAT_UNSUPPORTED"},
		{"freebsd", "appimage", "E_APP_PLATFORM"},
		{"", "deb", "E_APP_PLATFORM"},
	}
	for _, tc := range cases {
		err := validateAppFormat(tc.goos, tc.format)
		if tc.want == "" {
			if err != nil {
				t.Errorf("%s/%s: unexpected error %v", tc.goos, tc.format, err)
			}
			continue
		}
		if got := errpkg.Code(err); err == nil || got != tc.want {
			t.Errorf("%s/%s: code %q (%v), want %s", tc.goos, tc.format, got, err, tc.want)
		}
	}
}
