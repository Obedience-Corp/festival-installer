package app

import (
	"strings"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/hosts/shared"
	"github.com/Obedience-Corp/festival-installer/internal/source"
)

// PluginTarget preserves an explicit marketplace identity as well as its host selector.
type PluginTarget struct {
	Host      string
	Name      string
	PackageID string
}

// ParsePluginTarget accepts a bare plugin selector or namespace/plugin selector.
func ParsePluginTarget(selector string) (PluginTarget, error) {
	var target PluginTarget
	if strings.Count(selector, "/") > 1 {
		return target, errpkg.New("E_PLUGIN_TARGET", "expected namespace/camp-plugin or namespace/fest-plugin")
	}
	if namespace, short, qualified := strings.Cut(selector, "/"); qualified {
		if err := shared.ValidateSegment(namespace); err != nil {
			return target, err
		}
		target.PackageID = selector
		selector = short
	}
	host, name, ok := PluginHost(selector)
	if !ok {
		return target, errpkg.New("E_PLUGIN_TARGET", "expected a camp-* or fest-* plugin")
	}
	if err := shared.ValidateSegment(name); err != nil {
		return target, err
	}
	target.Host, target.Name = host, name
	return target, nil
}

func findSelectedPlugin(pkgs []source.BrowsePackage, target PluginTarget) (source.BrowsePackage, error) {
	if target.PackageID == "" {
		return FindPlugin(pkgs, target.Host, target.Name)
	}
	var selected []source.BrowsePackage
	for _, pkg := range pkgs {
		if pkg.Package.ID == target.PackageID {
			selected = append(selected, pkg)
		}
	}
	return FindPlugin(selected, target.Host, target.Name)
}
