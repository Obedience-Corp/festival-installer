//go:build container_fs && linux

package cli_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestV0312ProofWriter runs only from the v0.3.12 proof harness (APP_PROOF_MODE
// set by 05_release/results/v0312-branch-proof). Its name does not start with
// TestApp on purpose: app-container-proof fails on any skip, and this test
// skips outside the harness. Everything that writes state is the branch's
// production code driven through the same command tree cmd/festival wires up.
func TestV0312ProofWriter(t *testing.T) {
	mode := os.Getenv("APP_PROOF_MODE")
	if mode == "" {
		t.Skip("run from 003_INSTALLER/05_release/results/v0312-branch-proof only")
	}
	key := appNativeKey(t)
	switch mode {
	case "install":
		market := os.Getenv("APP_PROOF_MARKET")
		if market == "" {
			t.Fatal("APP_PROOF_MARKET is not set")
		}
		// The real 0.1.7 AppImage and its own .sig, listed for this container's
		// architecture (the receipt shape does not depend on it).
		asset := appNativeAppImageForHostArch(t)
		appNativeMarketplace(t, key, market, []map[string]any{appNativeArtifact(t, asset, asset)})
		if out, errOut, err := runInstaller(t, "marketplace", "add", market, "--name", "fixture-app"); err != nil {
			t.Fatalf("marketplace add: %v\n%s\n%s", err, out, errOut)
		}
		writeFile(t, filepath.Join(os.Getenv("FESTIVAL_HOME"), "config.toml"), "[marketplaces]\ndefault = \"fixture-app\"\n")
		r := appNativeResultOf(t, "install", appNativeJSON(t, "install", "app", "--format", "appimage", "--json"))
		want := filepath.Join(os.Getenv("HOME"), ".local", "bin", "festival-app")
		if r.Format != "appimage" || r.Location != want || r.Version != appNativeVersion {
			t.Fatalf("install result %+v, want appimage at %s", r, want)
		}
	case "status":
		s := appNativeStatusOf(t, "status")
		if !s.App.Installed || str(s.App.Version) != appNativeVersion {
			t.Fatalf("status.app = %+v", s.App)
		}
		appNativeToolOrder(t, "status", s)
	case "uninstall":
		appNativeResultOf(t, "uninstall", appNativeJSON(t, "uninstall", "app", "--json"))
	default:
		t.Fatalf("unknown APP_PROOF_MODE %q", mode)
	}
}

// appNativeAppImageForHostArch is the AppImage asset listed for this process's architecture.
func appNativeAppImageForHostArch(t *testing.T) appNativeAsset {
	t.Helper()
	for _, a := range appNativeAssets {
		if a.Kind == "appimage" {
			a.Arch = runtime.GOARCH
			return a
		}
	}
	t.Fatal("no appimage asset")
	return appNativeAsset{}
}
