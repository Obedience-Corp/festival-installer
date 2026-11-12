package app

import (
	"context"
	"io"
	"runtime"

	"github.com/Obedience-Corp/festival-installer/internal/installer"
	"github.com/Obedience-Corp/festival-installer/internal/source"
)

// StatusApp reports Festival App. States (D009): no receipt -> installed false
// and every other field null; receipt and copy present -> installed true;
// receipt but the copy is gone -> installed false with a note.
type StatusApp struct {
	Installed       bool    `json:"installed"`
	Version         *string `json:"version"`
	ReceiptVersion  *string `json:"receipt_version"`
	Format          *string `json:"format"`
	Location        *string `json:"location"`
	Adopted         *bool   `json:"adopted"`
	UpdatedByApp    *bool   `json:"updated_by_app"`
	Latest          *string `json:"latest"`
	UpdateAvailable *bool   `json:"update_available"`
	Note            *string `json:"note"`
}

const removedOutsideNote = "removed outside festival"

func strPtr(s string) *string { return &s }

// cachedAppLatest reads the newest stable release from the already-cloned
// catalog. It never refreshes: Festival App runs status on every launch.
func cachedAppLatest(ctx context.Context, rec *appReceipt) appLatest {
	m, err := loadPackageManifest(ctx, rec.Source, AppPackageID, source.DefaultVerifyOptions(io.Discard, false))
	if err != nil {
		return appLatest{}
	}
	rel, err := installer.SelectRelease(m, appChannel)
	if err != nil {
		return appLatest{}
	}
	art, err := installer.SelectAppArtifact(rel, runtime.GOOS, runtime.GOARCH, rec.Format)
	if err != nil {
		return appLatest{Version: rel.Version}
	}
	return appLatest{Version: rel.Version, SHA: art.Sha256}
}

// resolveStatusApp never fails and never creates the installer home.
func resolveStatusApp(ctx context.Context) StatusApp {
	rec, err := readAppReceipt(ctx)
	if err != nil || rec == nil {
		return StatusApp{}
	}
	out := StatusApp{ReceiptVersion: strPtr(rec.Version), Format: strPtr(rec.Format), Location: strPtr(rec.Location), Adopted: &rec.Adopted}
	latest := cachedAppLatest(ctx, rec)
	if latest.Version != "" {
		out.Latest = strPtr(latest.Version)
	}
	platform, err := hostAppPlatform(ctx)
	if err != nil {
		out.Installed = true
		return out
	}
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	cp, err := platform.onDisk(probeCtx, *rec, latest)
	if err == nil && !cp.Found {
		out.Note = strPtr(removedOutsideNote)
		out.Latest, out.UpdateAvailable = nil, nil
		return out
	}
	out.Installed = true
	if err != nil {
		return out
	}
	out.UpdatedByApp = &cp.UpdatedByApp
	if cp.Version != "" {
		out.Version = strPtr(cp.Version)
		if latest.Version != "" {
			avail := installer.VersionLess(cp.Version, latest.Version)
			out.UpdateAvailable = &avail
		}
	}
	return out
}
