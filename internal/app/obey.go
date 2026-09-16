package app

import (
	"context"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Obedience-Corp/festival-installer/internal/artifacts"
	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/hosts/shared"
	"github.com/Obedience-Corp/festival-installer/internal/installer"
	"github.com/Obedience-Corp/festival-installer/internal/metadata"
	"github.com/Obedience-Corp/festival-installer/internal/release"
	"github.com/Obedience-Corp/festival-installer/internal/source"
	"github.com/Obedience-Corp/festival-installer/internal/state"
)

// ObeyPackageID is the marketplace ID for the obey daemon product.
const ObeyPackageID = "obedience-corp/obey"

// obeyBinary is the daemon binary, the one the service verbs run through.
const obeyBinary = "obey"

// obDeveloperBinary is the second executable the obey product places.
const obDeveloperBinary = "ob"

// productResolved is one product release ready to place: where the archive is,
// what it hashes to, and which executables it carries.
type productResolved struct {
	version  string
	url      string
	sha256   string
	archive  bool
	binaries []string
}

// findProductPackage refreshes sourceName and returns the marketplace entry for
// packageID. Refreshing first is what keeps update from reporting "current"
// against a clone that predates the release. Only sourceName is loaded, so an
// unrelated marketplace that fails to parse cannot block this product the way
// it would if every registered source had to load first.
func findProductPackage(ctx context.Context, sourceName, packageID string, vo source.VerifyOptions) (source.BrowsePackage, error) {
	views, err := refreshMarketplacesForLoad(ctx, sourceName, vo)
	if err != nil {
		return source.BrowsePackage{}, errpkg.Wrap("E_MARKETPLACE_REFRESH", err, "refresh marketplace "+sourceName)
	}
	if len(views) != 1 {
		return source.BrowsePackage{}, errpkg.New("E_MARKETPLACE_REFRESH", "refresh marketplace "+sourceName+" returned no result")
	}
	if views[0].Err != "" {
		return source.BrowsePackage{}, errpkg.New("E_MARKETPLACE_REFRESH", "refresh marketplace "+sourceName+": "+views[0].Err)
	}
	pkgs, err := source.SourcePackages(ctx, sourceName, vo)
	if err != nil {
		return source.BrowsePackage{}, err
	}
	for _, bp := range pkgs {
		if bp.Package.ID == packageID {
			return bp, nil
		}
	}
	return source.BrowsePackage{}, errpkg.New("E_PACKAGE_NOT_FOUND",
		"no package "+packageID+" in marketplace "+sourceName)
}

// resolveProduct turns a marketplace entry into a placeable release. A
// release_source entry resolves through git tags and a checksums file; a
// manifest_path entry resolves through the static per-platform artifact list.
// Everything the placement depends on is decided here, before any bytes move,
// so a release this path cannot install is refused without a download.
func resolveProduct(ctx context.Context, bp source.BrowsePackage, channel string, vo source.VerifyOptions) (productResolved, error) {
	r, err := resolveProductRelease(ctx, bp, channel, vo)
	if err != nil {
		return productResolved{}, err
	}
	if err := requireArchive(bp.Package.ID, r); err != nil {
		return productResolved{}, err
	}
	return r, nil
}

func resolveProductRelease(ctx context.Context, bp source.BrowsePackage, channel string, vo source.VerifyOptions) (productResolved, error) {
	if rs := bp.Package.ReleaseSource; rs != nil {
		if err := gitReleaseConsentGate(bp, vo); err != nil {
			return productResolved{}, err
		}
		resolved, err := release.NewResolver().Resolve(ctx, *rs, channel, runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return productResolved{}, err
		}
		names, err := declaredBinaries(rs, bp.Package.ID)
		if err != nil {
			return productResolved{}, err
		}
		return productResolved{
			version:  resolved.Version,
			url:      resolved.URL,
			sha256:   resolved.Sha256,
			archive:  resolved.IsArchive,
			binaries: names,
		}, nil
	}

	manifest, err := loadPackageManifest(ctx, bp.Source, bp.Package.ID, vo)
	if err != nil {
		return productResolved{}, err
	}
	rel, err := installer.SelectRelease(manifest, channel)
	if err != nil {
		return productResolved{}, err
	}
	art, err := installer.SelectArtifact(rel, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return productResolved{}, err
	}
	names, err := manifestBinaries(rel, bp.Package.ID)
	if err != nil {
		return productResolved{}, err
	}
	return productResolved{
		version:  rel.Version,
		url:      art.URL,
		sha256:   art.Sha256,
		archive:  isArchiveArtifact(art.Kind),
		binaries: names,
	}, nil
}

// manifestBinaries reads the executables a manifest_path product places. The
// product path places binaries and nothing else, so a skill bundle or an
// extension entry is refused rather than skipped: dropping it would leave a
// receipt claiming a complete install of a release that was only half placed.
func manifestBinaries(rel metadata.Release, packageID string) ([]string, error) {
	var names []string
	for _, e := range rel.Install.Entries {
		if e.Kind != "binary" {
			return nil, errpkg.New("E_PRODUCT_UNSUPPORTED_ENTRY",
				packageID+" release "+rel.Version+" declares a "+entryKindLabel(e)+
					" install entry; the product install path places binaries only and will not "+
					"write a receipt that claims the rest was installed")
		}
		names = append(names, entryExecutableName(e))
	}
	if len(names) == 0 {
		return nil, errpkg.New("E_PRODUCT_NO_BINARIES",
			"no binary install entries in "+packageID+" release "+rel.Version)
	}
	return names, nil
}

// entryKindLabel names a kind for an error message, including the unset one a
// malformed manifest can carry.
func entryKindLabel(e metadata.InstallEntry) string {
	if e.Kind == "" {
		return "kindless"
	}
	return e.Kind
}

// requireArchive refuses a release whose artifact is a single file. placeProduct
// extracts named executables out of an archive, so a bare binary has nothing to
// extract from regardless of how many binaries the entry declares. The refusal
// reads off the resolved artifact name, and it runs before the download.
func requireArchive(packageID string, r productResolved) error {
	if r.archive {
		return nil
	}
	return errpkg.New("E_PRODUCT_NOT_ARCHIVE",
		packageID+" release "+r.version+" publishes a bare binary ("+path.Base(r.url)+
			"); this install extracts "+strings.Join(r.binaries, " and ")+
			" by name and needs an archive")
}

// declaredBinaries reads the executables a release_source product places:
// Binaries when set, then the singular Binary. A product that declares neither
// is a marketplace bug, not a guess to make, so nothing is inferred from the
// package id.
func declaredBinaries(rs *release.Source, packageID string) ([]string, error) {
	if len(rs.Binaries) > 0 {
		return rs.Binaries, nil
	}
	if rs.Binary != "" {
		return []string{rs.Binary}, nil
	}
	return nil, errpkg.New("E_PRODUCT_NO_BINARIES",
		"release_source for "+packageID+" declares no binaries")
}

// InstallObey installs the obey product: every binary its marketplace entry
// declares, staged into the managed bin dir under one receipt.
//
// It deliberately skips the package-channel refusal the suite install runs
// (internal/app/install.go). That guard keeps the hub from planting a second
// camp/fest/festival over an AUR, Homebrew or npm suite. No package manager
// ships obey, so refusing here would leave a Homebrew user with no daemon at
// all, which is why E_INSTALL_PACKAGE_CHANNEL is unreachable for this target.
func InstallObey(ctx context.Context, opts InstallOptions) (InstallResult, error) {
	if err := ctx.Err(); err != nil {
		return InstallResult{}, errpkg.Wrap("E_INSTALL_CTX", err, "context cancelled")
	}
	channel := opts.Channel
	if channel == "" {
		channel = "stable"
	}
	if err := ValidateChannel(channel); err != nil {
		return InstallResult{}, err
	}

	progress := opts.Progress
	report(progress, ProgressEvent{Stage: "resolve", Package: ObeyPackageID, Percent: 0.05, Message: "resolving obey"})

	home, err := state.Home(ctx)
	if err != nil {
		return InstallResult{}, err
	}
	cfg, err := state.LoadConfig(ctx, home)
	if err != nil {
		return InstallResult{}, err
	}
	sourceName := opts.Source
	if sourceName == "" {
		sourceName = cfg.Marketplaces.Default
	}
	if sourceName == state.OfficialSeedKey {
		if err := ensureOfficialSeed(ctx, opts.Verify); err != nil {
			coded := errpkg.Wrap("E_MARKETPLACE_SEED", err, "ensure official marketplace")
			return InstallResult{}, &MarketplaceSeedProblem{Err: coded, Fatal: true}
		}
	}

	bp, err := findProductPackage(ctx, sourceName, ObeyPackageID, opts.Verify)
	if err != nil {
		return InstallResult{}, err
	}
	resolved, err := resolveProduct(ctx, bp, channel, opts.Verify)
	if err != nil {
		return InstallResult{}, err
	}

	// Asked before the binaries change, because that is the only moment the
	// answer is knowable: obey service install writes the unit and leaves a
	// live daemon alone, so a daemon that was up keeps running the previous
	// image until a restart swaps it.
	daemon := obeyDaemonState(ctx)

	files, err := placeProduct(ctx, home, ObeyPackageID, sourceName, channel, resolved, progress)
	if err != nil {
		return InstallResult{}, err
	}

	svc := obeyServiceSwap(ctx, daemon, opts.NoRestart)

	report(progress, ProgressEvent{Stage: "done", Package: ObeyPackageID, Percent: 1, Message: "obey ready"})
	return InstallResult{
		Package: ObeyPackageID,
		Version: resolved.version,
		Channel: channel,
		Source:  sourceName,
		Files:   files,
		Service: &svc,
	}, nil
}

// obeyServiceSwap drives the whole service step for the obey that was just
// staged: verify the contract, register the unit, then settle the process that
// is serving. An obey whose service family predates the contract gets no verb
// at all, because every sentence this step would print about that obey is
// wrong; the warning says so and names the command to run by hand.
func obeyServiceSwap(ctx context.Context, daemon daemonState, noRestart bool) ServiceResult {
	if contract := obeyServiceContract(ctx); !contract.Supported {
		return ServiceResult{Unsupported: true, ContractReason: contract.Reason}
	}
	svc := serviceStep(ctx, serviceVerbInstall)
	return finishServiceSwap(ctx, svc, daemon, noRestart)
}

// finishServiceSwap completes the service step once the unit is registered.
// Installing the unit is enough on a machine with no daemon up: the supervisor
// starts it, and that is the one case that sets Started. A daemon that was
// already running keeps the replaced binary's image, so the restart is what
// makes the install true, and --no-restart turns that into a reported pending
// restart rather than a silent one. A daemon whose state could not be read is
// treated as one that may be running, because the cost of guessing wrong the
// other way is a live daemon left on a replaced binary under a payload saying
// it was started. A failed install step is left alone: there is no unit to
// restart through.
func finishServiceSwap(ctx context.Context, svc ServiceResult, daemon daemonState, noRestart bool) ServiceResult {
	if svc.Error != "" {
		return svc
	}
	if daemon == daemonStateStopped {
		svc.Started = svc.Installed
		return svc
	}
	svc.DaemonStateUnknown = daemon == daemonStateUnknown
	if noRestart {
		svc.Deferred = true
		return svc
	}
	restart := serviceStep(ctx, serviceVerbRestart)
	svc.Restarted = restart.Restarted
	svc.Error = restart.Error
	svc.TimedOut = restart.TimedOut
	return svc
}

// placeProduct downloads, verifies, extracts, and stages every declared binary
// under one transaction and one receipt. Every binary is staged before Commit,
// so a missing one leaves nothing behind.
func placeProduct(ctx context.Context, home, packageID, sourceName, channel string, r productResolved, progress ProgressFunc) ([]string, error) {
	binDir, err := state.BinDir(ctx)
	if err != nil {
		return nil, err
	}
	db, err := state.OpenDB(ctx, home)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close(ctx) }()

	tx, err := installer.Begin(ctx, db.Raw(), home)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	report(progress, ProgressEvent{Stage: "download", Package: packageID, Percent: 0.3, Message: "downloading " + r.version})
	staged, err := artifacts.NewDownloader().Download(ctx, r.url, tx.StagingDir())
	if err != nil {
		return nil, err
	}
	report(progress, ProgressEvent{Stage: "verify", Package: packageID, Percent: 0.55, Message: "verifying checksum"})
	if err := artifacts.VerifySHA256(ctx, staged, r.sha256); err != nil {
		return nil, err
	}

	extractDir := filepath.Join(tx.StagingDir(), "extracted")
	report(progress, ProgressEvent{Stage: "extract", Package: packageID, Percent: 0.7, Message: "extracting archive"})
	if err := artifacts.ExtractTarGz(ctx, staged, extractDir); err != nil {
		return nil, err
	}

	report(progress, ProgressEvent{Stage: "activate", Package: packageID, Percent: 0.9, Message: "activating " + packageID})
	var files []string
	for _, name := range r.binaries {
		if err := shared.ValidateSegment(name); err != nil {
			return nil, err
		}
		src, err := artifacts.SafeJoin(extractDir, name)
		if err != nil {
			return nil, err
		}
		hash, err := artifacts.SHA256(ctx, src)
		if err != nil {
			return nil, errpkg.Wrap("E_PRODUCT_BINARY_MISSING", err, "binary "+name+" not found in the "+packageID+" archive")
		}
		dst := filepath.Join(binDir, name)
		if err := tx.Stage(ctx, installer.StagedFile{StagedPath: src, DestPath: dst, Sha256: hash, Mode: 0o755}); err != nil {
			return nil, err
		}
		files = append(files, dst)
	}

	if _, err := tx.Commit(ctx, installer.ReceiptInfo{
		PackageID:   packageID,
		Version:     r.version,
		Channel:     channel,
		Source:      sourceName,
		ManifestURL: r.url,
	}); err != nil {
		return nil, err
	}
	return files, nil
}

// UpdateObey brings the installed obey product to channel-latest. It returns a
// human warning as the second value, matching UpdateFestival.
//
// The service step is the install step: refreshing the unit is idempotent and
// it starts a daemon that is not running, so an update on a machine where the
// daemon was stopped leaves it up on the new binary. Only a daemon that was
// already running needs the restart, because only that one keeps serving the
// replaced image.
func UpdateObey(ctx context.Context, opts UpdateOptions) (UpdateResult, string, error) {
	if err := ctx.Err(); err != nil {
		return UpdateResult{}, "", errpkg.Wrap("E_UPDATE_CTX", err, "context cancelled")
	}

	rec, found, err := readReceipt(ctx, ObeyPackageID)
	if err != nil {
		return UpdateResult{}, "", err
	}
	if !found {
		return unmanagedObey(ctx)
	}

	channel := rec.Channel
	if opts.ChannelOverride != "" {
		channel = opts.ChannelOverride
	}
	if channel == "" {
		channel = "stable"
	}
	if err := ValidateChannel(channel); err != nil {
		return UpdateResult{}, "", err
	}

	installedVersion := rec.Version
	warning := ""
	if live, derr := detectObeyVersion(ctx); derr == nil && LooksLikeVersion(live) && live != installedVersion {
		warning = "receipt reports " + installedVersion + " but the managed obey reports " + live + "; comparing against the live version"
		installedVersion = live
	}

	report(opts.Progress, ProgressEvent{Stage: "resolve", Package: ObeyPackageID, Percent: 0.1, Message: "checking for updates"})

	bp, err := findProductPackage(ctx, rec.Source, ObeyPackageID, opts.Verify)
	if err != nil {
		return UpdateResult{}, warning, err
	}
	resolved, err := resolveProduct(ctx, bp, channel, opts.Verify)
	if err != nil {
		return UpdateResult{}, warning, err
	}

	if !installer.VersionLess(installedVersion, resolved.version) {
		return UpdateResult{Package: ObeyPackageID, Action: "current", Version: installedVersion}, warning, nil
	}

	home, err := state.Home(ctx)
	if err != nil {
		return UpdateResult{}, warning, err
	}

	daemon := obeyDaemonState(ctx)

	if _, err := placeProduct(ctx, home, ObeyPackageID, rec.Source, channel, resolved, opts.Progress); err != nil {
		return UpdateResult{}, warning, err
	}

	svc := obeyServiceSwap(ctx, daemon, opts.NoRestart)
	// The started sentence is an update's alone. A fresh install that brings
	// the daemon up is doing what the caller asked; an update that finds it
	// stopped and leaves it running has changed something the caller did not
	// ask about, and this sentence is the only place that shows.
	warning = appendWarning(warning, serviceNote(&svc, resolved.version, true))
	return UpdateResult{
		Package: ObeyPackageID,
		Action:  "upgraded",
		Version: resolved.version,
		From:    installedVersion,
		Service: &svc,
	}, warning, nil
}

// unmanagedObey distinguishes "nothing is installed" from "an obey exists that
// festival does not own". The second case must never be replaced.
func unmanagedObey(ctx context.Context) (UpdateResult, string, error) {
	path, err := ResolveTool(ctx, obeyBinary)
	if err != nil {
		return UpdateResult{Package: ObeyPackageID, Action: "absent"},
			"obey is not installed; run `festival install obey`", nil
	}
	return UpdateResult{Package: ObeyPackageID, Action: "unmanaged"},
		"obey is installed but not managed by festival (found at " + path + "). " +
			"Refusing to modify an external install. Run `festival which obey --show-all` or `festival doctor` to inspect.", nil
}

// detectObeyVersion reads the managed obey's own version. It tries
// `obey version --short` first and `obey --version` second. The fallback is
// the live path, not a legacy one: obey registers no version subcommand at
// all, so it answers only the root flag, printing "obey version X.Y.Z". The
// first answer wins so a later obey that grows the subcommand is read the same
// way camp and fest are.
func detectObeyVersion(ctx context.Context) (string, error) {
	path, err := obeyServicePath(ctx)
	if err != nil {
		return "", err
	}
	return detectObeyVersionAt(ctx, path)
}

// detectObeyVersionAt is detectObeyVersion asked of an explicit binary, so the
// contract probe and its tests read the version of the obey they are grading
// rather than whichever one the managed bin dir holds.
func detectObeyVersionAt(ctx context.Context, path string) (string, error) {
	if out, err := exec.CommandContext(ctx, path, "version", "--short").Output(); err == nil { //nolint:gosec // path is the managed bin dir, args fixed
		if v := strings.TrimSpace(string(out)); v != "" {
			return v, nil
		}
	}
	out, err := exec.CommandContext(ctx, path, "--version").Output() //nolint:gosec // path is the managed bin dir, args fixed
	if err != nil {
		return "", errpkg.Wrap("E_VERSION_PROBE", err, "read obey version at "+path)
	}
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(out)), "obey version ")), nil
}
