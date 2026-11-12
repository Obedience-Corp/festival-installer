//go:build darwin && app_native

package installer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/apptest"
	"github.com/Obedience-Corp/festival-installer/internal/metadata"
)

func requireAppIsolation(t *testing.T) {
	t.Helper()
	apptest.RequireHostedRunner(t)
}

// TestAppSwapRenameSwapOnAPFS proves the real RENAME_SWAP path on a hosted macOS VM.
func TestAppSwapRenameSwapOnAPFS(t *testing.T) {
	apptest.RequireHostedRunner(t)
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	for name, marker := range map[string]string{a: "A", b: "B"} {
		if err := os.MkdirAll(name, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(name, "m"), []byte(marker), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := exchange(a, b); err != nil {
		t.Fatalf("real exchange: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(a, "m")); string(got) != "B" {
		t.Fatalf("a holds %q after exchange", got)
	}
	reached := runCrashTable(t, appScenario{metadata.ArtifactKindAppBundle, true})
	if !reached["after-exchange"] {
		t.Fatal("real RENAME_SWAP path never reached")
	}
}
