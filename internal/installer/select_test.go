package installer_test

import (
	"errors"
	"testing"

	"github.com/Obedience-Corp/obey-installer/internal/installer"
	"github.com/Obedience-Corp/obey-installer/internal/metadata"
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
