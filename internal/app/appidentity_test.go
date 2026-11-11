package app

import (
	"context"
	"errors"
	"io/fs"
	"strconv"
	"strings"
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

// Info.plist of Festival.App_0.1.7_aarch64.app.tar.gz (obey-releases app-test-25628d12...).
const festivalApp017Plist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleDevelopmentRegion</key>
	<string>English</string>
	<key>CFBundleDisplayName</key>
	<string>Festival App</string>
	<key>CFBundleExecutable</key>
	<string>festival-app</string>
	<key>CFBundleIconFile</key>
	<string>icon.icns</string>
	<key>CFBundleIdentifier</key>
	<string>com.obediencecorp.festival-app</string>
	<key>CFBundleInfoDictionaryVersion</key>
	<string>6.0</string>
	<key>CFBundleName</key>
	<string>Festival App</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleShortVersionString</key>
	<string>0.1.7</string>
	<key>CFBundleVersion</key>
	<string>0.1.7</string>
	<key>CSResourcesFileMapped</key>
	<true/>
	<key>LSMinimumSystemVersion</key>
	<string>12.0</string>
	<key>LSRequiresCarbon</key>
	<true/>
	<key>NSHighResolutionCapable</key>
	<true/>
	<key>ASWebAuthenticationSessionWebBrowserSupportCapabilities</key>
	<dict>
		<key>IsSupported</key>
		<true/>
		<key>EphemeralBrowserSessionIsSupported</key>
		<true/>
		<key>CallbackURLMatchingIsSupported</key>
		<true/>
		<key>AdditionalHeaderFieldsAreSupported</key>
		<true/>
	</dict>
	<key>NSBluetoothAlwaysUsageDescription</key>
	<string>Festival App uses Bluetooth for nearby passkey and security-key sign-in flows.</string>
	<key>NSCameraUsageDescription</key>
	<string>Festival App uses the camera for websites that request camera access.</string>
	<key>NSMicrophoneUsageDescription</key>
	<string>Festival App uses the microphone for websites that request microphone access.</string>
</dict>
</plist>`

func TestParseInfoPlistFestivalApp017(t *testing.T) {
	info, err := parseInfoPlist(strings.NewReader(festivalApp017Plist))
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "com.obediencecorp.festival-app" || info.ShortVersion != "0.1.7" || info.Executable != "festival-app" {
		t.Fatalf("%+v", info)
	}
}

func TestParseInfoPlistIgnoresNestedDict(t *testing.T) {
	plist := `<plist><dict><key>CFBundleIdentifier</key><string>real.id</string>
<key>Nested</key><dict><key>CFBundleIdentifier</key><string>nested.id</string><key>CFBundleShortVersionString</key><string>9.9.9</string></dict>
<key>CFBundleShortVersionString</key><string>1.2.3</string></dict></plist>`
	info, err := parseInfoPlist(strings.NewReader(plist))
	if err != nil || info.ID != "real.id" || info.ShortVersion != "1.2.3" {
		t.Fatalf("%+v %v", info, err)
	}
}

func TestParseInfoPlistRejectsBinary(t *testing.T) {
	_, err := parseInfoPlist(strings.NewReader("bplist00\x00\x01"))
	if errpkg.Code(err) != "E_APP_BUNDLE_INVALID" {
		t.Fatalf("%v", err)
	}
}

func TestParseInfoPlistMissingIdentifier(t *testing.T) {
	_, err := parseInfoPlist(strings.NewReader("<plist><dict><key>CFBundleName</key><string>x</string></dict></plist>"))
	if errpkg.Code(err) != "E_APP_BUNDLE_INVALID" {
		t.Fatalf("%v", err)
	}
}

type fakeRegistry map[string]string

func (f fakeRegistry) GetStringValue(name string) (string, uint32, error) {
	v, ok := f[name]
	if !ok {
		return "", 0, errors.New("not exist")
	}
	return v, 1, nil
}

func TestParseNSISRegistration(t *testing.T) {
	good := fakeRegistry{
		"DisplayName": "Festival App", "DisplayVersion": "0.1.7", "MainBinaryName": "festival-app.exe",
		"InstallLocation": `"C:\Users\u\AppData\Local\Festival App"`,
		"UninstallString": `"C:\Users\u\AppData\Local\Festival App\uninstall.exe"`,
	}
	reg, err := parseNSISRegistration(good)
	if err != nil || reg.InstallLocation != `C:\Users\u\AppData\Local\Festival App` || reg.UninstallString != `C:\Users\u\AppData\Local\Festival App\uninstall.exe` || reg.DisplayVersion != "0.1.7" {
		t.Fatalf("%+v %v", reg, err)
	}
	delete(good, "DisplayName")
	if _, err := parseNSISRegistration(good); errpkg.Code(err) != "E_APP_FOREIGN" {
		t.Fatalf("missing DisplayName: %v", err)
	}
	good["DisplayName"] = "Other App"
	if _, err := parseNSISRegistration(good); errpkg.Code(err) != "E_APP_FOREIGN" {
		t.Fatalf("other DisplayName: %v", err)
	}
}

func TestUnquoteRegistryPath(t *testing.T) {
	for in, want := range map[string]string{
		`"C:\a b"`: `C:\a b`, `  "C:\a"  `: `C:\a`, `C:\plain`: `C:\plain`, `"`: `"`, "": "",
	} {
		if got := unquoteRegistryPath(in); got != want {
			t.Errorf("unquote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseDpkgQuery(t *testing.T) {
	cases := []struct {
		in      string
		want    debInstall
		wantErr bool
	}{
		{"install ok installed\t0.1.7\n", debInstall{true, "0.1.7"}, false},
		{"deinstall ok config-files\t0.1.7\n", debInstall{}, false},
		{"unknown ok not-installed\t\n", debInstall{}, false},
		{"garbage", debInstall{}, true},
	}
	for _, tc := range cases {
		got, err := parseDpkgQuery(tc.in)
		if got != tc.want || (err != nil) != tc.wantErr {
			t.Errorf("parseDpkgQuery(%q) = %+v %v", tc.in, got, err)
		}
	}
}

type fakeExit int

func (e fakeExit) Error() string { return "exit status " + strconv.Itoa(int(e)) }
func (e fakeExit) ExitCode() int { return int(e) }

func TestProbeDebOutcomes(t *testing.T) {
	var gotName string
	var gotArgs []string
	probe := func(out string, err error) (debInstall, error) {
		return probeDeb(context.Background(), func(_ context.Context, name string, args ...string) (string, error) {
			gotName, gotArgs = name, args
			return out, err
		})
	}
	if d, err := probe("", fakeExit(1)); d.Installed || err != nil {
		t.Errorf("exit 1: %+v %v", d, err)
	}
	if d, err := probe("", &fs.PathError{Op: "fork/exec", Path: "/usr/bin/dpkg-query", Err: fs.ErrNotExist}); d.Installed || err != nil {
		t.Errorf("missing dpkg-query: %+v %v", d, err)
	}
	if _, err := probe("", fakeExit(2)); err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Errorf("exit 2 must return the error: %v", err)
	}
	if d, err := probe("install ok installed\t0.1.7\n", nil); !d.Installed || d.Version != "0.1.7" || err != nil {
		t.Errorf("installed: %+v %v", d, err)
	}
	want := []string{"-W", "-f", "${Status}\t${Version}\n", "festival-app"}
	if gotName != "/usr/bin/dpkg-query" || strings.Join(gotArgs, "|") != strings.Join(want, "|") {
		t.Errorf("runner got %q %q", gotName, gotArgs)
	}
}

func TestAppMainExecutable(t *testing.T) {
	cases := []struct{ format, loc, want string }{
		{"app-bundle", "/Applications/Festival App.app", "/Applications/Festival App.app/Contents/MacOS/festival-app"},
		{"nsis", `C:\Festival App`, `C:\Festival App\festival-app.exe`},
		{"deb", "dpkg:festival-app", "/usr/bin/festival-app"},
		{"appimage", "/home/u/.local/bin/festival-app", "/home/u/.local/bin/festival-app"},
	}
	for _, tc := range cases {
		if got, err := appMainExecutable(tc.format, tc.loc); got != tc.want || err != nil {
			t.Errorf("%s: %q %v", tc.format, got, err)
		}
	}
	if _, err := appMainExecutable("rpm", "/x"); errpkg.Code(err) != "E_APP_FORMAT_UNSUPPORTED" {
		t.Errorf("unknown format: %v", err)
	}
}

func TestForeignAppError(t *testing.T) {
	err := foreignAppError("/Applications/Festival App.app", "different bundle id")
	if errpkg.Code(err) != "E_APP_FOREIGN" || !strings.Contains(err.Error(), "/Applications/Festival App.app") {
		t.Fatalf("%v", err)
	}
}
