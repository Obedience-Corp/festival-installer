package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/state"
)

func TestHubState_ErrorsAndAbsentHome(t *testing.T) {
	t.Run("cancelled context is reported when reading", func(t *testing.T) {
		t.Setenv("FESTIVAL_HOME", t.TempDir())
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, _, err := HubState(ctx, state.HubStateTourStep); err == nil {
			t.Fatal("expected an error reading with a cancelled context")
		} else if code := errpkg.Code(err); code != "E_HUB_STATE_CTX" {
			t.Fatalf("code = %q, want E_HUB_STATE_CTX", code)
		}
	})

	t.Run("cancelled context is reported when deleting", func(t *testing.T) {
		t.Setenv("FESTIVAL_HOME", t.TempDir())
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := DeleteHubState(ctx, state.HubStateTourStep); err == nil {
			t.Fatal("expected an error deleting with a cancelled context")
		} else if code := errpkg.Code(err); code != "E_HUB_STATE_CTX" {
			t.Fatalf("code = %q, want E_HUB_STATE_CTX", code)
		}
	})

	t.Run("a relative home is refused rather than guessed at", func(t *testing.T) {
		t.Setenv("FESTIVAL_HOME", "relative/home")
		if _, _, err := HubState(context.Background(), state.HubStateTourStep); err == nil {
			t.Fatal("expected an error for a relative FESTIVAL_HOME")
		}
	})

	t.Run("a home with no database reports the key as absent", func(t *testing.T) {
		t.Setenv("FESTIVAL_HOME", t.TempDir())
		value, ok, err := HubState(context.Background(), state.HubStateTourStep)
		if err != nil {
			t.Fatalf("HubState: %v", err)
		}
		if ok || value != "" {
			t.Fatalf("got (%q, %v), want the key absent on a home that was never written", value, ok)
		}
	})

	t.Run("deleting from a home with no database does nothing", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("FESTIVAL_HOME", home)
		if err := DeleteHubState(context.Background(), state.HubStateTourStep); err != nil {
			t.Fatalf("DeleteHubState: %v", err)
		}
		if _, err := os.Stat(state.DatabasePath(home)); !os.IsNotExist(err) {
			t.Fatal("a delete on an empty home must not create state.db")
		}
	})
}

func TestHubState_WriteReadDeleteAcrossProcesses(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)

	if err := SetHubState(ctx, state.HubStateTourStep, "2"); err != nil {
		t.Fatalf("SetHubState: %v", err)
	}

	value, ok, err := HubState(ctx, state.HubStateTourStep)
	if err != nil {
		t.Fatalf("HubState: %v", err)
	}
	if !ok || value != "2" {
		t.Fatalf("got (%q, %v), want (2, true)", value, ok)
	}

	if err := SetHubState(ctx, state.HubStateTourStep, "3"); err != nil {
		t.Fatalf("second SetHubState: %v", err)
	}
	value, _, err = HubState(ctx, state.HubStateTourStep)
	if err != nil {
		t.Fatalf("HubState after overwrite: %v", err)
	}
	if value != "3" {
		t.Fatalf("value = %q, want 3 after overwrite", value)
	}

	if err := DeleteHubState(ctx, state.HubStateTourStep); err != nil {
		t.Fatalf("DeleteHubState: %v", err)
	}
	if _, ok, err = HubState(ctx, state.HubStateTourStep); err != nil {
		t.Fatalf("HubState after delete: %v", err)
	} else if ok {
		t.Fatal("value still present after delete")
	}
}

// TestSetHubState_CreatesHomeAndSurvivesRestart is the resumption guarantee the
// sequence goal asks for: a first write on a machine with no home at all, read
// back through a fresh open the way a relaunched hub would read it.
func TestSetHubState_CreatesHomeAndSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	parent := t.TempDir()
	home := filepath.Join(parent, "never-created")
	t.Setenv("FESTIVAL_HOME", home)

	if err := SetHubState(ctx, state.HubStateTourDismissed, "true"); err != nil {
		t.Fatalf("SetHubState on an absent home: %v", err)
	}
	if _, err := os.Stat(state.DatabasePath(home)); err != nil {
		t.Fatalf("state.db not created by the first write: %v", err)
	}

	value, ok, err := HubState(ctx, state.HubStateTourDismissed)
	if err != nil {
		t.Fatalf("HubState: %v", err)
	}
	if !ok || value != "true" {
		t.Fatalf("got (%q, %v), want (true, true) after a restart", value, ok)
	}
}
