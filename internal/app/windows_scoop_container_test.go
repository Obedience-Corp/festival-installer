//go:build windows && container_fs

package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The installed manifest and .shim file follow Scoop's generated layout. The
// tested path is the shim on PATH, not the executable inside apps/festival.
func TestWindowsScoopShimOriginAndGuidance(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Scoop Root [proof]")
	shims := filepath.Join(root, "shims")
	appDir := filepath.Join(root, "apps", "festival", "current")
	for _, dir := range []string{shims, appDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	manifest := []byte(`{"architecture":{"64bit":{"bin":["festival.exe","fest.exe","camp.exe"]}}}`)
	if err := os.WriteFile(filepath.Join(appDir, "scoop-manifest.json"), manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"camp", "fest", "festival"} {
		filename := name + ".exe"
		target := filepath.Join(appDir, filename)
		if err := os.WriteFile(target, []byte("exe"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(shims, filename), []byte("scoop shim"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(shims, name+".shim"), []byte("\ufeffpath = \""+target+"\"\r\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		kind, flavor, pkg := classifyPath(context.Background(), filepath.Join(shims, filename))
		if kind != OriginPackage || flavor != FlavorScoop || pkg != "festival" {
			t.Fatalf("%s origin = %q/%q/%q", name, kind, flavor, pkg)
		}
	}
	t.Setenv("FESTIVAL_HOME", filepath.Join(root, "installer home"))
	t.Setenv("PATH", shims)
	origin, err := DetectSuite(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if origin.Kind != OriginPackage || origin.Flavor != FlavorScoop || origin.Upgrade != "scoop update festival" || origin.Remove != "scoop uninstall festival" {
		t.Fatalf("Scoop origin/guidance = %+v", origin)
	}
	if check := managedBinOnPathFrom(origin, SetupState{}); check.Status != DoctorOK {
		t.Fatalf("doctor Scoop PATH check = %+v", check)
	}
	var pathCheck DoctorCheck
	for _, check := range Doctor(context.Background()) {
		if check.ID == "managed_bin_on_path" {
			pathCheck = check
		}
	}
	if pathCheck.Status != DoctorOK {
		t.Fatalf("full doctor Scoop PATH check = %+v", pathCheck)
	}
	guidance, err := ShellGuidanceFor(context.Background(), "powershell")
	if err != nil {
		t.Fatal(err)
	}
	if !guidance.OnPath || !strings.Contains(guidance.Snippet, "scoop update festival") || strings.Contains(guidance.Snippet, "source ") {
		t.Fatalf("PowerShell Scoop guidance = %+v", guidance)
	}
	old := LookupLatestSuite
	LookupLatestSuite = func(context.Context, string) (string, error) { return "1.1.0", nil }
	t.Cleanup(func() { LookupLatestSuite = old })
	res, _, err := UpdateFestival(context.Background(), UpdateOptions{})
	if err != nil || res.Action != "package" || res.Upgrade != "scoop update festival" {
		t.Fatalf("Scoop update = %+v, %v", res, err)
	}
	if _, err := os.Stat(filepath.Join(root, "installer home", "state.db")); !os.IsNotExist(err) {
		t.Fatalf("Scoop update created managed state: %v", err)
	}
	if _, err := InstallFestival(context.Background(), InstallOptions{}); err == nil || !strings.Contains(err.Error(), "scoop update festival") {
		t.Fatalf("Scoop package install must refuse managed overwrite with guidance: %v", err)
	}
	removed, err := UninstallPackage(context.Background(), FestivalPackageID)
	if err != nil || !strings.Contains(removed.Note, "scoop uninstall festival") {
		t.Fatalf("Scoop uninstall guidance = %+v, %v", removed, err)
	}
}

func TestWindowsScoopShimRejectsUnownedPaths(t *testing.T) {
	root := t.TempDir()
	t.Setenv("FESTIVAL_HOME", filepath.Join(root, "installer home"))
	shims := filepath.Join(root, "shims")
	appDir := filepath.Join(root, "apps", "festival", "current")
	otherDir := filepath.Join(root, "apps", "other", "current")
	for _, dir := range []string{shims, appDir, otherDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	shim := filepath.Join(shims, "camp.exe")
	for _, file := range []string{shim, filepath.Join(appDir, "camp.exe"), filepath.Join(otherDir, "camp.exe")} {
		if err := os.WriteFile(file, []byte("exe"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	manifestPath := filepath.Join(appDir, "scoop-manifest.json")
	shimMeta := filepath.Join(shims, "camp.shim")
	checkLeftover := func(label string) {
		t.Helper()
		kind, flavor, _ := classifyPath(context.Background(), shim)
		if kind != OriginLeftover || flavor != "" {
			t.Fatalf("%s = %q/%q, want leftover", label, kind, flavor)
		}
	}
	checkLeftover("no .shim")
	if err := os.WriteFile(shimMeta, []byte("path = \""+filepath.Join(otherDir, "camp.exe")+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	checkLeftover("foreign app target")
	if err := os.WriteFile(shimMeta, []byte("path = \""+filepath.Join(appDir, "camp.exe")+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	checkLeftover("missing installed manifest")
	if err := os.WriteFile(manifestPath, []byte(`{"architecture":{"64bit":{"bin":["fest.exe"]}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	checkLeftover("manifest does not own shim")
}
