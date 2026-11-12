package app

import (
	"context"
	"path"
	"runtime"
	"strings"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/installer"
	"github.com/Obedience-Corp/festival-installer/internal/metadata"
	"github.com/Obedience-Corp/festival-installer/internal/source"
	"github.com/Obedience-Corp/festival-installer/internal/state"
)

const (
	AppPackageID   = "obedience-corp/festival-app"
	AppBundleID    = "com.obediencecorp.festival-app"
	AppProductName = "Festival App"
	appChannel     = "stable"
	appDownloadURL = "https://fest.build/app"
)

var loadSignedAppManifest = source.LoadSignedPackageManifest

type appResolveOptions struct {
	Source string // "" means state config Marketplaces.Default (official-obey)
	Format string // metadata.ArtifactKind*; chosen by the caller
	GOOS   string // "" means runtime.GOOS
	GOARCH string // "" means runtime.GOARCH
	Verify source.VerifyOptions
}

type appRelease struct {
	PackageID   string
	Version     string
	Channel     string
	Source      string
	Format      string
	Artifact    metadata.Artifact // URL, Sha256, Filename (never empty), UpdaterSignature (never empty)
	BundleID    string
	ProductName string
}

// appFormatsByOS lists the artifact formats each OS can install.
var appFormatsByOS = map[string][]string{
	"darwin":  {metadata.ArtifactKindAppBundle},
	"windows": {metadata.ArtifactKindNSIS},
	"linux":   {metadata.ArtifactKindDeb, metadata.ArtifactKindAppImage},
}

// appSignatureCodes are the metadata and signature failures that mean the
// App's manifest cannot be trusted.
var appSignatureCodes = map[string]bool{
	"E_UNVERIFIED_REFUSED": true, "E_SIG_INVALID": true, "E_SIG_ALG": true,
	"E_SIG_MALFORMED": true, "E_KEY_NOT_FOUND": true,
}

func appNotAvailable(sourceName, detail string) error {
	return errpkg.New("E_APP_NOT_AVAILABLE", "Festival App is not available from the "+sourceName+" marketplace yet ("+detail+"). Download it from "+appDownloadURL)
}

// validateAppFormat checks format against what goos can install, before any
// network work.
func validateAppFormat(goos, format string) error {
	formats, ok := appFormatsByOS[goos]
	if !ok {
		return errpkg.New("E_APP_PLATFORM", "Festival App has no build for operating system "+goos)
	}
	for _, f := range formats {
		if f == format {
			return nil
		}
	}
	return errpkg.New("E_APP_FORMAT_UNSUPPORTED", "format "+format+" is not supported on "+goos+" (supported: "+strings.Join(formats, ", ")+")")
}

// resolveAppRelease resolves the App for this host's os and arch, the given
// source ("" = config default) and format. It takes no installer lock of its
// own beyond the ones inside findProductPackage, so callers run it before they
// take the App lock.
func resolveAppRelease(ctx context.Context, sourceName, format string, vo source.VerifyOptions) (appRelease, error) {
	return resolveAppReleaseFor(ctx, appResolveOptions{Source: sourceName, Format: format, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Verify: vo})
}

// resolveAppReleaseFor does the work; tests set GOOS and GOARCH.
func resolveAppReleaseFor(ctx context.Context, opts appResolveOptions) (appRelease, error) {
	if err := ctx.Err(); err != nil {
		return appRelease{}, err
	}
	goos, goarch := opts.GOOS, opts.GOARCH
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	if err := validateAppFormat(goos, opts.Format); err != nil {
		return appRelease{}, err
	}
	home, err := state.Home(ctx)
	if err != nil {
		return appRelease{}, err
	}
	cfg, err := state.LoadConfig(ctx, home)
	if err != nil {
		return appRelease{}, err
	}
	name := opts.Source
	if name == "" {
		name = cfg.Marketplaces.Default
	}
	vo := source.OptionsForSource(name, opts.Verify)
	vo.AllowUnverified = false
	if name == state.OfficialSeedKey {
		if err := ensureOfficialSeed(ctx, vo); err != nil {
			coded := errpkg.Wrap("E_MARKETPLACE_SEED", err, "ensure official marketplace")
			return appRelease{}, &MarketplaceSeedProblem{Err: coded, Fatal: true}
		}
	}
	bp, err := findProductPackage(ctx, name, AppPackageID, vo)
	if err != nil {
		if errpkg.Code(err) == "E_PACKAGE_NOT_FOUND" {
			return appRelease{}, appNotAvailable(name, "the marketplace does not list Festival App")
		}
		return appRelease{}, err
	}
	if !bp.Verified {
		return appRelease{}, errpkg.New("E_APP_SIGNATURE", "the "+name+" marketplace catalog is not signed; Festival App installs only from a signed catalog")
	}
	manifest, err := loadSignedAppManifest(ctx, name, AppPackageID, vo)
	if err != nil {
		if appSignatureCodes[errpkg.Code(err)] {
			return appRelease{}, errpkg.Wrap("E_APP_SIGNATURE", err, "Festival App manifest signature")
		}
		return appRelease{}, err
	}
	rel, err := installer.SelectRelease(manifest, appChannel)
	if err != nil {
		if errpkg.Code(err) == "E_NO_RELEASE_IN_CHANNEL" {
			return appRelease{}, appNotAvailable(name, "no stable release yet")
		}
		return appRelease{}, err
	}
	if !hasDesktopAppEntry(rel) {
		return appRelease{}, appNotAvailable(name, "release "+rel.Version+" is not a Festival App desktop release")
	}
	art, err := installer.SelectAppArtifact(rel, goos, goarch, opts.Format)
	if err != nil {
		return appRelease{}, err
	}
	if art.Filename == "" {
		art.Filename = path.Base(art.URL)
	}
	if strings.TrimSpace(art.UpdaterSignature) == "" {
		return appRelease{}, errpkg.New("E_APP_SIGNATURE", "no updater signature for "+art.Filename)
	}
	return appRelease{
		PackageID: AppPackageID, Version: rel.Version, Channel: appChannel, Source: name,
		Format: opts.Format, Artifact: art, BundleID: AppBundleID, ProductName: AppProductName,
	}, nil
}

// hasDesktopAppEntry reports whether rel carries exactly one desktop_app
// install entry for the Festival App bundle id.
func hasDesktopAppEntry(rel metadata.Release) bool {
	n := 0
	for _, e := range rel.Install.Entries {
		if e.Kind == metadata.InstallKindDesktopApp && e.BundleID == AppBundleID {
			n++
		}
	}
	return n == 1
}
