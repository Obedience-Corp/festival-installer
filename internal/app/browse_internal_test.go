package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/metadata"
	"github.com/Obedience-Corp/festival-installer/internal/source"
	"github.com/Obedience-Corp/festival-installer/internal/state"
)

func browseFixture() []source.BrowsePackage {
	return []source.BrowsePackage{
		{Source: "official-obey", Package: source.PackageRef{
			ID: "obedience-corp/fest", DisplayName: "Fest", Class: "tool",
			HostRuntimes: []string{"fest-cli", "fest-extension"},
		}},
		{Source: "official-obey", Package: source.PackageRef{
			ID: "acme/fest-demo", DisplayName: "Fest Demo", Class: "plugin",
			HostRuntimes: []string{"fest-cli"},
			Targets:      []metadata.RuntimeTarget{{Package: "obedience-corp/festival", Runtime: "fest-cli"}},
		}},
		{Source: "official-obey", Package: source.PackageRef{
			ID: "acme/camp-graph", DisplayName: "Camp Graph", Class: "plugin",
			HostRuntimes: []string{"camp-cli"},
		}},
	}
}

func collectIDs(res BrowseResult) []string {
	seen := map[string]bool{}
	var ids []string
	for _, g := range res.Groups {
		for _, p := range g.Packages {
			if !seen[p.ID] {
				seen[p.ID] = true
				ids = append(ids, p.ID)
			}
		}
	}
	return ids
}

func TestBuildBrowseResult_ProductAndKind(t *testing.T) {
	res := BuildBrowseResult(browseFixture(), "fest", "plugin")
	ids := collectIDs(res)
	if len(ids) != 1 || ids[0] != "acme/fest-demo" {
		t.Fatalf("expected only acme/fest-demo, got %v", ids)
	}
}

func TestBuildBrowseResult_KindOnly(t *testing.T) {
	res := BuildBrowseResult(browseFixture(), "", "plugin")
	ids := collectIDs(res)
	if len(ids) != 2 {
		t.Fatalf("expected both plugins, got %v", ids)
	}
}

func TestBuildBrowseResult_NoFilterGroupsSorted(t *testing.T) {
	res := BuildBrowseResult(browseFixture(), "", "")
	if len(res.Groups) == 0 {
		t.Fatal("expected groups")
	}
	for i := 1; i < len(res.Groups); i++ {
		if res.Groups[i-1].HostRuntime > res.Groups[i].HostRuntime {
			t.Fatalf("groups not sorted: %s before %s", res.Groups[i-1].HostRuntime, res.Groups[i].HostRuntime)
		}
	}
	var festExt *BrowseGroup
	for i := range res.Groups {
		if res.Groups[i].HostRuntime == "fest-extension" {
			festExt = &res.Groups[i]
		}
	}
	if festExt == nil || len(festExt.Packages) != 1 || festExt.Packages[0].ID != "obedience-corp/fest" {
		t.Fatalf("fest tool should appear under fest-extension group: %+v", festExt)
	}
}

func TestBuildBrowseResult_EmptyIsNonNil(t *testing.T) {
	res := BuildBrowseResult(nil, "fest", "plugin")
	if res.Groups == nil {
		t.Fatal("Groups should be non-nil empty slice")
	}
	if len(res.Groups) != 0 {
		t.Fatalf("expected no groups, got %d", len(res.Groups))
	}
}

const browseSeedManifest = `{
  "id": "obedience-corp/official",
  "name": "Official",
  "schema_version": "1",
  "packages": [
    {"id":"obedience-corp/fest","display_name":"Fest","class":"tool","host_runtimes":["fest-cli"],"channels":["stable"],"manifest_path":"packages/fest/obey-package.json"}
  ]
}
`

func browseSeedGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func browseSeedFixtureRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	browseSeedGit(t, dir, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "obey-marketplace.json"), []byte(browseSeedManifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "packages", "fest"), 0o755); err != nil {
		t.Fatalf("mkdir packages: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "packages", "fest", "obey-package.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write package: %v", err)
	}
	browseSeedGit(t, dir, "add", ".")
	browseSeedGit(t, dir, "commit", "-m", "init")
	return dir
}

func TestBrowseBootstrapsOfficialSourceOnFreshHome(t *testing.T) {
	t.Setenv("FESTIVAL_HOME", t.TempDir())
	t.Setenv("OBEY_INSTALLER_HOME", "")

	called := false
	previous := ensureOfficialSeed
	ensureOfficialSeed = func(context.Context) error {
		called = true
		return nil
	}
	t.Cleanup(func() { ensureOfficialSeed = previous })

	if _, err := Browse(context.Background(), BrowseOptions{}); err != nil {
		t.Fatalf("Browse: %v", err)
	}
	if !called {
		t.Fatal("Browse did not attempt to seed the official source before reading the catalog")
	}
}

func TestBrowseSurfacesSeedWarningButStillReturnsCatalog(t *testing.T) {
	t.Setenv("FESTIVAL_HOME", t.TempDir())
	t.Setenv("OBEY_INSTALLER_HOME", "")
	ctx := context.Background()

	fixture := browseSeedFixtureRepo(t)
	if _, err := source.AddMarketplace(ctx, fixture, "acme"); err != nil {
		t.Fatalf("seed local source: %v", err)
	}

	want := errors.New("git clone -- https://github.com/Obedience-Corp/marketplace.git: fatal: could not read Username")
	previous := ensureOfficialSeed
	ensureOfficialSeed = func(context.Context) error { return want }
	t.Cleanup(func() { ensureOfficialSeed = previous })

	res, err := Browse(ctx, BrowseOptions{})

	var warning *MarketplaceSeedWarning
	if !errors.As(err, &warning) {
		t.Fatalf("expected MarketplaceSeedWarning, got %v", err)
	}
	if !errors.Is(err, want) {
		t.Fatalf("warning lost the underlying seed error: %v", err)
	}
	if got := warning.Friendly(); got != marketplaceSeedFriendlyMessage || strings.Contains(got, "git clone") {
		t.Fatalf("Friendly() must never leak the raw seed error, got %q", got)
	}
	if got := collectIDs(res); len(got) != 1 || got[0] != "obedience-corp/fest" {
		t.Fatalf("expected the pre-existing local catalog despite the seed failure, got %v", got)
	}
}

func TestBrowseAlreadySeededSkipsReclone(t *testing.T) {
	t.Setenv("FESTIVAL_HOME", t.TempDir())
	t.Setenv("OBEY_INSTALLER_HOME", "")
	ctx := context.Background()

	fixture := browseSeedFixtureRepo(t)
	if _, err := source.AddMarketplace(ctx, fixture, state.OfficialSeedKey); err != nil {
		t.Fatalf("pre-register official source: %v", err)
	}

	// ensureOfficialSeed is left as the real source.EnsureOfficialSeed: with a
	// source already registered under OfficialSeedKey, seedFromURL's
	// len(sources) > 0 guard must short-circuit before any clone attempt
	// against the real (currently private) official URL.
	res, err := Browse(ctx, BrowseOptions{})
	if err != nil {
		t.Fatalf("Browse: %v", err)
	}
	if got := collectIDs(res); len(got) != 1 || got[0] != "obedience-corp/fest" {
		t.Fatalf("expected catalog populated from the pre-registered source, got %v", got)
	}
}
