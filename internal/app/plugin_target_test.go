package app

import (
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/metadata"
	"github.com/Obedience-Corp/festival-installer/internal/source"
)

func TestSelectedPluginIdentityCannotResolveAnotherPublisher(t *testing.T) {
	packages := []source.BrowsePackage{
		{Source: "other", Package: source.PackageRef{ID: "other/camp-app", Class: "plugin", HostRuntimes: []string{"camp-cli"}, Targets: []metadata.RuntimeTarget{{Runtime: "camp-cli"}}}},
		{Source: "official", Package: source.PackageRef{ID: "obedience-corp/camp-app", Class: "plugin", HostRuntimes: []string{"camp-cli"}, Targets: []metadata.RuntimeTarget{{Runtime: "camp-cli"}}}},
	}
	target, err := ParsePluginTarget("obedience-corp/camp-app")
	if err != nil {
		t.Fatal(err)
	}
	selected, err := findSelectedPlugin(packages, target)
	if err != nil || selected.Package.ID != target.PackageID {
		t.Fatalf("wrong selected identity: %+v %v", selected, err)
	}
	if _, err := findSelectedPlugin(packages[:1], target); err == nil {
		t.Fatal("resolved another publisher")
	}
}

func TestPluginSelectorRejectsPathsAndEmptyPluginNames(t *testing.T) {
	for _, selector := range []string{"../camp-app", "acme/../camp-app", "camp-", "acme/fest-", "/camp-app", "acme/camp-app/extra"} {
		if _, err := ParsePluginTarget(selector); err == nil {
			t.Errorf("accepted %q", selector)
		}
	}
}
