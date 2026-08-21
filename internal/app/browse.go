package app

import (
	"context"
	"sort"
	"strings"

	"github.com/Obedience-Corp/festival-installer/internal/source"
)

const noHostRuntime = "(unspecified)"

// BrowseOptions filter the merged catalog.
type BrowseOptions struct {
	Product string
	Kind    string
	Verify  source.VerifyOptions
}

// Browse returns packages grouped by host runtime (seeding official if needed).
func Browse(ctx context.Context, opts BrowseOptions) (BrowseResult, error) {
	seedErr := ensureOfficialSeed(ctx, opts.Verify)
	pkgs, err := source.AllPackages(ctx, opts.Verify)
	if err != nil {
		return BrowseResult{}, err
	}
	res := BuildBrowseResult(pkgs, opts.Product, opts.Kind)
	if seedErr != nil {
		return res, &MarketplaceSeedWarning{Err: seedErr}
	}
	return res, nil
}

func matchKind(p source.PackageRef, kind string) bool {
	return kind == "" || p.Class == kind
}

func matchProduct(p source.PackageRef, product string) bool {
	if product == "" {
		return true
	}
	if runtimeMatches(p.HostRuntimes, product) {
		return true
	}
	for _, t := range p.Targets {
		if runtimeMatches([]string{t.Runtime}, product) {
			return true
		}
	}
	return false
}

func runtimeMatches(runtimes []string, product string) bool {
	for _, r := range runtimes {
		if r == product || strings.HasPrefix(r, product+"-") {
			return true
		}
	}
	return false
}

// BuildBrowseResult groups and filters packages (exported for tests).
func BuildBrowseResult(pkgs []source.BrowsePackage, product, kind string) BrowseResult {
	grouped := map[string][]BrowseEntry{}
	for _, bp := range pkgs {
		p := bp.Package
		if !matchKind(p, kind) || !matchProduct(p, product) {
			continue
		}
		entry := BrowseEntry{
			ID:           p.ID,
			DisplayName:  p.DisplayName,
			Class:        p.Class,
			Summary:      p.Summary,
			HostRuntimes: nonNilStrings(p.HostRuntimes),
			Channels:     nonNilStrings(p.Channels),
			Source:       bp.Source,
		}
		runtimes := p.HostRuntimes
		if len(runtimes) == 0 {
			runtimes = []string{noHostRuntime}
		}
		for _, hr := range runtimes {
			grouped[hr] = append(grouped[hr], entry)
		}
	}

	keys := make([]string, 0, len(grouped))
	for k := range grouped {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	result := BrowseResult{Groups: []BrowseGroup{}}
	for _, k := range keys {
		entries := grouped[k]
		sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
		result.Groups = append(result.Groups, BrowseGroup{HostRuntime: k, Packages: entries})
	}
	return result
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
