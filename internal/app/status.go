package app

import (
	"context"

	"github.com/Obedience-Corp/festival-installer/internal/state"
)

// Status returns a summary for the TUI home screen.
func Status(ctx context.Context) (StatusSummary, error) {
	binDir, _ := state.BinDir(ctx)
	sum := StatusSummary{
		ManagedBin:       binDir,
		ManagedBinOnPath: binDir != "" && dirOnPath(binDir),
		Action:           "absent",
	}
	origin, err := DetectSuite(ctx)
	if err != nil {
		// Keep a paintable summary; surface the error only as a note.
		sum.ShadowNote = err.Error()
		err = nil
	}
	sum.Prefix = origin.Prefix
	sum.Dual = origin.Dual
	sum.Version = origin.Version
	sum.Channel = origin.RelChannel
	switch origin.Kind {
	case OriginManaged:
		sum.Action = "managed"
		sum.Installed = true
		if rec, found, rerr := ReadFestivalReceipt(ctx); rerr == nil && found {
			sum.Version = rec.Version
			sum.Channel = rec.Channel
			sum.Source = rec.Source
		}
	case OriginPackage:
		sum.Action = "package"
		sum.Installed = true
		sum.Source = string(origin.Flavor)
		if origin.Package != "" {
			sum.Source = string(origin.Flavor) + ":" + origin.Package
		}
	case OriginLeftover:
		sum.Action = "unmanaged"
	default:
		sum.Action = "absent"
	}
	if len(origin.Shadows) > 0 {
		sum.ShadowNote = "leftover binaries on PATH"
	}
	return sum, err
}
