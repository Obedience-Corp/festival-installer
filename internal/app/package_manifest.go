package app

import (
	"context"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/metadata"
	"github.com/Obedience-Corp/festival-installer/internal/source"
)

var (
	refreshMarketplacesForLoad = source.RefreshMarketplaces
	loadPackageManifest        = source.LoadPackageManifest
)

// loadFreshPackageManifest refreshes a package's recorded marketplace before
// resolving its manifest so install and update never mistake a stale cache for
// the channel-latest release.
func loadFreshPackageManifest(ctx context.Context, sourceName, packageID string, vo source.VerifyOptions) (metadata.PackageManifest, error) {
	views, err := refreshMarketplacesForLoad(ctx, sourceName, vo)
	if err != nil {
		return metadata.PackageManifest{}, errpkg.Wrap("E_MARKETPLACE_REFRESH", err, "refresh marketplace "+sourceName)
	}
	if len(views) != 1 {
		return metadata.PackageManifest{}, errpkg.New("E_MARKETPLACE_REFRESH", "refresh marketplace "+sourceName+" returned no result")
	}
	if views[0].Err != "" {
		return metadata.PackageManifest{}, errpkg.New("E_MARKETPLACE_REFRESH", "refresh marketplace "+sourceName+": "+views[0].Err)
	}
	return loadPackageManifest(ctx, sourceName, packageID, vo)
}
