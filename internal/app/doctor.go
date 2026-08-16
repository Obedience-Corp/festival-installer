package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/Obedience-Corp/festival-installer/internal/artifacts"
	"github.com/Obedience-Corp/festival-installer/internal/source"
	"github.com/Obedience-Corp/festival-installer/internal/state"
	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
)

// Doctor runs health checks for PATH, sources, receipts, and shadowing.
func Doctor(ctx context.Context) []DoctorCheck {
	// Ensure the manager home exists so source/receipt checks do not fail with
	// mkdir/db errors on a brand-new FESTIVAL_HOME.
	_ = state.EnsureHome(ctx, 0o700)
	return []DoctorCheck{
		checkManagedBinOnPath(ctx),
		checkSourcesReachable(ctx),
		checkReceiptsIntegrity(ctx),
		checkPathShadowing(ctx),
	}
}

// DoctorFailed reports whether any check failed.
func DoctorFailed(checks []DoctorCheck) bool {
	for _, c := range checks {
		if c.Status == "fail" {
			return true
		}
	}
	return false
}

func checkManagedBinOnPath(ctx context.Context) DoctorCheck {
	c := DoctorCheck{ID: "managed_bin_on_path"}
	binDir, err := state.BinDir(ctx)
	if err != nil {
		c.Status = "fail"
		c.Message = err.Error()
		return c
	}
	if dirOnPath(binDir) {
		c.Status = "ok"
		c.Message = "managed bin dir is on PATH: " + binDir
	} else {
		c.Status = "fail"
		c.Message = "managed bin dir is not on PATH: " + binDir + " (run: eval \"$(festival shell-init zsh)\")"
	}
	return c
}

func checkSourcesReachable(ctx context.Context) DoctorCheck {
	c := DoctorCheck{ID: "sources_reachable"}
	views, err := source.ListMarketplaces(ctx)
	if err != nil {
		c.Status = "fail"
		c.Message = err.Error()
		return c
	}
	var broken []string
	for _, v := range views {
		if v.Err != "" {
			broken = append(broken, v.Name)
		}
	}
	switch {
	case len(views) == 0:
		c.Status = "warn"
		c.Message = "no marketplaces registered"
	case len(broken) > 0:
		c.Status = "fail"
		c.Message = "unreachable sources: " + strings.Join(broken, ", ")
	default:
		c.Status = "ok"
		c.Message = fmt.Sprintf("%d source(s) reachable", len(views))
	}
	return c
}

func checkReceiptsIntegrity(ctx context.Context) DoctorCheck {
	c := DoctorCheck{ID: "receipts_integrity"}
	home, err := state.Home(ctx)
	if err != nil {
		c.Status = "fail"
		c.Message = err.Error()
		return c
	}
	db, err := state.OpenDB(ctx, home)
	if err != nil {
		c.Status = "fail"
		c.Message = err.Error()
		return c
	}
	defer func() { _ = db.Close(ctx) }()

	recs, err := receipts.List(ctx, db.Raw(), receipts.Filter{})
	if err != nil {
		c.Status = "fail"
		c.Message = err.Error()
		return c
	}
	var problems []string
	for _, r := range recs {
		for _, f := range r.OwnedFiles {
			if f.Hash == "" {
				continue
			}
			if err := artifacts.VerifySHA256(ctx, f.Path, f.Hash); err != nil {
				problems = append(problems, r.PackageID+":"+f.Path)
			}
		}
	}
	switch {
	case len(recs) == 0:
		c.Status = "ok"
		c.Message = "no receipts"
	case len(problems) > 0:
		c.Status = "fail"
		c.Message = "orphan or mismatched receipt files: " + strings.Join(problems, ", ")
	default:
		c.Status = "ok"
		c.Message = fmt.Sprintf("%d receipt(s) verified", len(recs))
	}
	return c
}

func checkPathShadowing(ctx context.Context) DoctorCheck {
	c := DoctorCheck{ID: "path_shadowing", Status: "ok", Message: "no managed binary is shadowed"}
	var shadowed []string
	for _, tool := range []string{"camp", "fest"} {
		res, err := ResolveWhich(ctx, tool)
		if err != nil {
			continue
		}
		if res.Shadowed {
			shadowed = append(shadowed, tool)
		}
	}
	if len(shadowed) > 0 {
		c.Status = "warn"
		c.Message = "managed binaries shadowed on PATH: " + strings.Join(shadowed, ", ")
	}
	return c
}
