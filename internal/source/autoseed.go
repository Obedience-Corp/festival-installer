package source

import (
	"context"

	"github.com/Obedience-Corp/festival-installer/internal/state"
)

var officialMarketplaceURL = "https://github.com/Obedience-Corp/marketplace.git"

// EnsureOfficialSeed clones and registers the official marketplace the first
// time festival runs with no sources configured. vo carries the caller's key
// store, allow-unverified override, and warn writer; the policy itself is
// always the official seed's (policyFor), not caller-chosen.
func EnsureOfficialSeed(ctx context.Context, vo VerifyOptions) error {
	_, err := seedFromURL(ctx, officialMarketplaceURL, vo)
	return err
}

func seedFromURL(ctx context.Context, gitURL string, vo VerifyOptions) (bool, error) {
	added := false
	err := withManager(ctx, func(ctx context.Context, db *state.DB) error {
		seeded, err := state.SeedMarkerExists(ctx, db.Raw(), state.OfficialSeedKey)
		if err != nil {
			return err
		}
		if seeded {
			return nil
		}

		sources, err := List(ctx, db.Raw())
		if err != nil {
			return err
		}
		if len(sources) > 0 {
			return state.RecordSeedMarker(ctx, db.Raw(), state.OfficialSeedKey)
		}

		dest, err := CloneDir(ctx, state.OfficialSeedKey)
		if err != nil {
			return err
		}
		commit, err := Clone(ctx, gitURL, dest)
		if err != nil {
			_ = RemoveClone(ctx, dest)
			return err
		}
		vo.Policy = policyFor(state.OfficialSeedKey)
		vo.SourceLabel = state.OfficialSeedKey
		if _, err := LoadMarketplace(ctx, dest, vo); err != nil {
			_ = RemoveClone(ctx, dest)
			return err
		}
		src := Source{Name: state.OfficialSeedKey, URL: gitURL, Commit: commit}
		if err := Add(ctx, db.Raw(), src); err != nil {
			_ = RemoveClone(ctx, dest)
			return err
		}
		if err := state.RecordSeedMarker(ctx, db.Raw(), state.OfficialSeedKey); err != nil {
			return err
		}
		added = true
		return nil
	})
	return added, err
}
