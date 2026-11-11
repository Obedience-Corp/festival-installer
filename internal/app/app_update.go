package app

import (
	"context"
	"strings"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/installer"
	"github.com/Obedience-Corp/festival-installer/internal/source"
)

// UpdateApp updates an installed Festival App in its own format (D002).
func UpdateApp(ctx context.Context, opts AppInstallOptions) (AppResult, error) {
	opts.UpdateOnly, opts.Format = true, ""
	return InstallApp(ctx, opts)
}

// updateResult maps an AppResult to the shared UpdateResult used by update --all
// and the TUI.
func (r AppResult) updateResult() UpdateResult {
	res := UpdateResult{Package: AppPackageID, Version: r.Version}
	switch {
	case r.Changed:
		res.Action, res.From = "upgraded", r.PreviousVersion
	case r.Version == "":
		res.Action = "absent"
	default:
		res.Action = "current"
	}
	return res
}

// appPlatformFor is the host platform; tests replace it.
var appPlatformFor = hostAppPlatform

const selfUpdatedDetail = "Festival App updated itself; festival leaves it in place"

// checkAppUpdate reports whether a newer stable Festival App exists. The
// installed version is the one on disk (D006), never the receipt's.
func checkAppUpdate(ctx context.Context, entry ListEntry, vo source.VerifyOptions) (PackageUpdate, error) {
	p := PackageUpdate{Package: entry.PackageID, Installed: entry.Version, Channel: "stable", Source: entry.Source}
	rec, err := readAppReceipt(ctx)
	if err != nil {
		return p, err
	}
	if rec == nil {
		return p, errpkg.New("E_UPDATE_RECEIPT", "installation changed; refresh updates")
	}
	rel, err := resolveAppRelease(ctx, entry.Source, rec.Format, source.OptionsForSource(entry.Source, vo))
	if err != nil {
		return p, err
	}
	p.Latest = rel.Version
	platform, err := appPlatformFor(ctx)
	if err != nil {
		return p, err
	}
	cp, err := platform.onDisk(ctx, *rec, appLatest{Version: rel.Version, SHA: rel.Artifact.Sha256})
	if err != nil {
		return p, err
	}
	if !cp.Found {
		return p, errpkg.New("E_UPDATE_RECEIPT", "Festival App is missing from "+rec.Location+"; run festival install app")
	}
	p.Status = UpdateCurrent
	if cp.Version == "" {
		p.Detail = selfUpdatedDetail
		return p, nil
	}
	p.Installed = cp.Version
	if cp.UpdatedByApp {
		p.Detail = "updated by the App (receipt " + rec.Version + ")"
	}
	if installer.VersionLess(cp.Version, rel.Version) {
		if err := checkUpdateArtifact(ctx, rel.Artifact.URL); err != nil {
			return p, err
		}
		p.Status = UpdateAvailable
	}
	return p, nil
}

func joinNotes(notes []string) string { return strings.Join(notes, "\n") }
