package app

import (
	"context"
	"path/filepath"
	"runtime"

	"github.com/Obedience-Corp/festival-installer/internal/artifacts"
	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/hosts/shared"
	"github.com/Obedience-Corp/festival-installer/internal/installer"
	"github.com/Obedience-Corp/festival-installer/internal/source"
	"github.com/Obedience-Corp/festival-installer/internal/state"
)

// InstallOptions configure a suite or plugin install.
type InstallOptions struct {
	Channel string
	// Source overrides the configured marketplace. Updates use the receipt's
	// source here so resolution and activation remain provenance-consistent.
	Source   string
	Verify   source.VerifyOptions
	Progress ProgressFunc
}

// Keep the bootstrap behind the app boundary so every install entry point
// gets the same first-run behavior. The variable also lets app tests exercise
// the empty-home path without cloning the production marketplace.
var ensureOfficialSeed = source.EnsureOfficialSeed

// InstallFestival installs the camp+fest suite bundle.
func InstallFestival(ctx context.Context, opts InstallOptions) (InstallResult, error) {
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
	vo := opts.Verify
	progress := opts.Progress

	report(progress, ProgressEvent{Stage: "resolve", Package: FestivalPackageID, Percent: 0.05, Message: "resolving festival suite"})

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
		if err := ensureOfficialSeed(ctx); err != nil {
			return InstallResult{}, errpkg.Wrap("E_MARKETPLACE_SEED", err, "ensure official marketplace")
		}
	}

	manifest, err := source.LoadPackageManifest(ctx, sourceName, FestivalPackageID, vo)
	if err != nil {
		return InstallResult{}, err
	}

	rel, err := installer.SelectRelease(manifest, channel)
	if err != nil {
		return InstallResult{}, err
	}
	art, err := installer.SelectArtifact(rel, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return InstallResult{}, err
	}

	binDir, err := state.BinDir(ctx)
	if err != nil {
		return InstallResult{}, err
	}

	db, err := state.OpenDB(ctx, home)
	if err != nil {
		return InstallResult{}, err
	}
	defer func() { _ = db.Close(ctx) }()

	tx, err := installer.Begin(ctx, db.Raw(), home)
	if err != nil {
		return InstallResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	stageDir := tx.StagingDir()
	report(progress, ProgressEvent{Stage: "download", Package: FestivalPackageID, Percent: 0.25, Message: "downloading " + rel.Version})
	archivePath, err := artifacts.NewDownloader().Download(ctx, art.URL, stageDir)
	if err != nil {
		return InstallResult{}, err
	}
	report(progress, ProgressEvent{Stage: "verify", Package: FestivalPackageID, Percent: 0.5, Message: "verifying checksum"})
	if err := artifacts.VerifySHA256(ctx, archivePath, art.Sha256); err != nil {
		return InstallResult{}, err
	}
	extractDir := filepath.Join(stageDir, "extracted")
	report(progress, ProgressEvent{Stage: "extract", Package: FestivalPackageID, Percent: 0.7, Message: "extracting archive"})
	if err := artifacts.ExtractTarGz(ctx, archivePath, extractDir); err != nil {
		return InstallResult{}, err
	}

	report(progress, ProgressEvent{Stage: "activate", Package: FestivalPackageID, Percent: 0.85, Message: "lighting camp & fest"})
	var files []string
	for _, entry := range rel.Install.Entries {
		if entry.Kind != "binary" {
			continue
		}
		srcName := entry.Source
		destName := entry.ExecutableName
		if destName == "" {
			destName = srcName
		}
		if err := shared.ValidateSegment(destName); err != nil {
			return InstallResult{}, err
		}
		stagedBin, err := artifacts.SafeJoin(extractDir, filepath.FromSlash(srcName))
		if err != nil {
			return InstallResult{}, err
		}
		hash, err := artifacts.SHA256(ctx, stagedBin)
		if err != nil {
			return InstallResult{}, err
		}
		dst := filepath.Join(binDir, destName)
		if err := tx.Stage(ctx, installer.StagedFile{StagedPath: stagedBin, DestPath: dst, Sha256: hash, Mode: 0o755}); err != nil {
			return InstallResult{}, err
		}
		files = append(files, dst)
	}

	if _, err := tx.Commit(ctx, installer.ReceiptInfo{
		PackageID:   FestivalPackageID,
		Version:     rel.Version,
		Channel:     channel,
		Source:      sourceName,
		ManifestURL: art.URL,
	}); err != nil {
		return InstallResult{}, err
	}

	report(progress, ProgressEvent{Stage: "done", Package: FestivalPackageID, Percent: 1, Message: "festival suite ready"})
	return InstallResult{
		Package: FestivalPackageID,
		Version: rel.Version,
		Channel: channel,
		Source:  sourceName,
		Files:   files,
	}, nil
}

// InstallTarget installs festival suite or a camp-*/fest-* plugin.
func InstallTarget(ctx context.Context, target string, opts InstallOptions) (InstallResult, error) {
	if host, name, ok := PluginHost(target); ok {
		return InstallPlugin(ctx, host, name, opts)
	}
	switch target {
	case "festival", "camp", "fest":
		return InstallFestival(ctx, opts)
	default:
		return InstallResult{}, errpkg.New("E_INSTALL_TARGET", "unknown install target "+target+" (expected festival, camp, fest, or a camp-*/fest-* plugin)")
	}
}
