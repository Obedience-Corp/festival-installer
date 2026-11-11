package app

import (
	"context"
	"database/sql"
	"io"
	"os"
	"runtime"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/installer"
	"github.com/Obedience-Corp/festival-installer/internal/source"
	"github.com/Obedience-Corp/festival-installer/internal/state"
)

// AppInstallOptions configures an install or update of Festival App.
type AppInstallOptions struct {
	Format     string // "" or, on Linux only, "appimage" | "deb" (rules in 03_platforms/04)
	Source     string // "" means the configured default, applied by resolveAppRelease
	Verify     source.VerifyOptions
	Progress   ProgressFunc // download and stage events (TUI bar, or NewTextProgress on stderr)
	Output     io.Writer    // system installer output (apt-get); nil means io.Discard
	UpdateOnly bool         // 04_lifecycle/02: requires a receipt, never adopts
}

// AppResult is the data of the install, update and uninstall envelopes (D009).
type AppResult struct {
	Package         string   `json:"package"`
	Version         string   `json:"version"`
	PreviousVersion string   `json:"previous_version"`
	Format          string   `json:"format"`
	Location        string   `json:"location"`
	Adopted         bool     `json:"adopted"`
	Changed         bool     `json:"-"` // bytes were placed or removed
	Notes           []string `json:"-"` // human notes; the CLI also puts them in envelope warnings
}

type appReceipt struct { // in-memory D006 receipt; 04_lifecycle/01 maps it to rows and metadata keys
	Version, Source, Channel, ManifestURL      string
	Format, Location, BundleID, ArtifactSHA256 string
	Adopted                                    bool
	Executable                                 string
	Mode                                       os.FileMode
}

type appReceiptStore interface {
	Read(ctx context.Context) (*appReceipt, error) // nil, nil when absent
	Write(ctx context.Context, r appReceipt) error
}

type appCopy struct {
	Found, FromReceipt, UpdatedByApp bool
	Location, Version                string // Version "" when unknown
	OtherAccount                     string // macOS: another account's copy festival left alone
}

type appPlacement struct {
	Location, Executable string
	Mode                 os.FileMode
	Notes                []string
}

type appPlaceRequest struct {
	Release      appRelease
	Artifact     verifiedAppArtifact
	Copy         appCopy
	Output       io.Writer
	WriteReceipt func(ctx context.Context, db *sql.DB, p appPlacement) error // run inside AppTxn.Commit
}

// appTxn is the part of *installer.AppTxn the platforms use; tests may wrap it.
type appTxn interface {
	Prepare(ctx context.Context, p installer.AppPlacement) (string, error)
	MarkStaged(ctx context.Context) error
	Swap(ctx context.Context) error
	Commit(ctx context.Context, writeReceipt func(context.Context, *sql.DB) error) (string, error)
	RecordExternal(ctx context.Context, p installer.AppPlacement) error
	Close(ctx context.Context) error
}

type appPlatform interface { // one implementation per format family
	format(ctx context.Context, requested string, existing *appReceipt) (string, error)
	discover(ctx context.Context, format string, existing *appReceipt, rel appRelease) (appCopy, error)
	target(format string, cp appCopy) (string, error) // where it lives or will live; used for the run scope
	place(ctx context.Context, txn appTxn, req appPlaceRequest) (appPlacement, error)
	// onDisk reports the receipt's copy as it is now (status and update); no discovery.
	onDisk(ctx context.Context, rec appReceipt, latest appLatest) (appCopy, error)
}

// appLatest is the newest stable release from the cached catalog; zero when unknown.
type appLatest struct{ Version, SHA string }

type appInstaller struct {
	goos          string
	euid          func() int
	platform      appPlatform
	resolve       func(ctx context.Context, sourceName, format string, vo source.VerifyOptions) (appRelease, error)
	download      func(ctx context.Context, home string, rel appRelease, opts appDownloadOptions) (verifiedAppArtifact, error)
	begin         func(ctx context.Context, db *sql.DB, home string) (appTxn, error)
	refuseRunning func(ctx context.Context, scope appRunScope) error
	receipts      func(db *sql.DB) appReceiptStore // production store comes from 04_lifecycle/01
}

type appAction int

const (
	appActionPlace appAction = iota
	appActionLeave
	appActionCurrent
)

// decideAppAction says whether to place the latest release, leave a copy that
// is ahead of the catalog, or report the copy as current.
func decideAppAction(hasReceipt bool, cp appCopy, latest string) appAction {
	switch {
	case !cp.Found:
		return appActionPlace
	case cp.UpdatedByApp && cp.Version == "", cp.Version != "" && installer.VersionLess(latest, cp.Version):
		return appActionLeave
	case hasReceipt && cp.Version == latest:
		return appActionCurrent
	}
	return appActionPlace
}

// downloadWithAppKey downloads with the pinned updater key (swappable only by tests).
func downloadWithAppKey(ctx context.Context, home string, rel appRelease, opts appDownloadOptions) (verifiedAppArtifact, error) {
	key := appUpdaterKey()
	opts.UpdaterKey = &key
	return downloadAppArtifact(ctx, home, rel, opts)
}

func beginAppTxn(ctx context.Context, db *sql.DB, home string) (appTxn, error) {
	tx, err := installer.BeginApp(ctx, db, home)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

// newAppInstaller wires the production seams for this host. receipts is the
// receipt store factory (04_lifecycle/01 supplies it).
func newAppInstaller(ctx context.Context, receipts func(db *sql.DB) appReceiptStore) (appInstaller, error) {
	platform, err := hostAppPlatform(ctx)
	if err != nil {
		return appInstaller{}, err
	}
	return appInstaller{
		goos: runtime.GOOS, euid: os.Geteuid, platform: platform,
		resolve: resolveAppRelease, download: downloadWithAppKey, begin: beginAppTxn,
		refuseRunning: refuseRunningApp, receipts: receipts,
	}, nil
}

func (ai appInstaller) install(ctx context.Context, opts AppInstallOptions) (AppResult, error) {
	if err := ctx.Err(); err != nil {
		return AppResult{}, errpkg.Wrap("E_INSTALL_CTX", err, "context cancelled")
	}
	if ai.goos != "windows" && ai.euid() == 0 {
		return AppResult{}, errpkg.New("E_APP_ELEVATION", "run festival install app as your normal user without sudo; festival asks for a password only when the system needs one")
	}
	home, err := state.Home(ctx)
	if err != nil {
		return AppResult{}, err
	}
	var db *state.DB
	if opts.UpdateOnly {
		var ok bool
		if db, ok, err = state.OpenDBIfExists(ctx, home); err != nil {
			return AppResult{}, err
		} else if !ok {
			return AppResult{Package: AppPackageID, Notes: []string{notInstalledNote}}, nil
		}
	} else {
		if err := state.EnsureHome(ctx, 0o700); err != nil {
			return AppResult{}, err
		}
		if db, err = state.OpenDB(ctx, home); err != nil {
			return AppResult{}, err
		}
	}
	defer func() { _ = db.Close(ctx) }()
	existing, err := ai.receipts(db.Raw()).Read(ctx)
	if err != nil {
		return AppResult{}, errpkg.Wrap("E_APP_RECEIPT", err, "read the Festival App receipt")
	}
	if opts.UpdateOnly && existing == nil {
		return AppResult{Package: AppPackageID, Notes: []string{notInstalledNote}}, nil
	}

	report(opts.Progress, ProgressEvent{Stage: "resolve", Package: AppPackageID, Percent: 0.05, Message: "resolving Festival App"})
	format, err := ai.platform.format(ctx, opts.Format, existing)
	if err != nil {
		return AppResult{}, err
	}
	rel, err := ai.resolve(ctx, opts.Source, format, opts.Verify)
	if err != nil {
		return AppResult{}, err
	}
	cp, err := ai.platform.discover(ctx, format, existing, rel)
	if err != nil {
		return AppResult{}, err
	}
	if res, done := ai.skipResult(opts, existing, cp, format, rel); done {
		return res, nil
	}

	loc := cp.Location
	if loc == "" {
		if loc, err = ai.platform.target(format, cp); err != nil {
			return AppResult{}, err
		}
	}
	scope, err := appRunScopeFor(format, loc)
	if err != nil {
		return AppResult{}, err
	}
	if err := ai.refuseRunning(ctx, scope); err != nil {
		return AppResult{}, err
	}
	art, err := ai.download(ctx, home, rel, appDownloadOptions{Progress: opts.Progress})
	if err != nil {
		return AppResult{}, err
	}
	defer art.Cleanup()

	txn, err := ai.begin(ctx, db.Raw(), home)
	if err != nil {
		return AppResult{}, err
	}
	defer func() { _ = txn.Close(ctx) }()
	again, err := ai.platform.discover(ctx, format, existing, rel)
	if err != nil {
		return AppResult{}, err
	}
	if again.Found != cp.Found || again.Location != cp.Location || again.Version != cp.Version {
		cp = again
		if res, done := ai.skipResult(opts, existing, cp, format, rel); done {
			return res, nil
		}
	}
	if err := ai.refuseRunning(ctx, scope); err != nil {
		return AppResult{}, err
	}

	report(opts.Progress, ProgressEvent{Stage: "activate", Package: AppPackageID, Percent: -1, Message: "installing Festival App"})
	adopted := cp.Found && !cp.FromReceipt
	out := opts.Output
	if out == nil {
		out = io.Discard
	}
	placement, err := ai.platform.place(ctx, txn, appPlaceRequest{
		Release: rel, Artifact: art, Copy: cp, Output: out,
		WriteReceipt: func(ctx context.Context, db *sql.DB, p appPlacement) error {
			return ai.receipts(db).Write(ctx, appReceipt{
				Version: rel.Version, Source: rel.Source, Channel: rel.Channel, ManifestURL: rel.Artifact.URL,
				Format: format, Location: p.Location, BundleID: rel.BundleID, ArtifactSHA256: rel.Artifact.Sha256,
				Adopted: adopted, Executable: p.Executable, Mode: p.Mode,
			})
		},
	})
	if err != nil {
		return AppResult{}, err
	}
	notes := placement.Notes
	if adopted {
		notes = append([]string{"Replaced the existing Festival App at " + placement.Location}, notes...)
	}
	report(opts.Progress, ProgressEvent{Stage: "done", Package: AppPackageID, Percent: 1, Message: "Festival App " + rel.Version + " installed"})
	return AppResult{
		Package: AppPackageID, Version: rel.Version, PreviousVersion: cp.Version, Format: format,
		Location: placement.Location, Adopted: adopted, Changed: true, Notes: notes,
	}, nil
}

const notInstalledNote = "Festival App is not installed through festival; run festival install app"

// skipResult returns the no-op result when nothing should be placed.
func (ai appInstaller) skipResult(opts AppInstallOptions, existing *appReceipt, cp appCopy, format string, rel appRelease) (AppResult, bool) {
	if opts.UpdateOnly && (!cp.Found || !cp.FromReceipt) {
		res := AppResult{Package: AppPackageID}
		if existing != nil {
			res.PreviousVersion, res.Format, res.Location, res.Adopted = existing.Version, existing.Format, existing.Location, existing.Adopted
			res.Notes = []string{"Festival App is missing from " + existing.Location + "; run festival install app"}
		}
		return res, true
	}
	var note string
	switch decideAppAction(existing != nil, cp, rel.Version) {
	case appActionLeave:
		note = "Festival App " + versionOrUnknown(cp.Version) + " on disk is ahead of the latest release " + rel.Version + "; left it untouched."
	case appActionCurrent:
		note = "Festival App " + cp.Version + " is already the latest release."
	default:
		return AppResult{}, false
	}
	return AppResult{
		Package: AppPackageID, Version: cp.Version, PreviousVersion: cp.Version, Format: format, Location: cp.Location,
		Adopted: existing != nil && existing.Adopted, Notes: []string{note},
	}, true
}

func versionOrUnknown(v string) string {
	if v == "" {
		return "(updated by the App)"
	}
	return v
}

// Every platform implements appPlatform on every GOOS, so the platform code
// that only one host's hostAppPlatform constructs still counts as used.
var (
	_ appPlatform = bundlePlatform{}
	_ appPlatform = nsisPlatform{}
	_ appPlatform = linuxPlatform{}
)
