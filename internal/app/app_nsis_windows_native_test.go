//go:build windows && app_native

package app

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/apptest"
	"github.com/Obedience-Corp/festival-installer/internal/source"
)

const nsisReleaseBase = "https://github.com/Obedience-Corp/obey-releases/releases/download/app-test-25628d122be1ab6795737af192171d227ec3f2f2/"

func nsisGet(t *testing.T, name, dest string) {
	t.Helper()
	resp, err := http.Get(nsisReleaseBase + name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: %s", name, resp.Status)
	}
	f, err := os.Create(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := io.Copy(f, resp.Body); err != nil {
		t.Fatal(err)
	}
}

// TestAppNSISNativeRealInstallerRegisters runs the real 0.1.7 setup.exe on a
// fresh hosted VM through nsisPlatform with the production registry key.
func nativeInstallNSIS(t *testing.T) (appInstaller, string) {
	apptest.RequireHostedRunner(t)
	const asset = "Festival.App_0.1.7_x64-setup.exe"
	dir := t.TempDir()
	setup, sums := filepath.Join(dir, asset), filepath.Join(dir, "SHA256SUMS")
	nsisGet(t, asset, setup)
	nsisGet(t, "SHA256SUMS", sums)
	want := ""
	f, err := os.Open(sums)
	if err != nil {
		t.Fatal(err)
	}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if fields := strings.Fields(sc.Text()); len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == asset {
			want = fields[0]
		}
	}
	_ = f.Close()
	raw, err := os.ReadFile(setup)
	if err != nil {
		t.Fatal(err)
	}
	if sum := sha256.Sum256(raw); want == "" || hex.EncodeToString(sum[:]) != want {
		t.Fatalf("setup sha256 does not match SHA256SUMS (want %q)", want)
	}

	t.Setenv("FESTIVAL_HOME", filepath.Join(dir, "home"))
	store := &fakeAppStore{}
	ai, err := newAppInstaller(context.Background(), store.factory())
	if err != nil {
		t.Fatal(err)
	}
	ai.resolve = func(context.Context, string, string, source.VerifyOptions) (appRelease, error) {
		rel := appRelease{PackageID: AppPackageID, Version: "0.1.7", Channel: "stable", Source: "official-obey", Format: "nsis", BundleID: AppBundleID}
		rel.Artifact.URL, rel.Artifact.Sha256, rel.Artifact.Filename = nsisReleaseBase+asset, want, asset
		return rel, nil
	}
	ai.download = func(_ context.Context, _ string, rel appRelease, _ appDownloadOptions) (verifiedAppArtifact, error) {
		return verifiedAppArtifact{Path: setup, Dir: dir, Release: rel}, nil
	}
	if _, err := ai.install(context.Background(), AppInstallOptions{}); err != nil {
		t.Fatal(err)
	}
	reg, ok, err := readNSISRegistration(nsisUninstallKey)
	if err != nil || !ok || reg.DisplayVersion != "0.1.7" {
		t.Fatalf("registration %+v ok=%v err=%v", reg, ok, err)
	}
	local := os.Getenv("LOCALAPPDATA")
	for _, p := range []string{
		filepath.Join(local, "Festival App", "festival-app.exe"),
		filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", "Festival App.lnk"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing %s: %v", p, err)
		}
	}
	return ai, filepath.Join(local, "Festival App")
}

func TestAppNSISNativeRealInstallerRegisters(t *testing.T) {
	apptest.RequireHostedRunner(t)
	nativeInstallNSIS(t)
}

// TestAppNSISNativeRealUninstall runs the real uninstaller on what it just installed.
func TestAppNSISNativeRealUninstall(t *testing.T) {
	apptest.RequireHostedRunner(t)
	ai, dir := nativeInstallNSIS(t)
	removed, notes, err := ai.platform.remove(context.Background(), appReceipt{Version: "0.1.7", Format: "nsis", Location: dir}, os.Stderr)
	if err != nil || !removed {
		t.Fatalf("remove: %v %v %v", removed, notes, err)
	}
	if _, ok, rerr := readNSISRegistration(nsisUninstallKey); rerr != nil || ok {
		t.Fatalf("registry key still present: ok=%v err=%v", ok, rerr)
	}
	if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
		t.Fatalf("install dir still present: %v", statErr)
	}
	lnk := filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", "Festival App.lnk")
	if _, statErr := os.Stat(lnk); !os.IsNotExist(statErr) {
		t.Fatalf("Start menu shortcut still present: %v", statErr)
	}
}
