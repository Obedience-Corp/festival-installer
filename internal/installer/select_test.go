package installer_test

import (
	"errors"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/installer"
	"github.com/Obedience-Corp/festival-installer/internal/metadata"
)

func festivalManifest() metadata.PackageManifest {
	return metadata.PackageManifest{
		ID: "obedience-corp/festival",
		Releases: []metadata.Release{
			{Version: "0.2.9", Channel: "stable", Artifacts: []metadata.Artifact{
				{Kind: "suite-archive", OS: "darwin", Arch: "all"},
			}},
			{Version: "0.2.10", Channel: "stable", Artifacts: []metadata.Artifact{
				{Kind: "suite-archive", OS: "darwin", Arch: "all"},
				{Kind: "suite-archive", OS: "linux", Arch: "amd64"},
				{Kind: "suite-archive", OS: "linux", Arch: "arm64"},
			}},
			{Version: "0.3.0-rc.1", Channel: "rc", Artifacts: []metadata.Artifact{
				{Kind: "suite-archive", OS: "linux", Arch: "amd64"},
			}},
			{Version: "0.3.0-rc.2", Channel: "rc", Artifacts: []metadata.Artifact{
				{Kind: "suite-archive", OS: "linux", Arch: "amd64"},
			}},
		},
	}
}

func TestSelectRelease_ChannelNotFound(t *testing.T) {
	_, err := installer.SelectRelease(festivalManifest(), "dev")
	if !errors.Is(err, installer.ErrNoReleaseInChannel) {
		t.Fatalf("expected ErrNoReleaseInChannel, got %v", err)
	}
}

func TestSelectRelease_LatestWithinChannel(t *testing.T) {
	rel, err := installer.SelectRelease(festivalManifest(), "stable")
	if err != nil {
		t.Fatalf("SelectRelease: %v", err)
	}
	if rel.Version != "0.2.10" {
		t.Fatalf("expected 0.2.10, got %s", rel.Version)
	}
}

func TestSelectRelease_PrereleaseOrdering(t *testing.T) {
	rel, err := installer.SelectRelease(festivalManifest(), "rc")
	if err != nil {
		t.Fatalf("SelectRelease: %v", err)
	}
	if rel.Version != "0.3.0-rc.2" {
		t.Fatalf("expected 0.3.0-rc.2 (rc.2 > rc.1), got %s", rel.Version)
	}
}

func TestSelectRelease_OnlyMatchingChannelConsidered(t *testing.T) {
	rel, err := installer.SelectRelease(festivalManifest(), "rc")
	if err != nil {
		t.Fatalf("SelectRelease: %v", err)
	}
	if rel.Channel != "rc" {
		t.Fatalf("expected an rc release, got channel %s", rel.Channel)
	}
}

func TestSelectArtifact_ExactMatch(t *testing.T) {
	rel, err := installer.SelectRelease(festivalManifest(), "stable")
	if err != nil {
		t.Fatalf("SelectRelease: %v", err)
	}
	art, err := installer.SelectArtifact(rel, "linux", "amd64")
	if err != nil {
		t.Fatalf("SelectArtifact: %v", err)
	}
	if art.OS != "linux" || art.Arch != "amd64" {
		t.Fatalf("expected linux/amd64, got %s/%s", art.OS, art.Arch)
	}
}

func TestSelectArtifact_DarwinAllFallback(t *testing.T) {
	rel, err := installer.SelectRelease(festivalManifest(), "stable")
	if err != nil {
		t.Fatalf("SelectRelease: %v", err)
	}
	art, err := installer.SelectArtifact(rel, "darwin", "arm64")
	if err != nil {
		t.Fatalf("SelectArtifact: %v", err)
	}
	if art.OS != "darwin" || art.Arch != "all" {
		t.Fatalf("expected darwin/all fallback, got %s/%s", art.OS, art.Arch)
	}
}

func TestSelectArtifact_NotFound(t *testing.T) {
	rel, err := installer.SelectRelease(festivalManifest(), "stable")
	if err != nil {
		t.Fatalf("SelectRelease: %v", err)
	}
	if _, err := installer.SelectArtifact(rel, "windows", "amd64"); !errors.Is(err, installer.ErrNoArtifactForPlatform) {
		t.Fatalf("expected ErrNoArtifactForPlatform, got %v", err)
	}
}

// A git describe stamp is the tag plus commits, not a pre-release below it.
// camp, fest, and festival all stamp `git describe --tags`, so ordering
// "0.10.1-4-gd1fb37c7" below "0.10.1" is what makes an up-to-date machine look
// like it is behind the floor and reinstall on every launch.
func TestVersionLess_GitDescribeStampRanksAboveItsTag(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"0.10.1-4-gd1fb37c7", "0.10.1", false},
		{"0.10.1", "0.10.1-4-gd1fb37c7", true},
		{"0.10.1-4-gd1fb37c7", "0.10.2", true},
		{"0.10.2", "0.10.1-4-gd1fb37c7", false},
		{"0.10.1-4-gd1fb37c7", "0.10.1-12-gaaaaaaa", true},
		{"0.10.1-12-gaaaaaaa", "0.10.1-4-gd1fb37c7", false},
		{"0.10.1-4-gd1fb37c7", "0.10.1-4-gd1fb37c7", false},
		{"0.10.1-4-gd1fb37c7-dirty", "0.10.1", false},
		// A dirty tree is not a version. At a tag exactly `git describe
		// --dirty` prints "0.10.1-dirty" with no commit count, so the
		// identifier has to be dropped rather than read as a pre-release:
		// otherwise a locally built tool at the tag ranks below its own tag and
		// the app reads it as under the floor. festival-app strips the same
		// identifier (src-tauri/src/setup/versions.rs), so both sides agree.
		{"0.10.1-dirty", "0.10.1", false},
		{"0.10.1", "0.10.1-dirty", false},
		{"0.10.1-dirty", "0.10.2", true},
		{"0.10.2", "0.10.1-dirty", false},
		{"0.10.1-dirty", "0.10.1-4-gd1fb37c7", true},
		{"0.10.1-4-gd1fb37c7-dirty", "0.10.1-dirty", false},
		{"0.10.1-4-gd1fb37c7-dirty", "0.10.1-4-gd1fb37c7", false},
		// A real pre-release still ranks below its release with a dirty tree.
		{"0.10.1-rc.1-dirty", "0.10.1", true},
		{"0.10.1-rc.1", "0.10.1-4-gd1fb37c7", true},
		{"0.10.1-4-gd1fb37c7", "0.10.1-rc.1", false},
		// A real pre-release still sorts below its release.
		{"0.3.0-rc.1", "0.3.0", true},
		{"0.3.0", "0.3.0-rc.1", false},
		{"0.3.0-rc.1", "0.3.0-rc.2", true},
	}
	for _, tc := range cases {
		if got := installer.VersionLess(tc.a, tc.b); got != tc.want {
			t.Errorf("VersionLess(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
