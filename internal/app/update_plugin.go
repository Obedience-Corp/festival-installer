package app

import (
	"context"
	"errors"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/installer"
	"github.com/Obedience-Corp/festival-installer/internal/source"
)

// UpdatePlugin reconciles the explicitly selected plugin. App-managed setup may
// create a separate managed copy; standalone files are never adopted or replaced.
func UpdatePlugin(ctx context.Context, selector string, opts UpdateOptions) (UpdateResult, string, error) {
	target, err := ParsePluginTarget(selector)
	if err != nil {
		return UpdateResult{}, "", err
	}
	if err := ctx.Err(); err != nil {
		return UpdateResult{}, "", errpkg.Wrap("E_PLUGIN_CTX", err, "update selected plugin")
	}
	if target.PackageID == "" {
		target.PackageID, err = ResolvePluginPackageID(ctx, target.Host, target.Name, opts.Verify)
		if err != nil {
			return UpdateResult{}, "", err
		}
	}
	receipt, found, err := pluginReceipt(ctx, target.PackageID)
	if err != nil {
		return UpdateResult{}, "", err
	}
	result := UpdateResult{Package: target.PackageID, Action: "absent"}
	if !found {
		if managedToolchain(ctx) {
			return installManagedPluginUpdate(ctx, target, opts)
		}
		if path, _, resolveErr := resolveToolFrom(ctx, target.Host+"-"+target.Name, SuiteOrigin{}); resolveErr == nil {
			result.Action = "unmanaged"
			result.Upgrade = "preserved " + path + "; update it through its existing installer"
		}
		return result, "", nil
	}
	if err := validatePluginReceiptVersion(receipt); err != nil {
		return UpdateResult{}, "", err
	}
	result.Version = receipt.Version
	intact, err := pluginReceiptIntact(ctx, receipt)
	if err != nil {
		var coded *errpkg.Error
		if errors.As(err, &coded) && coded.Code == "E_PLUGIN_FOREIGN" {
			result.Action, result.Upgrade = "unmanaged", err.Error()
			return result, "", nil
		}
		return UpdateResult{}, "", err
	}
	channel := receipt.Channel
	if opts.ChannelOverride != "" {
		channel = opts.ChannelOverride
	}
	if channel == "" {
		channel = "stable"
	}
	if err := ValidateChannel(channel); err != nil {
		return UpdateResult{}, "", err
	}
	if err := refreshPluginSource(ctx, receipt.Source, opts.Verify); err != nil {
		return UpdateResult{}, "", err
	}
	pkgs, err := source.SourcePackages(ctx, receipt.Source, opts.Verify)
	if err != nil {
		return UpdateResult{}, "", err
	}
	pkg, err := findSelectedPlugin(pkgs, target)
	if err != nil {
		return UpdateResult{}, "", err
	}
	var spec pluginSpec
	if pkg.Package.ReleaseSource != nil {
		spec, err = SpecFromGit(ctx, pkg, target.Host, target.Name, channel, opts.Verify)
	} else {
		spec, err = SpecFromManifest(ctx, pkg, target.Host, target.Name, channel, opts.Verify)
	}
	if err != nil {
		return UpdateResult{}, "", err
	}
	if err := chooseAdapter(ctx, target.Host).ValidateCompatibility(ctx, spec.target); err != nil {
		return UpdateResult{}, "", err
	}
	result.Latest = spec.version
	if installer.VersionLess(spec.version, receipt.Version) {
		if !intact {
			return result, "", errpkg.New("E_PLUGIN_REPAIR_VERSION", "repair needs the installed newer version; refusing to downgrade "+target.PackageID)
		}
		result.Action = "current"
		return result, "", nil
	}
	equivalent := !installer.VersionLess(receipt.Version, spec.version) && !installer.VersionLess(spec.version, receipt.Version)
	if equivalent && intact {
		result.Action = "current"
		return result, "", nil
	}
	if _, err := activatePlugin(ctx, spec, channel, opts.Progress); err != nil {
		return UpdateResult{}, "", err
	}
	result.Action, result.From, result.Version = "upgraded", receipt.Version, spec.version
	if equivalent {
		result.Action = "repaired"
	}
	return result, "", nil
}

func installManagedPluginUpdate(ctx context.Context, target PluginTarget, opts UpdateOptions) (UpdateResult, string, error) {
	pkgs, err := source.AllPackages(ctx, opts.Verify)
	if err != nil {
		return UpdateResult{}, "", err
	}
	pkg, err := findSelectedPlugin(pkgs, target)
	if err != nil {
		return UpdateResult{}, "", err
	}
	// A first managed copy has no receipt naming its source. Resolve that source
	// from the verified catalog and refresh it before selecting a release, just
	// as receipt-owned updates do. An old cached manifest may be below App's floor.
	if err := refreshPluginSource(ctx, pkg.Source, opts.Verify); err != nil {
		return UpdateResult{}, "", err
	}
	return installManagedUpdate(ctx, target.PackageID, opts)
}

func refreshPluginSource(ctx context.Context, name string, vo source.VerifyOptions) error {
	views, err := source.RefreshMarketplaces(ctx, name, vo)
	if err != nil {
		return err
	}
	for _, view := range views {
		if view.Err != "" {
			return errpkg.New("E_PLUGIN_SOURCE", view.Err)
		}
	}
	return nil
}
