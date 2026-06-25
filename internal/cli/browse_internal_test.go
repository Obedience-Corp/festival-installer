package cli

import (
	"testing"

	"github.com/Obedience-Corp/obey-installer/internal/metadata"
	"github.com/Obedience-Corp/obey-installer/internal/source"
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

func collectIDs(res browseResult) []string {
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
	res := buildBrowseResult(browseFixture(), "fest", "plugin")
	ids := collectIDs(res)
	if len(ids) != 1 || ids[0] != "acme/fest-demo" {
		t.Fatalf("expected only acme/fest-demo, got %v", ids)
	}
}

func TestBuildBrowseResult_KindOnly(t *testing.T) {
	res := buildBrowseResult(browseFixture(), "", "plugin")
	ids := collectIDs(res)
	if len(ids) != 2 {
		t.Fatalf("expected both plugins, got %v", ids)
	}
}

func TestBuildBrowseResult_NoFilterGroupsSorted(t *testing.T) {
	res := buildBrowseResult(browseFixture(), "", "")
	if len(res.Groups) == 0 {
		t.Fatal("expected groups")
	}
	for i := 1; i < len(res.Groups); i++ {
		if res.Groups[i-1].HostRuntime > res.Groups[i].HostRuntime {
			t.Fatalf("groups not sorted: %s before %s", res.Groups[i-1].HostRuntime, res.Groups[i].HostRuntime)
		}
	}
	var festExt *browseGroup
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
	res := buildBrowseResult(nil, "fest", "plugin")
	if res.Groups == nil {
		t.Fatal("Groups should be non-nil empty slice")
	}
	if len(res.Groups) != 0 {
		t.Fatalf("expected no groups, got %d", len(res.Groups))
	}
}
