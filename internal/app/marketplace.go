package app

import (
	"context"
	stderrors "errors"
	"fmt"

	"github.com/Obedience-Corp/festival-installer/internal/source"
)

// MarketplaceSeedProblem reports that the official source could not be
// bootstrapped. It covers both outcomes, because the failure is the same and
// only the caller's tolerance for it differs: a read path can still render the
// sources it already has, while install has nothing to fall back on.
//
// The type is deliberately not called a warning. It used to be, and install then
// had to either mislabel a fatal condition or reach around it and print the raw
// git chain, which is the bug this type now closes.
type MarketplaceSeedProblem struct {
	Err error
	// Fatal marks the case where the caller could not continue. It only selects
	// the wording; both cases hide Err from the terminal.
	Fatal bool
}

// Seed problem renderings. Both intentionally omit Err's detail (which can
// contain raw git command output, for example an auth failure trace) so
// terminal and JSON consumers never see it. The fatal line is more directive
// because the user asked for something that could not happen.
const (
	marketplaceSeedFriendlyMessage = "couldn't reach the official marketplace; showing local sources only"
	marketplaceSeedFatalMessage    = "couldn't reach the official marketplace; check your network, " +
		"or add a marketplace with 'festival marketplace add <url>'"
)

func (w *MarketplaceSeedProblem) Error() string {
	return fmt.Sprintf("could not seed official marketplace: %v", w.Err)
}

// Friendly returns the one-line, user-facing rendering of this problem.
// Callers rendering to a terminal or a JSON envelope should use this instead
// of Error(), which carries full diagnostic detail meant for debugging.
func (w *MarketplaceSeedProblem) Friendly() string {
	if w.Fatal {
		return marketplaceSeedFatalMessage
	}
	return marketplaceSeedFriendlyMessage
}

func (w *MarketplaceSeedProblem) Unwrap() error { return w.Err }

// friendlyError is implemented by errors whose Error() carries diagnostic
// detail, such as raw git command output, that must never reach a user.
type friendlyError interface {
	Friendly() string
}

// FriendlyMessage returns the text a user should see for err. Every surface that
// renders an error to a person routes through here, so an error that hides its
// diagnostic detail hides it on the CLI, in the JSON envelope and in the TUI
// alike rather than in whichever one someone remembered.
func FriendlyMessage(err error) string {
	if err == nil {
		return ""
	}
	var f friendlyError
	if stderrors.As(err, &f) {
		return f.Friendly()
	}
	return err.Error()
}

// MarketplaceAdd clones and registers a marketplace git URL.
func MarketplaceAdd(ctx context.Context, url, name string, vo source.VerifyOptions) (source.Source, error) {
	return source.AddMarketplace(ctx, url, name, vo)
}

// MarketplaceRemove drops a registered marketplace.
func MarketplaceRemove(ctx context.Context, name string) error {
	return source.RemoveMarketplace(ctx, name)
}

// MarketplaceListExisting lists registered marketplaces without seeding or
// creating installer home. Missing state.db yields an empty list.
func MarketplaceListExisting(ctx context.Context, vo source.VerifyOptions) ([]source.ListView, error) {
	views, err := source.ListMarketplacesIfExists(ctx, vo)
	if err != nil {
		return nil, err
	}
	if views == nil {
		views = []source.ListView{}
	}
	return views, nil
}

// MarketplaceSeedOfficial clones and registers the official marketplace.
// This mkdir's installer home; it is the explicit TUI `s` / CLI first-run path.
// Browse auto-seed (Browse -> ensureOfficialSeed) remains the other mkdir exception.
func MarketplaceSeedOfficial(ctx context.Context, vo source.VerifyOptions) error {
	return ensureOfficialSeed(ctx, vo)
}

// MarketplaceList returns marketplace views (seeding official if needed).
// Views are never nil so JSON consumers always see an array.
func MarketplaceList(ctx context.Context, vo source.VerifyOptions) ([]source.ListView, error) {
	seedErr := ensureOfficialSeed(ctx, vo)
	views, err := source.ListMarketplaces(ctx, vo)
	if err != nil {
		return nil, err
	}
	if views == nil {
		views = []source.ListView{}
	}
	if seedErr != nil {
		return views, &MarketplaceSeedProblem{Err: seedErr}
	}
	return views, nil
}

// MarketplaceRefresh refreshes one or all marketplaces.
// Views are never nil so JSON consumers always see an array.
func MarketplaceRefresh(ctx context.Context, name string, vo source.VerifyOptions) ([]source.RefreshView, error) {
	seedErr := ensureOfficialSeed(ctx, vo)
	views, err := source.RefreshMarketplaces(ctx, name, vo)
	if err != nil {
		return nil, err
	}
	if views == nil {
		views = []source.RefreshView{}
	}
	if seedErr != nil {
		return views, &MarketplaceSeedProblem{Err: seedErr}
	}
	return views, nil
}
