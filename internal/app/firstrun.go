package app

import (
	"context"
	"database/sql"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/source"
	"github.com/Obedience-Corp/festival-installer/internal/state"
	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
)

// SetupState describes how far along a FESTIVAL_HOME is toward being usable.
// It exists so doctor, the CLI and the TUI agree on one answer rather than each
// re-deriving it from the same three signals.
type SetupState struct {
	// HasReceipts is true once anything has been installed.
	HasReceipts bool `json:"has_receipts"`
	// HasMarketplaces is true once at least one source is registered.
	HasMarketplaces bool `json:"has_marketplaces"`
	// ManagedBinOnPath is true once the managed bin dir is reachable via PATH.
	ManagedBinOnPath bool `json:"managed_bin_on_path"`
	// ManagedBin is the managed bin dir, for rendering.
	ManagedBin string `json:"managed_bin,omitempty"`
}

// IsFirstRun reports whether this home has never been set up. All three signals
// must be absent: a home with a marketplace but no receipts is mid-setup, not a
// first run, and should not get first-run guidance.
func (s SetupState) IsFirstRun() bool {
	return !s.HasReceipts && !s.HasMarketplaces && !s.ManagedBinOnPath
}

// NeedsSetup reports whether anything still stands between this home and a
// working install. A registered marketplace is not enough on its own, so it is
// deliberately not part of this answer.
func (s SetupState) NeedsSetup() bool {
	return !s.HasReceipts || !s.ManagedBinOnPath
}

// ResolveSetupState computes the setup state from the three signals that decide
// whether a home has been set up.
//
// The only error it returns is a cancelled context. Every other read degrades:
// an unresolvable home, a locked database or an unreadable table all mean "no
// signal we can see", which produces the same guidance as the signal being
// absent. Reporting a broken home as a failure belongs to doctor, whose
// receipts_integrity and sources_reachable checks already do it, and which must
// keep working even when this function has nothing to say.
//
// It never creates the installer home or state.db, matching the read-only
// contract every other status path in this package holds to.
func ResolveSetupState(ctx context.Context) (SetupState, error) {
	if err := ctx.Err(); err != nil {
		return SetupState{}, errpkg.Wrap("E_SETUP_CTX", err, "context cancelled before resolving setup state")
	}
	st := SetupState{}
	if binDir, err := state.BinDir(ctx); err == nil && binDir != "" {
		st.ManagedBin = binDir
		st.ManagedBinOnPath = dirOnPath(binDir)
	}
	db, ok := openHomeDBIfExists(ctx)
	if !ok {
		return st, nil
	}
	defer func() { _ = db.Close(ctx) }()

	st.HasReceipts = hasAnyReceipt(ctx, db.Raw())
	st.HasMarketplaces = hasAnyMarketplace(ctx, db.Raw())
	return st, nil
}

// openHomeDBIfExists opens the installer database when it already exists,
// reporting only whether a usable handle came back. Callers here treat every
// failure the same way they treat a missing file.
func openHomeDBIfExists(ctx context.Context) (*state.DB, bool) {
	home, err := state.Home(ctx)
	if err != nil {
		return nil, false
	}
	db, ok, err := state.OpenDBIfExists(ctx, home)
	if err != nil || !ok || db == nil {
		return nil, false
	}
	return db, true
}

func hasAnyReceipt(ctx context.Context, db *sql.DB) bool {
	recs, err := receipts.List(ctx, db, receipts.Filter{})
	return err == nil && len(recs) > 0
}

// hasAnyMarketplace counts registered sources straight from the registry table
// rather than through source.ListMarketplacesIfExists, which also stats clone
// directories and verifies signatures. Registration is the signal; whether a
// source currently verifies is doctor's marketplace_trust check.
func hasAnyMarketplace(ctx context.Context, db *sql.DB) bool {
	srcs, err := source.List(ctx, db)
	return err == nil && len(srcs) > 0
}
