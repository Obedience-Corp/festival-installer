//go:build container_fs && !windows

package app

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"strings"
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/metadata"
)

func fullAppRelease(version, channel string) metadata.Release {
	return appFixtureRelease(version, channel,
		appFixtureArtifact(metadata.ArtifactKindAppBundle, "darwin", "arm64"),
		appFixtureArtifact(metadata.ArtifactKindAppBundle, "darwin", "amd64"),
		appFixtureArtifact(metadata.ArtifactKindNSIS, "windows", "amd64"),
		appFixtureArtifact(metadata.ArtifactKindDeb, "linux", "amd64"),
		appFixtureArtifact(metadata.ArtifactKindAppImage, "linux", "amd64"),
	)
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s, got nil", code)
	}
	if got := errpkg.Code(err); got != code {
		t.Fatalf("code = %s, want %s (%v)", got, code, err)
	}
}

func TestAppResolveSelectsEachPlatform(t *testing.T) {
	f := newAppFixture(t)
	f.publish(appFixtureManifest(fullAppRelease("0.1.8", "stable")), appFixtureOptions{})
	f.register("official-obey")
	cases := []struct{ goos, goarch, format string }{
		{"darwin", "arm64", metadata.ArtifactKindAppBundle},
		{"darwin", "amd64", metadata.ArtifactKindAppBundle},
		{"windows", "amd64", metadata.ArtifactKindNSIS},
		{"linux", "amd64", metadata.ArtifactKindDeb},
		{"linux", "amd64", metadata.ArtifactKindAppImage},
	}
	for _, tc := range cases {
		t.Run(tc.goos+"-"+tc.goarch+"-"+tc.format, func(t *testing.T) {
			before := f.SeedCalls
			got, err := resolveAppReleaseFor(context.Background(), appResolveOptions{Format: tc.format, GOOS: tc.goos, GOARCH: tc.goarch, Verify: f.Verify})
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			want := appFixtureArtifact(tc.format, tc.goos, tc.goarch)
			if got.Version != "0.1.8" || got.Format != tc.format || got.Source != "official-obey" || got.Channel != "stable" ||
				got.Artifact.URL != want.URL || got.Artifact.Sha256 != want.Sha256 || got.Artifact.UpdaterSignature == "" ||
				got.BundleID != AppBundleID || got.PackageID != AppPackageID {
				t.Fatalf("unexpected release: %+v", got)
			}
			if f.SeedCalls != before+1 {
				t.Fatalf("seed stub ran %d times, want 1", f.SeedCalls-before)
			}
		})
	}
}

func appOpts(f *appFixture, format string) appResolveOptions {
	return appResolveOptions{Format: format, GOOS: "linux", GOARCH: "amd64", Verify: f.Verify}
}

func TestAppResolvePicksNewestStable(t *testing.T) {
	f := newAppFixture(t)
	f.publish(appFixtureManifest(fullAppRelease("0.1.8", "stable"), fullAppRelease("0.1.9", "stable"), fullAppRelease("0.2.0", "rc")), appFixtureOptions{})
	f.register("official-obey")
	got, err := resolveAppReleaseFor(context.Background(), appOpts(f, metadata.ArtifactKindDeb))
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "0.1.9" {
		t.Fatalf("version = %s, want 0.1.9", got.Version)
	}
}

func TestAppResolveRefreshesBeforeLoad(t *testing.T) {
	f := newAppFixture(t)
	f.publish(appFixtureManifest(fullAppRelease("0.1.8", "stable")), appFixtureOptions{})
	f.register("official-obey")
	first, err := resolveAppReleaseFor(context.Background(), appOpts(f, metadata.ArtifactKindDeb))
	if err != nil || first.Version != "0.1.8" {
		t.Fatalf("first: %+v %v", first, err)
	}
	f.publish(appFixtureManifest(fullAppRelease("0.1.8", "stable"), fullAppRelease("0.1.9", "stable")), appFixtureOptions{})
	second, err := resolveAppReleaseFor(context.Background(), appOpts(f, metadata.ArtifactKindDeb))
	if err != nil || second.Version != "0.1.9" {
		t.Fatalf("second: %+v %v", second, err)
	}
}

func TestAppResolveNotAvailable(t *testing.T) {
	cases := map[string]struct {
		manifest metadata.PackageManifest
		opts     appFixtureOptions
	}{
		"not listed":      {appFixtureManifest(), appFixtureOptions{OmitApp: true}},
		"only rc release": {appFixtureManifest(fullAppRelease("0.2.0", "rc")), appFixtureOptions{}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newAppFixture(t)
			f.publish(tc.manifest, tc.opts)
			f.register("official-obey")
			_, err := resolveAppReleaseFor(context.Background(), appOpts(f, metadata.ArtifactKindDeb))
			wantCode(t, err, "E_APP_NOT_AVAILABLE")
			if !strings.Contains(err.Error(), "https://fest.build/app") {
				t.Fatalf("message lacks the download URL: %v", err)
			}
		})
	}
}

func TestAppResolveRejectsNonDesktopRelease(t *testing.T) {
	entries := map[string]metadata.InstallEntry{
		"binary entry":    {Kind: "binary", Source: "festival-app"},
		"other bundle id": {Kind: metadata.InstallKindDesktopApp, Source: "festival-app", BundleID: "com.example.other", ProductName: "Other"},
	}
	for name, entry := range entries {
		t.Run(name, func(t *testing.T) {
			f := newAppFixture(t)
			rel := fullAppRelease("0.1.8", "stable")
			rel.Install.Entries = []metadata.InstallEntry{entry}
			f.publish(appFixtureManifest(rel), appFixtureOptions{})
			f.register("official-obey")
			_, err := resolveAppReleaseFor(context.Background(), appOpts(f, metadata.ArtifactKindDeb))
			wantCode(t, err, "E_APP_NOT_AVAILABLE")
		})
	}
}

func TestAppResolvePlatformAndFormat(t *testing.T) {
	debOnly := appFixtureRelease("0.1.8", "stable", appFixtureArtifact(metadata.ArtifactKindDeb, "linux", "amd64"))
	cases := []struct {
		name               string
		rel                metadata.Release
		goos, goarch, kind string
		want               string
	}{
		{"linux arm64", fullAppRelease("0.1.8", "stable"), "linux", "arm64", metadata.ArtifactKindDeb, "E_APP_PLATFORM"},
		{"freebsd", fullAppRelease("0.1.8", "stable"), "freebsd", "amd64", metadata.ArtifactKindAppImage, "E_APP_PLATFORM"},
		{"nsis on macOS", fullAppRelease("0.1.8", "stable"), "darwin", "arm64", metadata.ArtifactKindNSIS, "E_APP_FORMAT_UNSUPPORTED"},
		{"appimage absent", debOnly, "linux", "amd64", metadata.ArtifactKindAppImage, "E_APP_FORMAT_UNSUPPORTED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAppFixture(t)
			f.publish(appFixtureManifest(tc.rel), appFixtureOptions{})
			f.register("official-obey")
			_, err := resolveAppReleaseFor(context.Background(), appResolveOptions{Format: tc.kind, GOOS: tc.goos, GOARCH: tc.goarch, Verify: f.Verify})
			wantCode(t, err, tc.want)
		})
	}
}

func TestAppResolveSignatureFailures(t *testing.T) {
	_, other, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	noSig := fullAppRelease("0.1.8", "stable")
	for i := range noSig.Artifacts {
		noSig.Artifacts[i].UpdaterSignature = ""
	}
	cases := []struct {
		name        string
		manifest    metadata.PackageManifest
		opts        appFixtureOptions
		allowUnsign bool
	}{
		{"unsigned catalog", appFixtureManifest(fullAppRelease("0.1.8", "stable")), appFixtureOptions{UnsignedCatalog: true}, false},
		{"unsigned manifest", appFixtureManifest(fullAppRelease("0.1.8", "stable")), appFixtureOptions{UnsignedManifest: true}, false},
		{"manifest signed by another key", appFixtureManifest(fullAppRelease("0.1.8", "stable")), appFixtureOptions{ManifestSignedBy: other}, false},
		{"allow-unverified does not help", appFixtureManifest(fullAppRelease("0.1.8", "stable")), appFixtureOptions{UnsignedManifest: true}, true},
		{"artifact without updater signature", appFixtureManifest(noSig), appFixtureOptions{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAppFixture(t)
			f.publish(tc.manifest, tc.opts)
			f.register("third-party")
			o := appOpts(f, metadata.ArtifactKindDeb)
			o.Source = "third-party"
			o.Verify.AllowUnverified = tc.allowUnsign
			_, err := resolveAppReleaseFor(context.Background(), o)
			wantCode(t, err, "E_APP_SIGNATURE")
		})
	}
}

func TestAppResolvePassesRefreshErrorsThrough(t *testing.T) {
	f := newAppFixture(t)
	f.publish(appFixtureManifest(fullAppRelease("0.1.8", "stable")), appFixtureOptions{})
	f.register("official-obey")
	if err := os.RemoveAll(f.Repo); err != nil {
		t.Fatal(err)
	}
	_, err := resolveAppReleaseFor(context.Background(), appOpts(f, metadata.ArtifactKindDeb))
	wantCode(t, err, "E_MARKETPLACE_REFRESH")
}

func TestAppResolveCancelled(t *testing.T) {
	f := newAppFixture(t)
	f.publish(appFixtureManifest(fullAppRelease("0.1.8", "stable")), appFixtureOptions{})
	f.register("official-obey")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := resolveAppReleaseFor(ctx, appOpts(f, metadata.ArtifactKindDeb))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if f.SeedCalls != 0 {
		t.Fatalf("seed stub ran %d times after cancellation", f.SeedCalls)
	}
}
