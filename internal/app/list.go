package app

import (
	"context"
	"sort"
	"time"

	"github.com/Obedience-Corp/festival-installer/internal/state"
	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
)

// ListInstalled returns receipt-owned packages plus a synthetic suite row
// for package/leftover origins so the TUI has something to show without
// creating state.db.
func ListInstalled(ctx context.Context) (ListResult, error) {
	if err := ctx.Err(); err != nil {
		return ListResult{}, err
	}
	origin, _ := DetectSuite(ctx)
	synth, haveSynth := syntheticSuiteEntry(origin)

	home, err := state.Home(ctx)
	if err != nil {
		if haveSynth {
			return ListResult{Packages: []ListEntry{synth}}, nil
		}
		return ListResult{}, err
	}
	db, ok, err := state.OpenDBIfExists(ctx, home)
	if err != nil {
		return ListResult{}, err
	}
	if !ok {
		if haveSynth {
			return ListResult{Packages: []ListEntry{synth}}, nil
		}
		return ListResult{}, nil
	}
	defer func() { _ = db.Close(ctx) }()

	recs, err := receipts.List(ctx, db.Raw(), receipts.Filter{})
	if err != nil {
		return ListResult{}, err
	}
	out := ListResult{Packages: make([]ListEntry, 0, len(recs)+1)}
	for _, r := range recs {
		files := make([]string, 0, len(r.OwnedFiles))
		for _, f := range r.OwnedFiles {
			files = append(files, f.Path)
		}
		installedAt := ""
		if !r.InstalledAt.IsZero() {
			installedAt = r.InstalledAt.UTC().Format(time.RFC3339)
		}
		out.Packages = append(out.Packages, ListEntry{
			PackageID:   r.PackageID,
			Version:     r.Version,
			Channel:     r.Channel,
			Source:      r.Source,
			InstalledAt: installedAt,
			Files:       files,
		})
	}
	if haveSynth {
		found := false
		for i, p := range out.Packages {
			if p.PackageID != synth.PackageID {
				continue
			}
			found = true
			if out.Packages[i].Origin == "" {
				out.Packages[i].Origin = synth.Origin
			}
			if out.Packages[i].Source == "" {
				out.Packages[i].Source = synth.Source
			}
			break
		}
		if !found {
			out.Packages = append(out.Packages, synth)
		}
	}
	sort.Slice(out.Packages, func(i, j int) bool {
		return out.Packages[i].PackageID < out.Packages[j].PackageID
	})
	return out, nil
}

func syntheticSuiteEntry(origin SuiteOrigin) (ListEntry, bool) {
	switch origin.Kind {
	case OriginPackage:
		src := string(origin.Flavor)
		if origin.Package != "" {
			src = string(origin.Flavor) + ":" + origin.Package
		}
		return ListEntry{
			PackageID: FestivalPackageID,
			Version:   origin.Version,
			Channel:   origin.RelChannel,
			Source:    src,
			Origin:    "package",
		}, true
	case OriginLeftover:
		return ListEntry{
			PackageID: FestivalPackageID,
			Version:   origin.Version,
			Source:    origin.Prefix,
			Origin:    "leftover",
		}, true
	default:
		return ListEntry{}, false
	}
}
