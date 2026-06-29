package resolver_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Obedience-Corp/obey-installer/internal/metadata"
	"github.com/Obedience-Corp/obey-installer/internal/resolver"
	"github.com/Obedience-Corp/obey-installer/internal/source"
)

type fakeIndex struct {
	sources   []resolver.Source
	pkgs      map[string][]source.PackageRef
	manifests map[string]metadata.PackageManifest
}

func (f *fakeIndex) EnabledSources(context.Context) ([]resolver.Source, error) { return f.sources, nil }

func (f *fakeIndex) Packages(_ context.Context, sourceID string) ([]source.PackageRef, error) {
	return f.pkgs[sourceID], nil
}

func (f *fakeIndex) Manifest(_ context.Context, sourceID, packageID string) (metadata.PackageManifest, error) {
	if m, ok := f.manifests[sourceID+"|"+packageID]; ok {
		return m, nil
	}
	return metadata.PackageManifest{}, &notFound{packageID}
}

type notFound struct{ id string }

func (e *notFound) Error() string { return "manifest not found: " + e.id }

type fakeScope struct {
	root bool
	lock bool
}

func (f fakeScope) CampaignRootKnown() bool { return f.root }
func (f fakeScope) CanLockUserScope() bool  { return f.lock }

func release(version, channel string, deps []metadata.Dependency, entryKind string) metadata.Release {
	return metadata.Release{
		Version:       version,
		Channel:       channel,
		Compatibility: metadata.Compatibility{OS: []string{"linux"}, Arch: []string{"amd64"}},
		Dependencies:  deps,
		Artifacts:     []metadata.Artifact{{Kind: "binary", OS: "linux", Arch: "amd64", URL: "u", Sha256: strings.Repeat("a", 64)}},
		Install:       metadata.Install{Entries: []metadata.InstallEntry{{Kind: entryKind, Source: "x", ExecutableName: "x"}}},
	}
}

func manifest(id, class string, rels []metadata.Release) metadata.PackageManifest {
	return metadata.PackageManifest{ID: id, Class: class, Releases: rels}
}

func newIndex(srcs []resolver.Source) *fakeIndex {
	return &fakeIndex{sources: srcs, pkgs: map[string][]source.PackageRef{}, manifests: map[string]metadata.PackageManifest{}}
}

func (f *fakeIndex) add(sourceID string, ref source.PackageRef, m metadata.PackageManifest) {
	f.pkgs[sourceID] = append(f.pkgs[sourceID], ref)
	f.manifests[sourceID+"|"+ref.ID] = m
}

func linuxReq(sel ...resolver.Selector) resolver.Request {
	return resolver.Request{Selectors: sel, OS: "linux", Arch: "amd64", DefaultChannel: "stable"}
}

func selectedIDs(p resolver.Plan) []string {
	var ids []string
	for _, r := range p.Selected {
		ids = append(ids, r.PackageID)
	}
	return ids
}

func TestResolve_ExactID(t *testing.T) {
	idx := newIndex([]resolver.Source{{ID: "official", Priority: 0}})
	idx.add("official", source.PackageRef{ID: "obedience-corp/festival", Class: "product"},
		manifest("obedience-corp/festival", "product", []metadata.Release{release("0.2.10", "stable", nil, "binary")}))
	r := resolver.New(idx, fakeScope{})

	plan, err := r.Resolve(context.Background(), linuxReq(resolver.Selector{Raw: "obedience-corp/festival"}))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got := selectedIDs(plan); len(got) != 1 || got[0] != "obedience-corp/festival" {
		t.Fatalf("unexpected plan: %v", got)
	}
	if plan.Selected[0].Reason != "requested" || plan.Selected[0].Version != "0.2.10" {
		t.Fatalf("unexpected resolved: %+v", plan.Selected[0])
	}
}

func TestResolve_ShortSelectorSingleMatch(t *testing.T) {
	idx := newIndex([]resolver.Source{{ID: "official", Priority: 0}})
	idx.add("official", source.PackageRef{ID: "obedience-corp/festival", Class: "product"},
		manifest("obedience-corp/festival", "product", []metadata.Release{release("0.2.10", "stable", nil, "binary")}))
	r := resolver.New(idx, fakeScope{})

	plan, err := r.Resolve(context.Background(), linuxReq(resolver.Selector{Raw: "festival"}))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got := selectedIDs(plan); len(got) != 1 || got[0] != "obedience-corp/festival" {
		t.Fatalf("unexpected plan: %v", got)
	}
}

func TestResolve_ShortSelectorAmbiguous(t *testing.T) {
	idx := newIndex([]resolver.Source{{ID: "official", Priority: 0}})
	idx.add("official", source.PackageRef{ID: "a/demo", Class: "plugin"},
		manifest("a/demo", "plugin", []metadata.Release{release("1.0.0", "stable", nil, "binary")}))
	idx.add("official", source.PackageRef{ID: "b/demo", Class: "plugin"},
		manifest("b/demo", "plugin", []metadata.Release{release("1.0.0", "stable", nil, "binary")}))
	r := resolver.New(idx, fakeScope{})

	_, err := r.Resolve(context.Background(), linuxReq(resolver.Selector{Raw: "demo"}))
	if err == nil || !strings.Contains(err.Error(), "E_SELECTOR_AMBIGUOUS") {
		t.Fatalf("expected E_SELECTOR_AMBIGUOUS, got %v", err)
	}
}

func TestResolve_NoMatchingRelease(t *testing.T) {
	idx := newIndex([]resolver.Source{{ID: "official", Priority: 0}})
	idx.add("official", source.PackageRef{ID: "obedience-corp/festival", Class: "product"},
		manifest("obedience-corp/festival", "product", []metadata.Release{release("0.2.10", "rc", nil, "binary")}))
	r := resolver.New(idx, fakeScope{})

	_, err := r.Resolve(context.Background(), linuxReq(resolver.Selector{Raw: "obedience-corp/festival", Channel: "stable"}))
	if err == nil || !strings.Contains(err.Error(), "E_NO_MATCHING_RELEASE") {
		t.Fatalf("expected E_NO_MATCHING_RELEASE, got %v", err)
	}
}

func TestResolve_PluginHostMissing(t *testing.T) {
	idx := newIndex([]resolver.Source{{ID: "official", Priority: 0}})
	pm := manifest("acme/fest-demo", "plugin", []metadata.Release{release("1.0.0", "stable", nil, "binary")})
	pm.Targets = []metadata.RuntimeTarget{{Package: "obedience-corp/festival", Runtime: "fest-cli", VersionConstraint: ">=0.4.0"}}
	idx.add("official", source.PackageRef{ID: "acme/fest-demo", Class: "plugin"}, pm)
	r := resolver.New(idx, fakeScope{})

	_, err := r.Resolve(context.Background(), linuxReq(resolver.Selector{Raw: "acme/fest-demo"}))
	if err == nil || !strings.Contains(err.Error(), "E_HOST_MISSING") {
		t.Fatalf("expected E_HOST_MISSING, got %v", err)
	}
}

func TestResolve_PluginHostInPlanOrdersHostFirst(t *testing.T) {
	idx := newIndex([]resolver.Source{{ID: "official", Priority: 0}})
	host := manifest("obedience-corp/festival", "product", []metadata.Release{release("0.4.5", "stable", nil, "binary")})
	host.HostRuntimes = []metadata.HostRuntime{{Runtime: "fest-cli", Features: []string{"marketplace_extension_source_v1"}}}
	idx.add("official", source.PackageRef{ID: "obedience-corp/festival", Class: "product"}, host)

	pm := manifest("acme/fest-demo", "plugin", []metadata.Release{release("1.0.0", "stable", nil, "binary")})
	pm.Targets = []metadata.RuntimeTarget{{Package: "obedience-corp/festival", Runtime: "fest-cli", VersionConstraint: ">=0.4.0", RequiredFeatures: []string{"marketplace_extension_source_v1"}}}
	idx.add("official", source.PackageRef{ID: "acme/fest-demo", Class: "plugin"}, pm)
	r := resolver.New(idx, fakeScope{})

	plan, err := r.Resolve(context.Background(), linuxReq(
		resolver.Selector{Raw: "acme/fest-demo"},
		resolver.Selector{Raw: "obedience-corp/festival"},
	))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	ids := selectedIDs(plan)
	hostIdx, pluginIdx := indexOf(ids, "obedience-corp/festival"), indexOf(ids, "acme/fest-demo")
	if hostIdx < 0 || pluginIdx < 0 || hostIdx > pluginIdx {
		t.Fatalf("expected host before plugin, got %v", ids)
	}
}

func TestResolve_DepConstraintsCompatibleAndNot(t *testing.T) {
	build := func(cConstraint string) *fakeIndex {
		idx := newIndex([]resolver.Source{{ID: "official", Priority: 0}})
		idx.add("official", source.PackageRef{ID: "x/b", Class: "tool"},
			manifest("x/b", "tool", []metadata.Release{release("2.0.0", "stable", nil, "binary")}))
		idx.add("official", source.PackageRef{ID: "x/a", Class: "tool"},
			manifest("x/a", "tool", []metadata.Release{release("1.0.0", "stable", []metadata.Dependency{{Package: "x/b", VersionConstraint: ">=1.0.0"}}, "binary")}))
		idx.add("official", source.PackageRef{ID: "x/c", Class: "tool"},
			manifest("x/c", "tool", []metadata.Release{release("1.0.0", "stable", []metadata.Dependency{{Package: "x/b", VersionConstraint: cConstraint}}, "binary")}))
		return idx
	}

	r := resolver.New(build(">=2.0.0"), fakeScope{})
	plan, err := r.Resolve(context.Background(), linuxReq(resolver.Selector{Raw: "x/a"}, resolver.Selector{Raw: "x/c"}))
	if err != nil {
		t.Fatalf("compatible Resolve: %v", err)
	}
	bCount := 0
	for _, id := range selectedIDs(plan) {
		if id == "x/b" {
			bCount++
		}
	}
	if bCount != 1 {
		t.Fatalf("expected exactly one x/b, got %d in %v", bCount, selectedIDs(plan))
	}

	r2 := resolver.New(build("<1.5.0"), fakeScope{})
	_, err = r2.Resolve(context.Background(), linuxReq(resolver.Selector{Raw: "x/a"}, resolver.Selector{Raw: "x/c"}))
	if err == nil || !strings.Contains(err.Error(), "E_DEP_CONSTRAINT_UNSAT") {
		t.Fatalf("expected E_DEP_CONSTRAINT_UNSAT, got %v", err)
	}
}

func TestResolve_CampaignScopeRequiresRoot(t *testing.T) {
	idx := newIndex([]resolver.Source{{ID: "official", Priority: 0}})
	idx.add("official", source.PackageRef{ID: "obey/skills", Class: "plugin"},
		manifest("obey/skills", "plugin", []metadata.Release{release("1.0.0", "stable", nil, "skill_bundle")}))

	r := resolver.New(idx, fakeScope{root: false})
	_, err := r.Resolve(context.Background(), linuxReq(resolver.Selector{Raw: "obey/skills"}))
	if err == nil || !strings.Contains(err.Error(), "E_CAMPAIGN_ROOT_REQUIRED") {
		t.Fatalf("expected E_CAMPAIGN_ROOT_REQUIRED, got %v", err)
	}

	r2 := resolver.New(idx, fakeScope{root: true})
	if _, err := r2.Resolve(context.Background(), linuxReq(resolver.Selector{Raw: "obey/skills"})); err != nil {
		t.Fatalf("with root known, expected success, got %v", err)
	}
}

func TestResolve_DuplicateIDHigherPriorityWins(t *testing.T) {
	idx := newIndex([]resolver.Source{{ID: "third-party", Priority: 10}, {ID: "official", Priority: 0}})
	idx.add("third-party", source.PackageRef{ID: "obedience-corp/festival", Class: "product"},
		manifest("obedience-corp/festival", "product", []metadata.Release{release("9.9.9", "stable", nil, "binary")}))
	idx.add("official", source.PackageRef{ID: "obedience-corp/festival", Class: "product"},
		manifest("obedience-corp/festival", "product", []metadata.Release{release("0.2.10", "stable", nil, "binary")}))
	r := resolver.New(idx, fakeScope{})

	plan, err := r.Resolve(context.Background(), linuxReq(resolver.Selector{Raw: "obedience-corp/festival"}))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if plan.Selected[0].SourceID != "official" || plan.Selected[0].Version != "0.2.10" {
		t.Fatalf("expected official 0.2.10 to win, got %+v", plan.Selected[0])
	}
}

func indexOf(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}
