package installer_test

import (
	"errors"
	"strings"
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/installer"
	"github.com/Obedience-Corp/festival-installer/internal/metadata"
)

func appRelease() metadata.Release {
	return metadata.Release{Version: "0.1.8", Channel: "stable", Artifacts: []metadata.Artifact{
		{Kind: "app-bundle", OS: "darwin", Arch: "arm64", Filename: "mac-arm64"},
		{Kind: "app-bundle", OS: "darwin", Arch: "amd64", Filename: "mac-amd64"},
		{Kind: "nsis", OS: "windows", Arch: "amd64", Filename: "win"},
		{Kind: "deb", OS: "linux", Arch: "amd64", Filename: "linux-deb"},
		{Kind: "appimage", OS: "linux", Arch: "amd64", Filename: "linux-appimage"},
	}}
}

func TestSelectAppArtifact(t *testing.T) {
	suiteOnly := metadata.Release{Version: "1.0.0", Artifacts: []metadata.Artifact{{Kind: "tar.gz", OS: "darwin", Arch: "arm64"}}}
	allArch := metadata.Release{Version: "1.0.0", Artifacts: []metadata.Artifact{{Kind: "app-bundle", OS: "darwin", Arch: "all"}}}
	cases := []struct {
		name               string
		rel                metadata.Release
		goos, goarch, kind string
		wantFile           string
		wantCode           string
		wantIs             error
		wantMsg            string
	}{
		{name: "mac arm64 bundle", rel: appRelease(), goos: "darwin", goarch: "arm64", kind: "app-bundle", wantFile: "mac-arm64"},
		{name: "mac intel bundle", rel: appRelease(), goos: "darwin", goarch: "amd64", kind: "app-bundle", wantFile: "mac-amd64"},
		{name: "windows", rel: appRelease(), goos: "windows", goarch: "amd64", kind: "nsis", wantFile: "win"},
		{name: "linux deb", rel: appRelease(), goos: "linux", goarch: "amd64", kind: "deb", wantFile: "linux-deb"},
		{name: "linux appimage after deb", rel: appRelease(), goos: "linux", goarch: "amd64", kind: "appimage", wantFile: "linux-appimage"},
		{name: "linux arm64", rel: appRelease(), goos: "linux", goarch: "arm64", kind: "deb", wantCode: "E_APP_PLATFORM", wantIs: installer.ErrNoAppArtifactForPlatform},
		{name: "nsis on linux", rel: appRelease(), goos: "linux", goarch: "amd64", kind: "nsis", wantCode: "E_APP_FORMAT_UNSUPPORTED", wantIs: installer.ErrAppFormatUnavailable, wantMsg: "deb, appimage"},
		{name: "unknown format", rel: appRelease(), goos: "linux", goarch: "amd64", kind: "rpm", wantCode: "E_APP_FORMAT_UNSUPPORTED"},
		{name: "suite kinds ignored", rel: suiteOnly, goos: "darwin", goarch: "arm64", kind: "app-bundle", wantCode: "E_APP_PLATFORM", wantIs: installer.ErrNoAppArtifactForPlatform},
		{name: "arch all not used", rel: allArch, goos: "darwin", goarch: "arm64", kind: "app-bundle", wantCode: "E_APP_PLATFORM", wantIs: installer.ErrNoAppArtifactForPlatform},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := installer.SelectAppArtifact(tc.rel, tc.goos, tc.goarch, tc.kind)
			if tc.wantCode == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got.Filename != tc.wantFile {
					t.Fatalf("picked %q, want %q", got.Filename, tc.wantFile)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected %s, got artifact %+v", tc.wantCode, got)
			}
			if c := errpkg.Code(err); c != tc.wantCode {
				t.Fatalf("code = %s, want %s (%v)", c, tc.wantCode, err)
			}
			if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
				t.Fatalf("error does not wrap %v: %v", tc.wantIs, err)
			}
			if tc.wantMsg != "" && !strings.Contains(err.Error(), tc.wantMsg) {
				t.Fatalf("error %q lacks %q", err, tc.wantMsg)
			}
		})
	}
}
