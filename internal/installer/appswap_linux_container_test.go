//go:build container_fs && linux

package installer

import (
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/metadata"
)

// requireAppIsolation is a no-op: the container is the isolation.
func requireAppIsolation(t *testing.T) { t.Helper() }

// TestAppSwapSimulatedExchange runs the bundle crash table through a fake
// atomic exchange so the after-exchange point and reconcile's exchange step
// run on Linux too.
func TestAppSwapSimulatedExchange(t *testing.T) {
	requireAppIsolation(t)
	prev := exchangeFn
	exchangeFn = fakeExchange
	t.Cleanup(func() { exchangeFn = prev })
	reached := runCrashTable(t, appScenario{metadata.ArtifactKindAppBundle, true})
	if !reached["after-exchange"] {
		t.Fatal("after-exchange never reached with the simulated exchange")
	}
}
