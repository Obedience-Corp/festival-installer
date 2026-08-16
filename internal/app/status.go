package app

import (
	"context"

	"github.com/Obedience-Corp/festival-installer/internal/state"
)

// Status returns a summary for the TUI home screen.
func Status(ctx context.Context) (StatusSummary, error) {
	_ = state.EnsureHome(ctx, 0o700)
	binDir, err := state.BinDir(ctx)
	if err != nil {
		return StatusSummary{Action: "absent"}, err
	}
	sum := StatusSummary{
		ManagedBin:       binDir,
		ManagedBinOnPath: dirOnPath(binDir),
		Action:           "absent",
	}

	rec, found, err := ReadFestivalReceipt(ctx)
	if err != nil {
		// Still return a usable summary so the TUI never sticks on "checking…".
		return sum, err
	}
	if found {
		sum.Installed = true
		sum.Version = rec.Version
		sum.Channel = rec.Channel
		sum.Source = rec.Source
		sum.Action = "managed"
		return sum, nil
	}

	if _, werr := ResolveWhich(ctx, "camp"); werr == nil {
		sum.Action = "unmanaged"
		return sum, nil
	}
	sum.Action = "absent"
	return sum, nil
}
