package resolver

import (
	"context"
	"slices"
	"sort"
	"strings"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/installer"
	"github.com/Obedience-Corp/festival-installer/internal/metadata"
	"github.com/Obedience-Corp/festival-installer/internal/source"
)

type Selector struct {
	Raw     string
	Channel string
	Source  string
}

type Scope string

const (
	ScopeUser     Scope = "user"
	ScopeCampaign Scope = "campaign"
)

type Request struct {
	Selectors      []Selector
	OS             string
	Arch           string
	DefaultChannel string
	CampaignRoot   string
}

type Resolved struct {
	PackageID string
	Version   string
	Scope     Scope
	Reason    string
	Release   metadata.Release
	Artifact  metadata.Artifact
	SourceID  string
}

type Plan struct {
	Requested []string
	Selected  []Resolved
}

type Source struct {
	ID       string
	Priority int
}

// SourceIndex resolves packages backed by a static package manifest. Packages
// backed only by a PackageRef.ReleaseSource (no manifest_path) are NOT handled
// here: this resolver is not yet wired into any command, and the install CLI
// resolves release_source plugins directly (see installPlugin). When the CLI
// adopts this resolver, the SourceIndex adapter must synthesize a manifest for
// release_source refs (or such refs must be excluded by selection) before they
// reach Manifest, which would otherwise fail with a missing-manifest error.
type SourceIndex interface {
	EnabledSources(ctx context.Context) ([]Source, error)
	Packages(ctx context.Context, sourceID string) ([]source.PackageRef, error)
	Manifest(ctx context.Context, sourceID, packageID string) (metadata.PackageManifest, error)
}

type ScopeChecker interface {
	CampaignRootKnown() bool
	CanLockUserScope() bool
}

type Resolver struct {
	sources SourceIndex
	scope   ScopeChecker
}

func New(sources SourceIndex, scope ScopeChecker) *Resolver {
	return &Resolver{sources: sources, scope: scope}
}

type resolution struct {
	index     SourceIndex
	sources   []Source
	active    map[string]*Resolved
	manifests map[string]metadata.PackageManifest
	edges     map[string][]string
	req       Request
}

func (r *Resolver) Resolve(ctx context.Context, req Request) (Plan, error) {
	srcs, err := r.sources.EnabledSources(ctx)
	if err != nil {
		return Plan{}, err
	}
	sortSources(srcs)

	st := &resolution{
		index:     r.sources,
		sources:   srcs,
		active:    map[string]*Resolved{},
		manifests: map[string]metadata.PackageManifest{},
		edges:     map[string][]string{},
		req:       req,
	}

	plan := Plan{}
	for _, sel := range req.Selectors {
		plan.Requested = append(plan.Requested, sel.Raw)
		pkgID, srcID, err := st.normalize(ctx, sel)
		if err != nil {
			return Plan{}, err
		}
		if err := st.resolvePackage(ctx, pkgID, srcID, sel.Channel, "requested", ""); err != nil {
			return Plan{}, err
		}
	}

	if err := st.validateHostTargets(); err != nil {
		return Plan{}, err
	}
	if err := st.validateScopes(r.scope); err != nil {
		return Plan{}, err
	}

	ordered, err := st.topoOrder()
	if err != nil {
		return Plan{}, err
	}
	for _, id := range ordered {
		plan.Selected = append(plan.Selected, *st.active[id])
	}
	return plan, nil
}

func sortSources(srcs []Source) {
	sort.SliceStable(srcs, func(i, j int) bool {
		if srcs[i].Priority != srcs[j].Priority {
			return srcs[i].Priority < srcs[j].Priority
		}
		return srcs[i].ID < srcs[j].ID
	})
}

func (st *resolution) normalize(ctx context.Context, sel Selector) (string, string, error) {
	if strings.Contains(sel.Raw, "/") {
		for _, s := range st.sources {
			if sel.Source != "" && s.ID != sel.Source {
				continue
			}
			refs, err := st.index.Packages(ctx, s.ID)
			if err != nil {
				return "", "", err
			}
			for _, ref := range refs {
				if ref.ID == sel.Raw {
					return sel.Raw, s.ID, nil
				}
			}
		}
		return "", "", errpkg.New("E_SELECTOR_NOT_FOUND", "package "+sel.Raw+" not found in any enabled source")
	}

	type hit struct {
		pkgID string
		srcID string
	}
	seen := map[string]string{}
	var hits []hit
	for _, s := range st.sources {
		if sel.Source != "" && s.ID != sel.Source {
			continue
		}
		refs, err := st.index.Packages(ctx, s.ID)
		if err != nil {
			return "", "", err
		}
		for _, ref := range refs {
			if nameSegment(ref.ID) != sel.Raw {
				continue
			}
			if _, ok := seen[ref.ID]; ok {
				continue
			}
			seen[ref.ID] = s.ID
			hits = append(hits, hit{pkgID: ref.ID, srcID: s.ID})
		}
	}
	switch len(hits) {
	case 0:
		return "", "", errpkg.New("E_SELECTOR_NOT_FOUND", "no package matching "+sel.Raw)
	case 1:
		return hits[0].pkgID, hits[0].srcID, nil
	default:
		return "", "", errpkg.New("E_SELECTOR_AMBIGUOUS", "selector "+sel.Raw+" matches multiple packages")
	}
}

func nameSegment(id string) string {
	if i := strings.LastIndex(id, "/"); i >= 0 {
		return id[i+1:]
	}
	return id
}

func (st *resolution) sourceForPackage(ctx context.Context, packageID string) (string, error) {
	for _, s := range st.sources {
		refs, err := st.index.Packages(ctx, s.ID)
		if err != nil {
			return "", err
		}
		for _, ref := range refs {
			if ref.ID == packageID {
				return s.ID, nil
			}
		}
	}
	return "", errpkg.New("E_SELECTOR_NOT_FOUND", "package "+packageID+" not found in any enabled source")
}

func (st *resolution) resolvePackage(ctx context.Context, packageID, srcID, channel, reason, constraint string) error {
	if existing, ok := st.active[packageID]; ok {
		if constraint != "" {
			ok, cerr := installer.SatisfiesConstraint(existing.Version, constraint)
			if cerr != nil {
				return errpkg.Wrap("E_DEP_CONSTRAINT", cerr, packageID+" constraint "+constraint)
			}
			if !ok {
				return errpkg.New("E_DEP_CONSTRAINT_UNSAT", packageID+" "+existing.Version+" does not satisfy "+constraint)
			}
		}
		return nil
	}

	if srcID == "" {
		s, err := st.sourceForPackage(ctx, packageID)
		if err != nil {
			return err
		}
		srcID = s
	}

	manifest, err := st.index.Manifest(ctx, srcID, packageID)
	if err != nil {
		return err
	}

	chosen := firstNonEmpty(channel, defaultChannel(manifest), st.req.DefaultChannel)
	rel, err := installer.SelectRelease(manifest, chosen)
	if err != nil {
		return errpkg.Wrap("E_NO_MATCHING_RELEASE", err, "release for "+packageID+" channel "+chosen)
	}
	if constraint != "" {
		ok, cerr := installer.SatisfiesConstraint(rel.Version, constraint)
		if cerr != nil {
			return errpkg.Wrap("E_DEP_CONSTRAINT", cerr, packageID+" constraint "+constraint)
		}
		if !ok {
			return errpkg.New("E_DEP_CONSTRAINT_UNSAT", packageID+" "+rel.Version+" does not satisfy "+constraint)
		}
	}
	art, err := installer.SelectArtifact(rel, st.req.OS, st.req.Arch)
	if err != nil {
		return errpkg.Wrap("E_NO_MATCHING_RELEASE", err, "artifact for "+packageID)
	}

	st.active[packageID] = &Resolved{
		PackageID: packageID,
		Version:   rel.Version,
		Scope:     scopeOf(rel),
		Reason:    reason,
		Release:   rel,
		Artifact:  art,
		SourceID:  srcID,
	}
	st.manifests[packageID] = manifest

	for _, dep := range rel.Dependencies {
		st.edges[packageID] = append(st.edges[packageID], dep.Package)
		if err := st.resolvePackage(ctx, dep.Package, srcID, chosen, "dependency", dep.VersionConstraint); err != nil {
			return err
		}
	}
	return nil
}

func (st *resolution) validateHostTargets() error {
	for id := range st.active {
		manifest := st.manifests[id]
		for _, target := range manifest.Targets {
			host, ok := st.active[target.Package]
			if !ok {
				return errpkg.New("E_HOST_MISSING", id+" requires host "+target.Package+" which is not installed or in the plan")
			}
			st.edges[id] = append(st.edges[id], target.Package)
			if err := validateHostExposure(st.manifests[target.Package], host.Version, target); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateHostExposure(hostManifest metadata.PackageManifest, hostVersion string, target metadata.RuntimeTarget) error {
	var runtime *metadata.HostRuntime
	for i := range hostManifest.HostRuntimes {
		if hostManifest.HostRuntimes[i].Runtime == target.Runtime {
			runtime = &hostManifest.HostRuntimes[i]
		}
	}
	if target.Runtime != "" && runtime == nil {
		return errpkg.New("E_HOST_FEATURE_MISSING", "host does not expose runtime "+target.Runtime)
	}
	for _, want := range target.RequiredFeatures {
		if runtime == nil || !slices.Contains(runtime.Features, want) {
			return errpkg.New("E_HOST_FEATURE_MISSING", "host runtime "+target.Runtime+" missing feature "+want)
		}
	}
	if target.VersionConstraint != "" && hostVersion != "" {
		ok, err := installer.SatisfiesConstraint(hostVersion, target.VersionConstraint)
		if err != nil {
			return errpkg.Wrap("E_HOST_CONSTRAINT", err, "host "+target.Runtime+" constraint "+target.VersionConstraint)
		}
		if !ok {
			return errpkg.New("E_HOST_INCOMPATIBLE", "host "+hostVersion+" does not satisfy "+target.VersionConstraint)
		}
	}
	return nil
}

func (st *resolution) validateScopes(checker ScopeChecker) error {
	var hasUser, hasCampaign bool
	for _, res := range st.active {
		switch res.Scope {
		case ScopeCampaign:
			hasCampaign = true
		default:
			hasUser = true
		}
	}
	if hasCampaign && !checker.CampaignRootKnown() {
		return errpkg.New("E_CAMPAIGN_ROOT_REQUIRED", "a campaign-scope package requires a known campaign root")
	}
	if hasCampaign && hasUser && !checker.CanLockUserScope() {
		return errpkg.New("E_USER_SCOPE_LOCK", "a mixed user+campaign plan requires the user-scope lock")
	}
	return nil
}

func (st *resolution) topoOrder() ([]string, error) {
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	var order []string
	var visit func(id string) error
	visit = func(id string) error {
		switch color[id] {
		case black:
			return nil
		case gray:
			return errpkg.New("E_DEP_CYCLE", "dependency cycle at "+id)
		}
		color[id] = gray
		deps := append([]string(nil), st.edges[id]...)
		sort.Strings(deps)
		for _, dep := range deps {
			if _, ok := st.active[dep]; !ok {
				continue
			}
			if err := visit(dep); err != nil {
				return err
			}
		}
		color[id] = black
		order = append(order, id)
		return nil
	}

	ids := make([]string, 0, len(st.active))
	for id := range st.active {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	return order, nil
}

func (r *Resolver) ResolveUpdate(ctx context.Context, installedID, sourceID, channel string) (Resolved, error) {
	manifest, err := r.sources.Manifest(ctx, sourceID, installedID)
	if err != nil {
		return Resolved{}, err
	}
	chosen := firstNonEmpty(channel, defaultChannel(manifest))
	rel, err := installer.SelectRelease(manifest, chosen)
	if err != nil {
		return Resolved{}, errpkg.Wrap("E_NO_MATCHING_RELEASE", err, "update release for "+installedID)
	}
	return Resolved{
		PackageID: installedID,
		Version:   rel.Version,
		Scope:     scopeOf(rel),
		Reason:    "update",
		Release:   rel,
		SourceID:  sourceID,
	}, nil
}

func scopeOf(rel metadata.Release) Scope {
	for _, e := range rel.Install.Entries {
		if e.Kind == "skill_bundle" {
			return ScopeCampaign
		}
	}
	return ScopeUser
}

func defaultChannel(m metadata.PackageManifest) string {
	for _, rel := range m.Releases {
		if rel.Channel == "stable" {
			return "stable"
		}
	}
	if len(m.Releases) > 0 {
		return m.Releases[0].Channel
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
