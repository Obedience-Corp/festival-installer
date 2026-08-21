package source

import (
	"context"
	"os"

	"github.com/Obedience-Corp/festival-installer/internal/state"
)

var officialMarketplaceURL = "https://github.com/Obedience-Corp/marketplace.git"

func EnsureOfficialSeed(ctx context.Context) error {
	_, err := seedFromURL(ctx, officialMarketplaceURL)
	return err
}

func seedFromURL(ctx context.Context, gitURL string) (bool, error) {
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
		// TODO(task 02): this placeholder policy is replaced with the real
		// per-site VerifyOptions once EnsureOfficialSeed/seedFromURL thread vo.
		if _, err := LoadMarketplace(ctx, dest, DefaultVerifyOptions(os.Stderr, false)); err != nil {
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
