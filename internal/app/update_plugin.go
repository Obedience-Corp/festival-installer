package app

import (
	"context"
	"errors"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/installer"
	"github.com/Obedience-Corp/festival-installer/internal/source"
)

// UpdatePlugin reconciles only the explicitly selected receipt-owned plugin.
// Foreign replacements and standalone installs are reported, never adopted.
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
	views, err := source.RefreshMarketplaces(ctx, receipt.Source, opts.Verify)
	if err != nil {
		return UpdateResult{}, "", err
	}
	for _, view := range views {
		if view.Err != "" {
			return UpdateResult{}, "", errpkg.New("E_PLUGIN_SOURCE", view.Err)
		}
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
	if err := chooseAdapter(target.Host).ValidateCompatibility(ctx, spec.target); err != nil {
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
