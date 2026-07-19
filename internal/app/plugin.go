package app

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Obedience-Corp/obey-installer/internal/artifacts"
	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
	"github.com/Obedience-Corp/obey-installer/internal/hosts"
	"github.com/Obedience-Corp/obey-installer/internal/hosts/camp"
	"github.com/Obedience-Corp/obey-installer/internal/hosts/fest"
	"github.com/Obedience-Corp/obey-installer/internal/hosts/shared"
	"github.com/Obedience-Corp/obey-installer/internal/installer"
	"github.com/Obedience-Corp/obey-installer/internal/metadata"
	"github.com/Obedience-Corp/obey-installer/internal/release"
	"github.com/Obedience-Corp/obey-installer/internal/source"
	"github.com/Obedience-Corp/obey-installer/internal/state"
)

// PluginHost parses camp-*/fest-* selectors.
func PluginHost(selector string) (host, name string, ok bool) {
	switch {
	case strings.HasPrefix(selector, "camp-"):
		return "camp", strings.TrimPrefix(selector, "camp-"), true
	case strings.HasPrefix(selector, "fest-"):
		return "fest", strings.TrimPrefix(selector, "fest-"), true
	default:
		return "", "", false
	}
}

func chooseAdapter(host string) hosts.Host {
	if host == "camp" {
		return camp.New()
	}
	return fest.New()
}

// FindPlugin locates a plugin package by host and short name.
func FindPlugin(pkgs []source.BrowsePackage, host, name string) (source.BrowsePackage, error) {
	want := host + "-" + name
	var matches []source.BrowsePackage
	for _, bp := range pkgs {
		if bp.Package.Class != "plugin" {
			continue
		}
		id := bp.Package.ID
		seg := id
		if i := strings.LastIndex(id, "/"); i >= 0 {
			seg = id[i+1:]
		}
		if id == want || seg == want {
			matches = append(matches, bp)
		}
	}
	switch len(matches) {
	case 0:
		return source.BrowsePackage{}, errpkg.New("E_PLUGIN_NOT_FOUND", "no plugin "+want+" in registered marketplaces")
	case 1:
		return matches[0], nil
	default:
		return source.BrowsePackage{}, errpkg.New("E_PLUGIN_AMBIGUOUS", "plugin "+want+" found in multiple marketplaces")
	}
}

func selectPluginArtifact(rel metadata.Release, goos, goarch string) (metadata.Artifact, error) {
	for _, art := range rel.Artifacts {
		if art.OS == goos && art.Arch == goarch && isPluginArtifactKind(art.Kind) {
			return art, nil
		}
	}
	for _, art := range rel.Artifacts {
		if art.OS == goos && art.Arch == "all" && isPluginArtifactKind(art.Kind) {
			return art, nil
		}
	}
	return metadata.Artifact{}, errpkg.New("E_NO_ARTIFACT", "no plugin artifact for "+goos+"/"+goarch)
}

func isPluginArtifactKind(kind string) bool {
	switch kind {
	case "binary", "tar.gz", "suite-archive":
		return true
	default:
		return false
	}
}

func isArchiveArtifact(kind string) bool {
	return kind == "tar.gz" || kind == "suite-archive"
}

func locateArchiveBinary(extractDir, name string) (string, error) {
	if err := shared.ValidateSegment(name); err != nil {
		return "", err
	}
	p := filepath.Join(extractDir, name)
	fi, err := os.Stat(p)
	if err != nil {
		return "", errpkg.New("E_PLUGIN_BINARY_MISSING", "binary "+name+" not found in plugin archive")
	}
	if fi.IsDir() {
		return "", errpkg.New("E_PLUGIN_BINARY_MISSING", name+" in the plugin archive is a directory, not a binary")
	}
	return p, nil
}

type pluginSpec struct {
	packageID       string
	source          string
	version         string
	artifactURL     string
	sha256          string
	isArchive       bool
	binaryInArchive string
	execName        string
	target          metadata.RuntimeTarget
}

// SpecFromManifest builds a plugin install plan from a marketplace package manifest.
func SpecFromManifest(ctx context.Context, bp source.BrowsePackage, host, name, channel string, vo source.VerifyOptions) (pluginSpec, error) {
	manifest, err := source.LoadPackageManifest(ctx, bp.Source, bp.Package.ID, vo)
	if err != nil {
		return pluginSpec{}, err
	}
	rel, err := installer.SelectRelease(manifest, channel)
	if err != nil {
		return pluginSpec{}, err
	}
	art, err := selectPluginArtifact(rel, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return pluginSpec{}, err
	}
	entry, err := binaryEntry(rel, host+"-"+name)
	if err != nil {
		return pluginSpec{}, err
	}
	execName := entryExecutableName(entry)
	binIn := entry.Source
	if binIn == "" {
		binIn = execName
	}
	return pluginSpec{
		packageID:       bp.Package.ID,
		source:          bp.Source,
		version:         rel.Version,
		artifactURL:     art.URL,
		sha256:          art.Sha256,
		isArchive:       isArchiveArtifact(art.Kind),
		binaryInArchive: binIn,
		execName:        execName,
		target:          selectTarget(bp.Package.Targets, host),
	}, nil
}

// SpecFromGit builds a plugin install plan from a git release_source.
//
// VER-02: git-release plugins are an untrusted class (checksums are self-asserted
// over the same transport as the asset). They go through the same refuse-by-default
// / --allow-unverified policy as unsigned package manifests.
func SpecFromGit(ctx context.Context, bp source.BrowsePackage, host, name, channel string, vo source.VerifyOptions) (pluginSpec, error) {
	if err := metadata.EnforceUnverifiedPolicy(metadata.IngestOptions{
		Policy:          vo.Policy,
		AllowUnverified: vo.AllowUnverified,
		WarnWriter:      vo.WarnWriter,
		SourceLabel:     "git-release plugin " + bp.Source + "/" + bp.Package.ID,
	}); err != nil {
		return pluginSpec{}, err
	}
	rs := bp.Package.ReleaseSource
	resolved, err := release.NewResolver().Resolve(ctx, *rs, channel, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return pluginSpec{}, err
	}
	execName := host + "-" + name
	binIn := rs.Binary
	if binIn == "" {
		binIn = execName
	}
	return pluginSpec{
		packageID:       bp.Package.ID,
		source:          bp.Source,
		version:         resolved.Version,
		artifactURL:     resolved.URL,
		sha256:          resolved.Sha256,
		isArchive:       resolved.IsArchive,
		binaryInArchive: binIn,
		execName:        execName,
		target:          selectTarget(bp.Package.Targets, host),
	}, nil
}

func entryExecutableName(e metadata.InstallEntry) string {
	if e.ExecutableName != "" {
		return e.ExecutableName
	}
	return e.Source
}

func binaryEntry(rel metadata.Release, want string) (metadata.InstallEntry, error) {
	for _, e := range rel.Install.Entries {
		if e.Kind != "binary" {
			continue
		}
		if entryExecutableName(e) == want {
			return e, nil
		}
	}
	return metadata.InstallEntry{}, errpkg.New("E_PLUGIN_ENTRY", "no binary install-entry named "+want+" in the plugin manifest")
}

func selectTarget(targets []metadata.RuntimeTarget, host string) metadata.RuntimeTarget {
	for _, tgt := range targets {
		if tgt.Runtime == host || strings.HasPrefix(tgt.Runtime, host+"-") {
			return tgt
		}
	}
	if len(targets) > 0 {
		return targets[0]
	}
	return metadata.RuntimeTarget{}
}

// InstallPlugin installs a camp or fest plugin by short name.
func InstallPlugin(ctx context.Context, host, name string, opts InstallOptions) (InstallResult, error) {
	if err := ctx.Err(); err != nil {
		return InstallResult{}, errpkg.Wrap("E_INSTALL_CTX", err, "context cancelled")
	}
	channel := opts.Channel
	if channel == "" {
		channel = "stable"
	}
	progress := opts.Progress
	report(progress, ProgressEvent{Stage: "resolve", Package: host + "-" + name, Percent: 0.1, Message: "finding plugin"})

	pkgs, err := source.AllPackages(ctx)
	if err != nil {
		return InstallResult{}, err
	}
	bp, err := FindPlugin(pkgs, host, name)
	if err != nil {
		return InstallResult{}, err
	}

	var spec pluginSpec
	if bp.Package.ReleaseSource != nil {
		spec, err = SpecFromGit(ctx, bp, host, name, channel, opts.Verify)
	} else {
		spec, err = SpecFromManifest(ctx, bp, host, name, channel, opts.Verify)
	}
	if err != nil {
		return InstallResult{}, err
	}

	if err := chooseAdapter(host).ValidateCompatibility(ctx, spec.target); err != nil {
		return InstallResult{}, err
	}
	return activatePlugin(ctx, spec, channel, progress)
}

func activatePlugin(ctx context.Context, spec pluginSpec, channel string, progress ProgressFunc) (InstallResult, error) {
	if err := shared.ValidateSegment(spec.execName); err != nil {
		return InstallResult{}, err
	}
	home, err := state.Home(ctx)
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

	report(progress, ProgressEvent{Stage: "download", Package: spec.packageID, Percent: 0.3, Message: "downloading plugin"})
	staged, err := artifacts.NewDownloader().Download(ctx, spec.artifactURL, tx.StagingDir())
	if err != nil {
		return InstallResult{}, err
	}
	report(progress, ProgressEvent{Stage: "verify", Package: spec.packageID, Percent: 0.55, Message: "verifying checksum"})
	if err := artifacts.VerifySHA256(ctx, staged, spec.sha256); err != nil {
		return InstallResult{}, err
	}

	binaryPath := staged
	if spec.isArchive {
		extractDir := filepath.Join(tx.StagingDir(), "extracted")
		report(progress, ProgressEvent{Stage: "extract", Package: spec.packageID, Percent: 0.7, Message: "extracting plugin"})
		if err := artifacts.ExtractTarGz(ctx, staged, extractDir); err != nil {
			return InstallResult{}, err
		}
		inner, err := locateArchiveBinary(extractDir, spec.binaryInArchive)
		if err != nil {
			return InstallResult{}, err
		}
		binaryPath = inner
	}

	report(progress, ProgressEvent{Stage: "activate", Package: spec.packageID, Percent: 0.9, Message: "activating " + spec.execName})
	hash, err := artifacts.SHA256(ctx, binaryPath)
	if err != nil {
		return InstallResult{}, err
	}
	dst := filepath.Join(binDir, spec.execName)
	if err := tx.Stage(ctx, installer.StagedFile{StagedPath: binaryPath, DestPath: dst, Sha256: hash, Mode: 0o755}); err != nil {
		return InstallResult{}, err
	}
	if _, err := tx.Commit(ctx, installer.ReceiptInfo{
		PackageID:   spec.packageID,
		Version:     spec.version,
		Channel:     channel,
		Source:      spec.source,
		ManifestURL: spec.artifactURL,
	}); err != nil {
		return InstallResult{}, err
	}

	report(progress, ProgressEvent{Stage: "done", Package: spec.packageID, Percent: 1, Message: "plugin ready"})
	return InstallResult{
		Package: spec.packageID,
		Version: spec.version,
		Channel: channel,
		Source:  spec.source,
		Files:   []string{dst},
	}, nil
}

// ResolvePluginPackageID maps host+name to marketplace package ID.
func ResolvePluginPackageID(ctx context.Context, host, name string) (string, error) {
	pkgs, err := source.AllPackages(ctx)
	if err != nil {
		return "", err
	}
	bp, err := FindPlugin(pkgs, host, name)
	if err != nil {
		return "", err
	}
	return bp.Package.ID, nil
}
