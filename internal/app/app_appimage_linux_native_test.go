//go:build linux && app_native

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

const imageReleaseBase = "https://github.com/Obedience-Corp/obey-releases/releases/download/app-test-25628d122be1ab6795737af192171d227ec3f2f2/"

func imageGet(t *testing.T, name, dest string) {
	t.Helper()
	resp, err := http.Get(imageReleaseBase + name)
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

// TestAppImageNativeRealInstall places the real 0.1.7 AppImage into the
// runner's own home on a fresh hosted ubuntu runner.
func TestAppImageNativeRealInstall(t *testing.T) {
	apptest.RequireHostedRunner(t)
	const asset = "Festival.App_0.1.7_amd64.AppImage"
	dir := t.TempDir()
	image, sums := filepath.Join(dir, asset), filepath.Join(dir, "SHA256SUMS")
	imageGet(t, asset, image)
	imageGet(t, "SHA256SUMS", sums)
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
	raw, err := os.ReadFile(image)
	if err != nil {
		t.Fatal(err)
	}
	if sum := sha256.Sum256(raw); want == "" || hex.EncodeToString(sum[:]) != want {
		t.Fatalf("AppImage sha256 does not match SHA256SUMS (want %q)", want)
	}

	t.Setenv("FESTIVAL_HOME", filepath.Join(dir, "home"))
	store := &fakeAppStore{}
	ai, err := newAppInstaller(context.Background(), store.factory())
	if err != nil {
		t.Fatal(err)
	}
	ai.resolve = func(context.Context, string, string, source.VerifyOptions) (appRelease, error) {
		rel := appRelease{PackageID: AppPackageID, Version: "0.1.7", Channel: "stable", Source: "official-obey", Format: "appimage", BundleID: AppBundleID}
		rel.Artifact.URL, rel.Artifact.Sha256, rel.Artifact.Filename = imageReleaseBase+asset, want, asset
		return rel, nil
	}
	ai.download = func(_ context.Context, _ string, rel appRelease, _ appDownloadOptions) (verifiedAppArtifact, error) {
		return verifiedAppArtifact{Path: image, Dir: "", Release: rel}, nil
	}
	res, err := ai.install(context.Background(), AppInstallOptions{Format: "appimage"})
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	p := appImagePlacer{binDir: filepath.Join(home, ".local", "bin"), dataHome: xdgDataHome(os.Getenv, home)}
	if res.Location != p.target() {
		t.Fatalf("location %s, want %s", res.Location, p.target())
	}
	if fi, err := os.Stat(p.target()); err != nil || fi.Mode().Perm() != 0o755 {
		t.Fatalf("binary: %v %v", fi, err)
	}
	if _, err := os.Stat(p.desktopPath()); err != nil {
		t.Fatalf("desktop entry: %v", err)
	}
	icon, err := os.ReadFile(p.iconPath())
	if err != nil || len(icon) < 4 || string(icon[:4]) != "\x89PNG" {
		t.Fatalf("icon: %v", err)
	}
}
