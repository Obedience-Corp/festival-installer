package cli

import (
	"testing"

	"github.com/Obedience-Corp/obey-installer/internal/metadata"
)

func TestSelectTarget_MatchesHostRuntime(t *testing.T) {
	targets := []metadata.RuntimeTarget{
		{Package: "obedience-corp/festival", Runtime: "camp-cli", VersionConstraint: ">=0.2.0"},
		{Package: "obedience-corp/festival", Runtime: "fest-cli", VersionConstraint: ">=0.4.0"},
	}

	if got := selectTarget(targets, "fest"); got.Runtime != "fest-cli" {
		t.Fatalf("fest host selected %q, want fest-cli", got.Runtime)
	}
	if got := selectTarget(targets, "camp"); got.Runtime != "camp-cli" {
		t.Fatalf("camp host selected %q, want camp-cli", got.Runtime)
	}
}

func TestSelectTarget_FallbacksWhenNoMatch(t *testing.T) {
	if got := selectTarget(nil, "fest"); got.Runtime != "" {
		t.Fatalf("no targets should yield zero value, got %q", got.Runtime)
	}
	only := []metadata.RuntimeTarget{{Runtime: "camp-cli"}}
	if got := selectTarget(only, "fest"); got.Runtime != "camp-cli" {
		t.Fatalf("non-matching declared target should fall back to first, got %q", got.Runtime)
	}
}

func TestEntryExecutableName_FallsBackToSource(t *testing.T) {
	if got := entryExecutableName(metadata.InstallEntry{Kind: "binary", Source: "fest-demo"}); got != "fest-demo" {
		t.Fatalf("omitted executable_name should fall back to source, got %q", got)
	}
	if got := entryExecutableName(metadata.InstallEntry{Kind: "binary", Source: "fest-demo", ExecutableName: "renamed"}); got != "renamed" {
		t.Fatalf("explicit executable_name should win, got %q", got)
	}
}

func TestSelectPluginArtifact_ExactThenAllFallback(t *testing.T) {
	rel := metadata.Release{Artifacts: []metadata.Artifact{
		{Kind: "tar.gz", OS: "darwin", Arch: "all", URL: "u-darwin-all"},
		{Kind: "binary", OS: "linux", Arch: "amd64", URL: "u-linux-amd64"},
	}}

	exact, err := selectPluginArtifact(rel, "linux", "amd64")
	if err != nil || exact.URL != "u-linux-amd64" {
		t.Fatalf("exact match: %+v err=%v", exact, err)
	}

	fallback, err := selectPluginArtifact(rel, "darwin", "arm64")
	if err != nil || fallback.URL != "u-darwin-all" {
		t.Fatalf("expected arch=all fallback for a universal plugin artifact, got %+v err=%v", fallback, err)
	}

	if _, err := selectPluginArtifact(rel, "windows", "amd64"); err == nil {
		t.Fatal("expected error for an unsupported platform")
	}
}
