package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/Obedience-Corp/festival-installer/internal/artifacts"
	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/installer"
	"github.com/Obedience-Corp/festival-installer/internal/source"
)

const (
	UpdateAvailable   = "available"
	UpdateCurrent     = "current"
	UpdateExternal    = "external"
	UpdateUnsupported = "unsupported"
	UpdateCheckFailed = "error"
)

// PackageUpdate is a fresh availability check, never an installation request.
type PackageUpdate struct {
	Package   string `json:"package"`
	Installed string `json:"installed"`
	Latest    string `json:"latest,omitempty"`
	Channel   string `json:"channel"`
	Source    string `json:"source"`
	Status    string `json:"status"`
	Detail    string `json:"detail,omitempty"`
}

type UpdatesResult struct {
	CheckedAt time.Time       `json:"checked_at"`
	Packages  []PackageUpdate `json:"packages"`
}

func (r UpdatesResult) Available() int {
	n := 0
	for _, p := range r.Packages {
		if p.Status == UpdateAvailable {
			n++
		}
	}
	return n
}

func (r UpdatesResult) Failed() bool {
	for _, p := range r.Packages {
		if p.Status == UpdateCheckFailed {
			return true
		}
	}
	return false
}

// CheckUpdates refreshes each installed source once. It changes only catalog
// caches; receipt ownership, installed files and running services are untouched.
func CheckUpdates(ctx context.Context, vo source.VerifyOptions) (UpdatesResult, error) {
	entries, err := ListInstalled(ctx)
	if err != nil {
		return UpdatesResult{}, err
	}
	type sourceResult struct {
		packages []source.BrowsePackage
		err      error
	}
	sources := map[string]sourceResult{}
	load := func(name string) ([]source.BrowsePackage, error) {
		if cached, ok := sources[name]; ok {
			return cached.packages, cached.err
		}
		var result sourceResult
		if name == "" {
			result.err = errpkg.New("E_UPDATE_SOURCE", "installed receipt has no source; repair the receipt before updating")
		} else {
			options := source.OptionsForSource(name, vo)
			views, refreshErr := source.RefreshMarketplaces(ctx, name, options)
			result.err = refreshErr
			if result.err == nil && (len(views) != 1 || views[0].Err != "") {
				detail := "no refresh result"
				if len(views) == 1 {
					detail = views[0].Err
				}
				result.err = errpkg.New("E_UPDATE_SOURCE", "refresh "+name+": "+detail)
			}
			if result.err == nil {
				result.packages, result.err = source.SourcePackages(ctx, name, options)
			}
		}
		sources[name] = result
		return result.packages, result.err
	}
	return checkInstalledUpdates(ctx, entries.Packages, func(ctx context.Context, entry ListEntry) (PackageUpdate, error) {
		return checkPackageUpdate(ctx, entry, vo, load)
	}), nil
}

func checkInstalledUpdates(ctx context.Context, entries []ListEntry, check func(context.Context, ListEntry) (PackageUpdate, error)) UpdatesResult {
	result := UpdatesResult{Packages: make([]PackageUpdate, 0, len(entries))}
	for _, entry := range entries {
		p := PackageUpdate{Package: entry.PackageID, Installed: entry.Version, Channel: entry.Channel, Source: entry.Source}
		if p.Channel == "" {
			p.Channel = "stable"
		}
		err := ctx.Err()
		if err == nil {
			p, err = check(ctx, entry)
		}
		if err != nil {
			p.Status, p.Detail = UpdateCheckFailed, err.Error()
			if errors.Is(err, installer.ErrNoArtifactForPlatform) || errpkg.Code(err) == "E_NO_ARTIFACT" {
				p.Status = UpdateUnsupported
			}
		}
		result.Packages = append(result.Packages, p)
	}
	result.CheckedAt = time.Now().UTC()
	return result
}

func checkPackageUpdate(ctx context.Context, entry ListEntry, vo source.VerifyOptions, load func(string) ([]source.BrowsePackage, error)) (PackageUpdate, error) {
	p := PackageUpdate{Package: entry.PackageID, Installed: entry.Version, Channel: entry.Channel, Source: entry.Source}
	if p.Channel == "" {
		p.Channel = "stable"
	}
	if entry.Origin != "" {
		p.Status = UpdateExternal
		p.Detail = "Update through the existing installer."
		if entry.PackageID == FestivalPackageID && entry.Origin == "package" {
			origin, err := DetectSuite(ctx)
			if err != nil {
				return p, err
			}
			p.Detail = origin.Upgrade
			p.Latest, err = LookupLatestSuite(ctx, p.Channel)
			return p, err
		}
		return p, nil
	}
	if err := ValidateChannel(p.Channel); err != nil {
		return p, err
	}
	receipt, exists, err := readReceipt(ctx, entry.PackageID)
	if err != nil {
		return p, err
	}
	if !exists {
		return p, errpkg.New("E_UPDATE_RECEIPT", "installation changed; refresh updates")
	}
	if err := validatePluginReceiptVersion(receipt); err != nil {
		return p, err
	}
	_, err = pluginReceiptIntact(ctx, receipt)
	if err != nil {
		if errpkg.Code(err) == "E_PLUGIN_FOREIGN" {
			p.Status, p.Detail = UpdateExternal, err.Error()
			return p, nil
		}
		return p, err
	}
	packages, err := load(entry.Source)
	if err != nil {
		return p, err
	}
	var bp source.BrowsePackage
	found := false
	for _, candidate := range packages {
		if candidate.Package.ID == entry.PackageID {
			bp, found = candidate, true
			break
		}
	}
	if !found {
		return p, errpkg.New("E_PACKAGE_NOT_FOUND", "no package "+entry.PackageID+" in "+entry.Source)
	}
	options := source.OptionsForSource(entry.Source, vo)
	var url string
	switch entry.PackageID {
	case FestivalPackageID, ObeyPackageID:
		resolved, resolveErr := resolveProduct(ctx, bp, p.Channel, options)
		if resolveErr != nil {
			return p, resolveErr
		}
		p.Latest, url = resolved.version, resolved.url
	default:
		target, parseErr := ParsePluginTarget(entry.PackageID)
		if parseErr != nil {
			p.Status, p.Detail = UpdateUnsupported, "This package has no supported update operation."
			return p, nil
		}
		var spec pluginSpec
		if bp.Package.ReleaseSource != nil {
			// Match UpdatePlugin's explicit consent policy for unsigned dynamic
			// releases; source refresh policy alone must not advertise eligibility.
			spec, err = SpecFromGit(ctx, bp, target.Host, target.Name, p.Channel, vo)
		} else {
			spec, err = SpecFromManifest(ctx, bp, target.Host, target.Name, p.Channel, options)
		}
		if err != nil {
			return p, err
		}
		if err := chooseAdapter(ctx, target.Host).ValidateCompatibility(ctx, spec.target); err != nil {
			return p, err
		}
		p.Latest, url = spec.version, spec.artifactURL
	}
	p.Status = UpdateCurrent
	if installer.VersionLess(p.Installed, p.Latest) {
		// Tags and checksum entries can precede asset upload. Do not advertise
		// an update until its platform artifact is publicly downloadable.
		if err := checkUpdateArtifact(ctx, url); err != nil {
			return p, err
		}
		p.Status = UpdateAvailable
	}
	return p, nil
}

func checkUpdateArtifact(ctx context.Context, url string) error {
	if err := artifacts.RequireHTTPS(url); err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return errpkg.Wrap("E_UPDATE_ARTIFACT", err, "check release artifact")
	}
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: artifacts.CheckRedirect}
	response, err := client.Do(request)
	if err != nil {
		return errpkg.Wrap("E_UPDATE_ARTIFACT", err, "check release artifact")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return errpkg.New("E_UPDATE_ARTIFACT", fmt.Sprintf("release artifact is not ready (%s); refresh updates to retry", response.Status))
	}
	return nil
}

type PackageUpdateResult struct {
	UpdateResult
	Warning string `json:"warning,omitempty"`
	Error   string `json:"error,omitempty"`
}

type UpdateAllResult struct {
	Packages []PackageUpdateResult `json:"packages"`
}

func (r UpdateAllResult) Failed() bool {
	for _, p := range r.Packages {
		if p.Error != "" {
			return true
		}
	}
	return false
}

// UpdateInstalled updates an existing, unchanged receipt-owned package only.
// Checking again protects a UI selection whose files or receipt changed while
// the user was reading it. The existing engines re-resolve before placement.
func UpdateInstalled(ctx context.Context, packageID string, opts UpdateOptions) (UpdateResult, string, error) {
	receipt, exists, err := readReceipt(ctx, packageID)
	if err != nil {
		return UpdateResult{}, "", err
	}
	if !exists {
		return UpdateResult{}, "", errpkg.New("E_UPDATE_RECEIPT", "package is no longer installed through Festival; refresh updates")
	}
	if _, err := pluginReceiptIntact(ctx, receipt); err != nil {
		return UpdateResult{}, "", err
	}
	switch packageID {
	case FestivalPackageID:
		return UpdateFestival(ctx, opts)
	case ObeyPackageID:
		return UpdateObey(ctx, opts)
	default:
		return UpdatePlugin(ctx, packageID, opts)
	}
}

// UpdateAll preserves each receipt's source/channel and defers Obey restarts.
// External installs receive guidance. One failure does not hide other results.
func UpdateAll(ctx context.Context, opts UpdateOptions) (UpdateAllResult, error) {
	if opts.ChannelOverride != "" || opts.Force {
		return UpdateAllResult{}, errpkg.New("E_UPDATE_ALL_OPTIONS", "update all retains each installed channel and owner; select a package to override them")
	}
	checked, err := CheckUpdates(ctx, opts.Verify)
	if err != nil {
		return UpdateAllResult{}, err
	}
	opts.NoRestart = true
	return updateCheckedPackages(ctx, checked, func(ctx context.Context, id string) (UpdateResult, string, error) {
		return UpdateInstalled(ctx, id, opts)
	}), nil
}

func updateCheckedPackages(ctx context.Context, checked UpdatesResult, update func(context.Context, string) (UpdateResult, string, error)) UpdateAllResult {
	packages := append([]PackageUpdate(nil), checked.Packages...)
	// Replace the running hub last, after the plugin/product operations finish.
	sort.SliceStable(packages, func(i, j int) bool {
		return packages[i].Package != FestivalPackageID && packages[j].Package == FestivalPackageID
	})
	result := UpdateAllResult{Packages: make([]PackageUpdateResult, 0, len(packages))}
	for _, p := range packages {
		item := PackageUpdateResult{UpdateResult: UpdateResult{Package: p.Package, Version: p.Installed, Latest: p.Latest, Action: p.Status}, Warning: p.Detail}
		switch p.Status {
		case UpdateAvailable:
			err := ctx.Err()
			if err == nil {
				item.UpdateResult, item.Warning, err = update(ctx, p.Package)
			}
			item.Package = p.Package
			if err != nil {
				item.Action, item.Error = "failed", err.Error()
			}
		case UpdateCheckFailed:
			item.Action, item.Error = "failed", p.Detail
		case UpdateExternal:
			item.Action, item.Upgrade = "unmanaged", p.Detail
		}
		result.Packages = append(result.Packages, item)
	}
	return result
}
