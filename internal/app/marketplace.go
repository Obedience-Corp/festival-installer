package app

import (
	"context"
	"fmt"

	"github.com/Obedience-Corp/festival-installer/internal/source"
)

// MarketplaceSeedWarning reports that the official source could not be
// bootstrapped while still allowing callers to render any existing sources.
// CLI callers print it as a warning; TUI callers keep it alongside the views.
type MarketplaceSeedWarning struct {
	Err error
}

// marketplaceSeedFriendlyMessage is the user-facing rendering of a
// MarketplaceSeedWarning. It intentionally omits Err's detail (which can
// contain raw git command output, e.g. an auth failure trace) so terminal
// and JSON consumers never see it.
const marketplaceSeedFriendlyMessage = "couldn't reach the official marketplace; showing local sources only"

func (w *MarketplaceSeedWarning) Error() string {
	return fmt.Sprintf("could not seed official marketplace: %v", w.Err)
}

// Friendly returns the one-line, user-facing rendering of this warning.
// Callers rendering to a terminal or a JSON envelope should use this instead
// of Error(), which carries full diagnostic detail meant for debugging.
func (w *MarketplaceSeedWarning) Friendly() string { return marketplaceSeedFriendlyMessage }

func (w *MarketplaceSeedWarning) Unwrap() error { return w.Err }

// MarketplaceAdd clones and registers a marketplace git URL.
func MarketplaceAdd(ctx context.Context, url, name string) (source.Source, error) {
	return source.AddMarketplace(ctx, url, name)
}

// MarketplaceRemove drops a registered marketplace.
func MarketplaceRemove(ctx context.Context, name string) error {
	return source.RemoveMarketplace(ctx, name)
}

// MarketplaceList returns marketplace views (seeding official if needed).
// Views are never nil so JSON consumers always see an array.
func MarketplaceList(ctx context.Context) ([]source.ListView, error) {
	seedErr := ensureOfficialSeed(ctx)
	views, err := source.ListMarketplaces(ctx)
	if err != nil {
		return nil, err
	}
	if views == nil {
		views = []source.ListView{}
	}
	if seedErr != nil {
		return views, &MarketplaceSeedWarning{Err: seedErr}
	}
	return views, nil
}

// MarketplaceRefresh refreshes one or all marketplaces.
// Views are never nil so JSON consumers always see an array.
func MarketplaceRefresh(ctx context.Context, name string) ([]source.RefreshView, error) {
	seedErr := ensureOfficialSeed(ctx)
	views, err := source.RefreshMarketplaces(ctx, name)
	if err != nil {
		return nil, err
	}
	if views == nil {
		views = []source.RefreshView{}
	}
	if seedErr != nil {
		return views, &MarketplaceSeedWarning{Err: seedErr}
	}
	return views, nil
}
