package app

import (
	"context"
	"runtime"
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/source"
)

func TestAppDecideAction(t *testing.T) {
	cases := []struct {
		name       string
		hasReceipt bool
		cp         appCopy
		latest     string
		want       appAction
	}{
		{"no copy", false, appCopy{}, "0.1.8", appActionPlace},
		{"copy without receipt, same version", false, appCopy{Found: true, Version: "0.1.8"}, "0.1.8", appActionPlace},
		{"receipt, same version", true, appCopy{Found: true, Version: "0.1.8"}, "0.1.8", appActionCurrent},
		{"receipt, older on disk", true, appCopy{Found: true, Version: "0.1.7"}, "0.1.8", appActionPlace},
		{"on disk newer", true, appCopy{Found: true, Version: "0.2.0"}, "0.1.8", appActionLeave},
		{"self-updated, unknown version", true, appCopy{Found: true, UpdatedByApp: true}, "0.1.8", appActionLeave},
	}
	for _, tc := range cases {
		if got := decideAppAction(tc.hasReceipt, tc.cp, tc.latest); got != tc.want {
			t.Errorf("%s: got %d want %d", tc.name, got, tc.want)
		}
	}
}

func TestAppBundleFormatRejectsLinuxFormats(t *testing.T) {
	b := bundlePlatform{}
	ctx := context.Background()
	if f, err := b.format(ctx, "", nil); f != "app-bundle" || err != nil {
		t.Fatalf("default: %q %v", f, err)
	}
	for _, req := range []string{"deb", "appimage", "app-bundle"} {
		if _, err := b.format(ctx, req, nil); errpkg.Code(err) != "E_APP_FORMAT_UNSUPPORTED" {
			t.Errorf("--format %s: %v", req, err)
		}
	}
	if _, err := b.format(ctx, "", &appReceipt{Format: "deb"}); errpkg.Code(err) != "E_APP_FORMAT_UNSUPPORTED" {
		t.Errorf("receipt in another format: %v", err)
	}
}

func TestAppInstallerRefusesRoot(t *testing.T) {
	resolved := 0
	ai := appInstaller{
		goos: "darwin", euid: func() int { return 0 },
		resolve: func(context.Context, string, string, source.VerifyOptions) (appRelease, error) {
			resolved++
			return appRelease{}, nil
		},
	}
	_, err := ai.install(context.Background(), AppInstallOptions{})
	if errpkg.Code(err) != "E_APP_ELEVATION" || resolved != 0 {
		t.Fatalf("err=%v resolved=%d", err, resolved)
	}
}

func TestDefaultAppInstallerWiring(t *testing.T) {
	ai, err := newAppInstaller(context.Background(), nil)
	switch runtime.GOOS {
	case "darwin", "linux", "windows":
		if err != nil {
			// A host without a usable home or LOCALAPPDATA reports the platform error, never a panic.
			if errpkg.Code(err) != "E_APP_PLATFORM" {
				t.Fatalf("%v", err)
			}
		} else if ai.resolve == nil || ai.download == nil || ai.begin == nil || ai.refuseRunning == nil || ai.platform == nil {
			t.Fatal("wiring incomplete")
		}
	default:
		if errpkg.Code(err) != "E_APP_PLATFORM" {
			t.Fatalf("unsupported host: %v", err)
		}
	}
	b := newBundlePlatform("/Applications", "/Users/u/Applications", 501, nil, nil, nil)
	if b.info == nil || b.systemApps != "/Applications" || b.tildePath("/Users/u/Applications/Festival App.app") != "~/Applications/Festival App.app" {
		t.Fatalf("bundle platform: %+v", b)
	}
}
