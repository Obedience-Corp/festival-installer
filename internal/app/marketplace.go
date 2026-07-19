package app

import (
	"context"

	"github.com/Obedience-Corp/obey-installer/internal/source"
)

// MarketplaceAdd clones and registers a marketplace git URL.
func MarketplaceAdd(ctx context.Context, url, name string) (source.Source, error) {
	return source.AddMarketplace(ctx, url, name)
}

// MarketplaceRemove drops a registered marketplace.
func MarketplaceRemove(ctx context.Context, name string) error {
	return source.RemoveMarketplace(ctx, name)
}

// MarketplaceList returns marketplace views (seeding official if needed).
func MarketplaceList(ctx context.Context) ([]source.ListView, error) {
	_ = source.EnsureOfficialSeed(ctx)
	return source.ListMarketplaces(ctx)
}

// MarketplaceRefresh refreshes one or all marketplaces.
func MarketplaceRefresh(ctx context.Context, name string) ([]source.RefreshView, error) {
	_ = source.EnsureOfficialSeed(ctx)
	return source.RefreshMarketplaces(ctx, name)
}
