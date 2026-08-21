package app

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/Obedience-Corp/festival-installer/internal/artifacts"
	"github.com/Obedience-Corp/festival-installer/internal/source"
	"github.com/Obedience-Corp/festival-installer/internal/state"
	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
	"github.com/Obedience-Corp/festival-installer/internal/verify"
)

// managedBinaries are the binaries the suite places and therefore the ones
// doctor checks for shadowing. Kept as a constant list rather than derived
// from the receipt because the check must also work on a fresh home where no
// receipt exists yet, which is exactly the first-run case doctor cares about.
var managedBinaries = []string{"camp", "fest", selfBinaryName}

// Doctor runs health checks for PATH, sources, receipts, and shadowing.
func Doctor(ctx context.Context) []DoctorCheck {
	// Ensure the manager home exists so source/receipt checks do not fail with
	// mkdir/db errors on a brand-new FESTIVAL_HOME.
	_ = state.EnsureHome(ctx, 0o700)
	return []DoctorCheck{
		checkManagedBinOnPath(ctx),
		checkSourcesReachable(ctx),
		checkMarketplaceTrust(ctx),
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
	// This check is about reachability, not verification; checkMarketplaceTrust
	// below is the dedicated check that reads ListView.Verified.
	views, err := source.ListMarketplaces(ctx, source.DefaultVerifyOptions(nil, false))
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

// checkMarketplaceTrust reports whether every registered source verifies
// against the pinned trust root, so a stale or missing signature is visible
// in one command instead of requiring someone to diff a manifest against its
// detached .sig by hand.
func checkMarketplaceTrust(ctx context.Context) DoctorCheck {
	// Doctor never installs; it only reports. AllowUnverified is true and the
	// warn writer is discarded so an unsigned source does not abort the check
	// or corrupt `doctor --json` output. This does not weaken verification: a
	// tampered document still fails with E_SIG_INVALID (surfaced via
	// ListView.Err), and ListView.Verified is still false for anything
	// unsigned. Policy is intentionally not set here: ListMarketplaces routes
	// every source through voFor, which overwrites Policy per source anyway
	// (internal/source/manager.go), so a value here would never take effect.
	vo := source.VerifyOptions{
		KeyStore:        verify.PinnedKeyStore(),
		AllowUnverified: true,
		WarnWriter:      io.Discard,
	}
	views, err := source.ListMarketplaces(ctx, vo)
	if err != nil {
		return DoctorCheck{ID: "marketplace_trust", Status: "fail", Message: err.Error()}
	}
	return marketplaceTrustFrom(views)
}

// signatureErrorCodes are errpkg codes that mean a signature was present but
// could not be trusted: malformed, wrong algorithm, unknown key id, does not
// verify, or the .sig file itself could not be read. Distinct from an absent
// signature (ListView.Err == "", ListView.Verified == false), which is the
// benign, expected state for a third-party source with no key infrastructure.
var signatureErrorCodes = []string{
	"E_SIG_INVALID", "E_SIG_MALFORMED", "E_SIG_ALG", "E_SIG_CTX", "E_KEY_NOT_FOUND", "E_MARKETPLACE_SIG_READ",
}

func isSignatureError(errText string) bool {
	for _, code := range signatureErrorCodes {
		if strings.Contains(errText, code) {
			return true
		}
	}
	return false
}

// marketplaceTrustFrom is the pure decision logic behind checkMarketplaceTrust.
// It is unexported but tested directly (package app, doctor_internal_test.go)
// so it can be table-tested without a database or a filesystem. An unverified
// official source always fails, even alongside an unverified third party,
// because the official source is signed and unsigned content there means
// something is wrong. A third-party source with a present-but-invalid
// signature also fails, distinctly from one with no signature at all: a
// tampered document must not read the same as a normal, knowingly-unsigned
// add, or the message stops being actionable.
func marketplaceTrustFrom(views []source.ListView) DoctorCheck {
	c := DoctorCheck{ID: "marketplace_trust"}
	var officialUnverified, thirdPartyInvalid, thirdPartyUnsigned []string
	for _, v := range views {
		if v.Verified {
			continue
		}
		if v.Name == state.OfficialSeedKey {
			officialUnverified = append(officialUnverified, v.Name)
			continue
		}
		if isSignatureError(v.Err) {
			thirdPartyInvalid = append(thirdPartyInvalid, v.Name)
			continue
		}
		thirdPartyUnsigned = append(thirdPartyUnsigned, v.Name)
	}
	switch {
	case len(views) == 0:
		c.Status = "warn"
		c.Message = "no marketplaces registered"
	case len(officialUnverified) > 0:
		c.Status = "fail"
		c.Message = "official marketplace metadata is not signed or does not verify: " +
			strings.Join(officialUnverified, ", ")
	case len(thirdPartyInvalid) > 0:
		c.Status = "fail"
		c.Message = "third-party source has a signature that does not verify: " +
			strings.Join(thirdPartyInvalid, ", ")
	case len(thirdPartyUnsigned) > 0:
		c.Status = "warn"
		c.Message = "unsigned third-party sources: " + strings.Join(thirdPartyUnsigned, ", ")
	default:
		c.Status = "ok"
		c.Message = fmt.Sprintf("%d source(s) verified against the pinned key", len(views))
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
	for _, tool := range managedBinaries {
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
