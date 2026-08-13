package app

import (
	"context"
	"sort"
	"time"

	"github.com/Obedience-Corp/festival-installer/internal/state"
	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
)

// ListInstalled returns all package receipts.
func ListInstalled(ctx context.Context) (ListResult, error) {
	if err := ctx.Err(); err != nil {
		return ListResult{}, err
	}
	home, err := state.Home(ctx)
	if err != nil {
		return ListResult{}, err
	}
	if err := state.EnsureHome(ctx, 0o700); err != nil {
		return ListResult{}, err
	}
	db, err := state.OpenDB(ctx, home)
	if err != nil {
		return ListResult{}, err
	}
	defer func() { _ = db.Close(ctx) }()

	recs, err := receipts.List(ctx, db.Raw(), receipts.Filter{})
	if err != nil {
		return ListResult{}, err
	}
	out := ListResult{Packages: make([]ListEntry, 0, len(recs))}
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
	sort.Slice(out.Packages, func(i, j int) bool {
		return out.Packages[i].PackageID < out.Packages[j].PackageID
	})
	return out, nil
}
