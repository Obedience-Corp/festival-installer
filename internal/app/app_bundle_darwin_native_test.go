//go:build darwin && app_native

package app

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/apptest"
	"github.com/Obedience-Corp/festival-installer/internal/source"
)

const nativeReleaseBase = "https://github.com/Obedience-Corp/obey-releases/releases/download/app-test-25628d122be1ab6795737af192171d227ec3f2f2/"

func nativeGet(t *testing.T, name, dest string) {
	t.Helper()
	resp, err := http.Get(nativeReleaseBase + name)
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

// TestAppBundleNativeRealBundleKeepsSignature places the real 0.1.7 bundle with
// real codesign into roots under t.TempDir() on a fresh hosted VM.
func TestAppBundleNativeRealBundleKeepsSignature(t *testing.T) {
	apptest.RequireHostedRunner(t)
	asset := "Festival.App_0.1.7_aarch64.app.tar.gz"
	if runtime.GOARCH == "amd64" {
		asset = "Festival.App_0.1.7_x64.app.tar.gz"
	}
	dir := t.TempDir()
	archive, sums := filepath.Join(dir, asset), filepath.Join(dir, "SHA256SUMS")
	nativeGet(t, asset, archive)
	nativeGet(t, "SHA256SUMS", sums)
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
	raw, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	if sum := sha256.Sum256(raw); want == "" || hex.EncodeToString(sum[:]) != want {
		t.Fatalf("archive sha256 does not match SHA256SUMS (want %q)", want)
	}

	sysApps, userApps := filepath.Join(dir, "Applications"), filepath.Join(dir, "home", "Applications")
	if err := os.MkdirAll(sysApps, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FESTIVAL_HOME", filepath.Join(dir, "home", "installer"))
	store := &fakeAppStore{}
	ai := appInstaller{
		goos: "darwin", euid: os.Geteuid,
		platform: newBundlePlatform(sysApps, userApps, os.Getuid(), darwinOwner, darwinWritable, codesignVerify),
		resolve: func(context.Context, string, string, source.VerifyOptions) (appRelease, error) {
			rel := appRelease{PackageID: AppPackageID, Version: "0.1.7", Channel: "stable", Source: "official-obey", Format: "app-bundle", BundleID: AppBundleID}
			rel.Artifact.URL, rel.Artifact.Sha256, rel.Artifact.Filename = nativeReleaseBase+asset, want, asset
			return rel, nil
		},
		download: func(_ context.Context, _ string, rel appRelease, _ appDownloadOptions) (verifiedAppArtifact, error) {
			return verifiedAppArtifact{Path: archive, Release: rel}, nil
		},
		begin: beginAppTxn, refuseRunning: refuseRunningApp, receipts: store.factory(),
	}
	res, err := ai.install(context.Background(), AppInstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(sysApps, appBundleName)
	if res.Location != dest {
		t.Fatalf("location %s, want %s", res.Location, dest)
	}
	if out, err := exec.Command("/usr/bin/xattr", "-r", "-l", dest).CombinedOutput(); err != nil || strings.Contains(string(out), "com.apple.quarantine") {
		t.Fatalf("xattr: %v\n%s", err, out)
	}
	if out, err := exec.Command("/usr/bin/codesign", "--verify", "--deep", "--strict", dest).CombinedOutput(); err != nil {
		t.Fatalf("codesign after install: %v\n%s", err, out)
	}
	// A second run finds the receipt's copy, which is already current.
	again, err := ai.install(context.Background(), AppInstallOptions{})
	if err != nil || again.Changed {
		t.Fatalf("second run: %+v %v", again, err)
	}
	entries, _ := os.ReadDir(sysApps)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".festival-") {
			t.Fatalf("sibling left: %s", e.Name())
		}
	}
}
