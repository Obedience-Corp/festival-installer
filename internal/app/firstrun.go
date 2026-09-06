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
	// SignalsIncomplete is true when a signal could not be read at all: an
	// unresolvable home, or a database that exists but will not open. Such a
	// home is not a first run. It has state, we just cannot see it, and telling
	// its owner that nothing is set up would be wrong during exactly the
	// incident where the guidance matters most.
	SignalsIncomplete bool `json:"signals_incomplete,omitempty"`
}

// A SetupState answers one of three questions, and Unknown excludes the other
// two: either the hub could not read this home at all, or it could, in which
// case the home is a first run, needs setup, or is ready. Callers that render
// guidance ask Unknown first, because guidance derived from signals nobody could
// read is guidance about a machine that does not exist.

// Unknown reports whether a signal could not be read at all. Nothing about how
// far along this home is can be concluded while it is true.
func (s SetupState) Unknown() bool { return s.SignalsIncomplete }

// IsFirstRun reports whether this home has never been set up. All three signals
// must be absent: a home with a marketplace but no receipts is mid-setup, not a
// first run, and should not get first-run guidance. A home whose signals could
// not be read is never a first run.
func (s SetupState) IsFirstRun() bool {
	if s.Unknown() {
		return false
	}
	return !s.HasReceipts && !s.HasMarketplaces && !s.ManagedBinOnPath
}

// NeedsSetup reports whether anything still stands between this home and a
// working install. A registered marketplace is not enough on its own, so it is
// deliberately not part of this answer.
//
// It is false when the signals could not be read. An existing home whose
// database is locked or unreadable reads as no receipts and no PATH, and
// answering "you still need to install the suite" would dress an incident up as
// onboarding for the one user who most needs to know something is wrong.
func (s SetupState) NeedsSetup() bool {
	if s.Unknown() {
		return false
	}
	return !s.HasReceipts || !s.ManagedBinOnPath
}

// ResolveSetupState computes the setup state from the three signals that decide
// whether a home has been set up.
//
// The only error it returns is a cancelled context. Every other read degrades
// into SignalsIncomplete: an unresolvable home, a locked database or an
// unreadable table all mean "no signal we can see", which is reported as its own
// state rather than folded into the signal being absent. Callers ask Unknown
// before concluding anything. Reporting a broken home as a failure belongs to
// doctor, whose receipts_integrity and sources_reachable checks already do it,
// and which must keep working even when this function has nothing to say.
//
// It never creates the installer home or state.db, matching the read-only
// contract every other status path in this package holds to.
func ResolveSetupState(ctx context.Context) (SetupState, error) {
	if err := ctx.Err(); err != nil {
		return SetupState{}, errpkg.Wrap("E_SETUP_CTX", err, "context cancelled before resolving setup state")
	}
	st := SetupState{}
	binDir, err := state.BinDir(ctx)
	if err != nil || binDir == "" {
		st.SignalsIncomplete = true
	} else {
		st.ManagedBin = binDir
		st.ManagedBinOnPath = dirOnPath(binDir)
	}
	db, readable := openHomeDBIfExists(ctx)
	if db == nil {
		st.SignalsIncomplete = st.SignalsIncomplete || !readable
		return st, nil
	}
	defer func() { _ = db.Close(ctx) }()

	receipts, receiptsOK := anyReceipt(ctx, db.Raw())
	sources, sourcesOK := anyMarketplace(ctx, db.Raw())
	st.HasReceipts = receipts
	st.HasMarketplaces = sources
	if !receiptsOK || !sourcesOK {
		st.SignalsIncomplete = true
	}
	return st, nil
}

// openHomeDBIfExists opens the installer database when it already exists. The
// second return distinguishes the two ways of getting no handle: a home with no
// database yet (readable, and a genuine first run) from one whose database is
// there but will not open (not readable, and not a first run).
func openHomeDBIfExists(ctx context.Context) (db *state.DB, readable bool) {
	home, err := state.Home(ctx)
	if err != nil {
		return nil, false
	}
	opened, exists, err := state.OpenDBIfExists(ctx, home)
	if err != nil {
		return nil, false
	}
	if !exists || opened == nil {
		return nil, true
	}
	return opened, true
}

func anyReceipt(ctx context.Context, db *sql.DB) (found, ok bool) {
	recs, err := receipts.List(ctx, db, receipts.Filter{})
	if err != nil {
		return false, false
	}
	return len(recs) > 0, true
}

// anyMarketplace counts registered sources straight from the registry table
// rather than through source.ListMarketplacesIfExists, which also stats clone
// directories and verifies signatures. Registration is the signal; whether a
// source currently verifies is doctor's marketplace_trust check.
func anyMarketplace(ctx context.Context, db *sql.DB) (found, ok bool) {
	srcs, err := source.List(ctx, db)
	if err != nil {
		return false, false
	}
	return len(srcs) > 0, true
}
