package cli

import (
	"context"
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
	"github.com/Obedience-Corp/obey-installer/internal/source"
	"github.com/Obedience-Corp/obey-installer/internal/state"
)

func pluginHost(selector string) (string, string, bool) {
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

func findPlugin(pkgs []source.BrowsePackage, host, name string) (source.BrowsePackage, error) {
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

func selectBinaryArtifact(rel metadata.Release, goos, goarch string) (metadata.Artifact, error) {
	for _, art := range rel.Artifacts {
		if art.Kind == "binary" && art.OS == goos && art.Arch == goarch {
			return art, nil
		}
	}
	return metadata.Artifact{}, errpkg.New("E_NO_ARTIFACT", "no binary artifact for "+goos+"/"+goarch)
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

func installPlugin(ctx context.Context, host, name, channel string) (installResult, error) {
	if err := ctx.Err(); err != nil {
		return installResult{}, errpkg.Wrap("E_INSTALL_CTX", err, "context cancelled")
	}

	pkgs, err := source.AllPackages(ctx)
	if err != nil {
		return installResult{}, err
	}
	bp, err := findPlugin(pkgs, host, name)
	if err != nil {
		return installResult{}, err
	}
	pkg := bp.Package

	manifest, err := source.LoadPackageManifest(ctx, bp.Source, pkg.ID)
	if err != nil {
		return installResult{}, err
	}
	rel, err := installer.SelectRelease(manifest, channel)
	if err != nil {
		return installResult{}, err
	}
	art, err := selectBinaryArtifact(rel, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return installResult{}, err
	}
	entry, err := binaryEntry(rel, host+"-"+name)
	if err != nil {
		return installResult{}, err
	}

	if err := chooseAdapter(host).ValidateCompatibility(ctx, selectTarget(pkg.Targets, host)); err != nil {
		return installResult{}, err
	}

	home, err := state.Home(ctx)
	if err != nil {
		return installResult{}, err
	}
	binDir, err := state.BinDir(ctx)
	if err != nil {
		return installResult{}, err
	}
	db, err := state.OpenDB(ctx, home)
	if err != nil {
		return installResult{}, err
	}
	defer func() { _ = db.Close(ctx) }()

	tx, err := installer.Begin(ctx, db.Raw(), home)
	if err != nil {
		return installResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	staged, err := artifacts.NewDownloader().Download(ctx, art.URL, tx.StagingDir())
	if err != nil {
		return installResult{}, err
	}
	if err := artifacts.VerifySHA256(ctx, staged, art.Sha256); err != nil {
		return installResult{}, err
	}
	hash, err := artifacts.SHA256(ctx, staged)
	if err != nil {
		return installResult{}, err
	}
	execName := entryExecutableName(entry)
	if err := shared.ValidateSegment(execName); err != nil {
		return installResult{}, err
	}
	dst := filepath.Join(binDir, execName)
	if err := tx.Stage(ctx, installer.StagedFile{StagedPath: staged, DestPath: dst, Sha256: hash, Mode: 0o755}); err != nil {
		return installResult{}, err
	}
	if _, err := tx.Commit(ctx, installer.ReceiptInfo{
		PackageID:   pkg.ID,
		Version:     rel.Version,
		Channel:     channel,
		Source:      bp.Source,
		ManifestURL: art.URL,
	}); err != nil {
		return installResult{}, err
	}

	return installResult{
		Package: pkg.ID,
		Version: rel.Version,
		Channel: channel,
		Source:  bp.Source,
		Files:   []string{dst},
	}, nil
}

func resolvePluginPackageID(ctx context.Context, host, name string) (string, error) {
	pkgs, err := source.AllPackages(ctx)
	if err != nil {
		return "", err
	}
	bp, err := findPlugin(pkgs, host, name)
	if err != nil {
		return "", err
	}
	return bp.Package.ID, nil
}
