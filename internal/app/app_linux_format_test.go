package app

import (
	"strings"
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

func TestAppLinuxFormatChoice(t *testing.T) {
	appimageRcpt := &appReceipt{Format: "appimage", Location: "/h/.local/bin/festival-app"}
	debRcpt := &appReceipt{Format: "deb", Location: "dpkg:festival-app"}
	cases := []struct {
		name      string
		requested string
		existing  *appReceipt
		hasApt    bool
		debInst   bool
		want      string
		wantErr   string
	}{
		{"none, apt", "", nil, true, false, "deb", ""},
		{"none, apt, dpkg has it", "", nil, true, true, "deb", ""},
		{"none, no apt, no dpkg", "", nil, false, false, "appimage", ""},
		{"appimage requested, apt, no dpkg (R14)", "appimage", nil, true, false, "appimage", ""},
		{"deb requested, no apt", "deb", nil, false, false, "", "--format deb needs apt-get (Debian or Ubuntu family)"},
		{"deb requested, apt", "deb", nil, true, false, "deb", ""},
		{"receipt appimage, none requested", "", appimageRcpt, true, false, "appimage", ""},
		{"receipt deb, same requested", "deb", debRcpt, true, true, "deb", ""},
		{"receipt appimage, deb requested", "deb", appimageRcpt, true, false, "", "Festival App is installed as appimage at /h/.local/bin/festival-app. Run festival uninstall app first to switch formats."},
		{"receipt deb, appimage requested", "appimage", debRcpt, true, true, "", "Festival App is installed as deb at dpkg:festival-app. Run festival uninstall app first to switch formats."},
		{"appimage requested, dpkg has it, no receipt", "appimage", nil, true, true, "", "Festival App is installed as a deb package. Remove it with sudo apt-get remove festival-app before installing the AppImage."},
		{"rpm", "rpm", nil, true, false, "", "--format must be appimage or deb"},
		{"rpm with receipt", "rpm", appimageRcpt, true, false, "", "--format must be appimage or deb"},
	}
	for _, tc := range cases {
		got, err := chooseLinuxAppFormat(tc.requested, tc.existing, tc.hasApt, tc.debInst)
		if tc.wantErr != "" {
			if errpkg.Code(err) != "E_APP_FORMAT_UNSUPPORTED" || !strings.HasSuffix(err.Error(), tc.wantErr) {
				t.Errorf("%s: err = %v, want %q", tc.name, err, tc.wantErr)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("%s: got %q %v want %q", tc.name, got, err, tc.want)
		}
	}
}

func TestAppDesktopEntryExactContent(t *testing.T) {
	got, err := desktopEntry("/home/fixture/.local/bin/festival-app")
	want := `[Desktop Entry]
Type=Application
Name=Festival App
Exec="/home/fixture/.local/bin/festival-app"
TryExec=/home/fixture/.local/bin/festival-app
Icon=festival-app
StartupWMClass=festival-app
Terminal=false
Categories=Development;
X-Festival-Installer-Package=obedience-corp/festival-app
`
	if err != nil || got != want {
		t.Fatalf("got:\n%s\nerr=%v", got, err)
	}
}

func TestAppDesktopExecQuoting(t *testing.T) {
	cases := map[string]string{
		`/a b/festival-app`: `"/a b/festival-app"`,
		`/a$b`:              `"/a\\$b"`,
		`/a"b`:              `"/a\\"b"`,
		"/a`b":              "\"/a\\\\`b\"",
		`/a\b`:              `"/a\\\\b"`,
		`/a%b`:              `"/a%%b"`,
	}
	for in, want := range cases {
		if got, err := desktopExecQuote(in); err != nil || got != want {
			t.Errorf("quote(%q) = %q %v, want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"/a\nb", "/a\tb", "/a\x00b"} {
		if _, err := desktopExecQuote(bad); err == nil {
			t.Errorf("quote(%q) must fail", bad)
		}
	}
}

func TestAppXDGDataHome(t *testing.T) {
	env := func(v string) func(string) string { return func(string) string { return v } }
	if got := xdgDataHome(env("/xdg"), "/home/u"); got != "/xdg" {
		t.Errorf("absolute: %q", got)
	}
	if got := xdgDataHome(env("relative/dir"), "/home/u"); got != "/home/u/.local/share" {
		t.Errorf("relative: %q", got)
	}
	if got := xdgDataHome(env(""), "/home/u"); got != "/home/u/.local/share" {
		t.Errorf("empty: %q", got)
	}
}
