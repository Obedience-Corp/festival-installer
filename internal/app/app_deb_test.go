package app

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

func TestLinuxPackageManagerDetection(t *testing.T) {
	only := func(paths ...string) func(string) bool {
		return func(p string) bool {
			for _, want := range paths {
				if p == want {
					return true
				}
			}
			return false
		}
	}
	cases := []struct {
		name   string
		exists func(string) bool
		want   string
	}{
		{"apt-get only", only("/usr/bin/apt-get"), "/usr/bin/apt-get"},
		{"dnf only", only("/usr/bin/dnf"), "/usr/bin/dnf"},
		{"pacman only", only("/usr/bin/pacman"), "/usr/bin/pacman"},
		{"zypper only", only("/usr/bin/zypper"), "/usr/bin/zypper"},
		{"none", only(), ""},
		{"apt-get and dnf", only("/usr/bin/apt-get", "/usr/bin/dnf"), "/usr/bin/apt-get"},
		{"pkexec makes no difference", only("/usr/bin/pkexec", "/usr/bin/dnf"), "/usr/bin/dnf"},
	}
	for _, tc := range cases {
		if got := linuxPackageManager(tc.exists); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestAppDebInstallArgvIsFixed(t *testing.T) {
	got := strings.Join(debInstallArgv("/tmp/x/Festival.App_0.2.0_amd64.deb"), " ")
	if got != "/usr/bin/pkexec /usr/bin/apt-get install --yes /tmp/x/Festival.App_0.2.0_amd64.deb" {
		t.Fatalf("argv = %q", got)
	}
}

func TestAppDebElevationErrors(t *testing.T) {
	const path = "/tmp/x/f.deb"
	cases := []struct {
		name   string
		code   int
		start  error
		want   string
		start2 string
		hints  bool
	}{
		{"pkexec missing", -1, exec.ErrNotFound, "E_APP_ELEVATION", "pkexec is not installed.", true},
		{"dismissed", 126, nil, "E_APP_ELEVATION", "The password prompt was dismissed.", true},
		{"no agent", 127, nil, "E_APP_ELEVATION", "No password prompt is available in this session.", true},
		{"apt failure", 100, nil, "E_APP_INSTALLER_FAILED", "apt-get exited 100", false},
	}
	for _, tc := range cases {
		err := debElevationError(tc.code, tc.start, path)
		if errpkg.Code(err) != tc.want || !strings.HasPrefix(err.Error(), tc.want+": "+tc.start2) {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		hasHints := strings.Contains(err.Error(), "--format appimage") && strings.Contains(err.Error(), "sudo apt-get install --yes "+path)
		if hasHints != tc.hints {
			t.Errorf("%s: hints present = %v, want %v: %v", tc.name, hasHints, tc.hints, err)
		}
	}
	if debElevationError(0, nil, path) != nil {
		t.Error("exit 0 must be nil")
	}
}

func TestAppDebDiscoverUsesProbe(t *testing.T) {
	probe := func(inst debInstall) func(context.Context) (debInstall, error) {
		return func(context.Context) (debInstall, error) { return inst, nil }
	}
	ctx := context.Background()
	cp, err := debPlacer{probe: probe(debInstall{Installed: true, Version: "0.1.7"})}.discover(ctx, nil)
	if err != nil || !cp.Found || cp.Version != "0.1.7" || cp.Location != "dpkg:festival-app" || cp.FromReceipt {
		t.Fatalf("%+v %v", cp, err)
	}
	if cp, _ := (debPlacer{probe: probe(debInstall{})}).discover(ctx, nil); cp.Found {
		t.Fatalf("not installed but found: %+v", cp)
	}
	cp, _ = debPlacer{probe: probe(debInstall{Installed: true, Version: "0.1.8"})}.discover(ctx, &appReceipt{Version: "0.1.7", Format: "deb"})
	if !cp.UpdatedByApp || !cp.FromReceipt {
		t.Fatalf("self-update not detected: %+v", cp)
	}
}
