package app

import (
	"context"
	"strconv"
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/state"
)

func TestLoadTour_Errors(t *testing.T) {
	t.Run("cancelled context is reported", func(t *testing.T) {
		t.Setenv("FESTIVAL_HOME", t.TempDir())
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := LoadTour(ctx); err == nil {
			t.Fatal("expected an error with a cancelled context")
		} else if code := errpkg.Code(err); code != "E_TOUR_CTX" {
			t.Fatalf("code = %q, want E_TOUR_CTX", code)
		}
	})

	t.Run("a relative home is refused", func(t *testing.T) {
		t.Setenv("FESTIVAL_HOME", "relative/home")
		if _, err := LoadTour(context.Background()); err == nil {
			t.Fatal("expected an error for a relative FESTIVAL_HOME")
		}
	})

	t.Run("a negative step index is refused", func(t *testing.T) {
		t.Setenv("FESTIVAL_HOME", t.TempDir())
		if err := RecordTourStep(context.Background(), -1); err == nil {
			t.Fatal("expected an error recording a negative step")
		} else if code := errpkg.Code(err); code != "E_TOUR_STEP" {
			t.Fatalf("code = %q, want E_TOUR_STEP", code)
		}
	})
}

func TestTourSteps_AreFourCompiledInSteps(t *testing.T) {
	steps := TourSteps()
	if len(steps) != 4 {
		t.Fatalf("len(TourSteps) = %d, want 4", len(steps))
	}
	for i, st := range steps {
		if st.Title == "" {
			t.Fatalf("step %d has no title", i)
		}
		if st.Detail == "" {
			t.Fatalf("step %d has no detail", i)
		}
		if st.Done {
			t.Fatalf("step %d is marked done before anything has been observed", i)
		}
	}
}

func TestTour_NextStepAndComplete(t *testing.T) {
	tests := []struct {
		name         string
		done         []bool
		wantNext     int
		wantComplete bool
	}{
		{name: "nothing done", done: []bool{false, false, false, false}, wantNext: 0},
		{name: "first done", done: []bool{true, false, false, false}, wantNext: 1},
		{name: "all done", done: []bool{true, true, true, true}, wantNext: 4, wantComplete: true},
		{name: "a later step done out of order still points at the gap", done: []bool{false, true, false, false}, wantNext: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			steps := TourSteps()
			for i := range steps {
				steps[i].Done = tc.done[i]
			}
			tour := Tour{Steps: steps}
			if got := tour.NextStep(); got != tc.wantNext {
				t.Fatalf("NextStep = %d, want %d", got, tc.wantNext)
			}
			if got := tour.Complete(); got != tc.wantComplete {
				t.Fatalf("Complete = %v, want %v", got, tc.wantComplete)
			}
		})
	}
}

func TestLoadTour_EmptyHomeHasNothingDone(t *testing.T) {
	t.Setenv("FESTIVAL_HOME", t.TempDir())
	tour, err := LoadTour(context.Background())
	if err != nil {
		t.Fatalf("LoadTour: %v", err)
	}
	if len(tour.Steps) != 4 {
		t.Fatalf("len(Steps) = %d, want 4", len(tour.Steps))
	}
	if tour.NextStep() != 0 {
		t.Fatalf("NextStep = %d, want 0 on a home the hub has never written", tour.NextStep())
	}
	if tour.Dismissed {
		t.Fatal("a fresh home must not report the tour as dismissed")
	}
}

// TestLoadTour_ProgressSurvivesARestart is the resumption guarantee: the state
// a second LoadTour sees is the state the first one recorded, with no process
// memory in between.
func TestLoadTour_ProgressSurvivesARestart(t *testing.T) {
	ctx := context.Background()
	t.Setenv("FESTIVAL_HOME", t.TempDir())

	if err := RecordTourStep(ctx, 1); err != nil {
		t.Fatalf("RecordTourStep: %v", err)
	}
	tour, err := LoadTour(ctx)
	if err != nil {
		t.Fatalf("LoadTour: %v", err)
	}
	if !tour.Steps[0].Done || !tour.Steps[1].Done {
		t.Fatal("steps 1 and 2 must be done after recording step index 1")
	}
	if tour.Steps[2].Done {
		t.Fatal("step 3 must not be done")
	}
	if got := tour.NextStep(); got != 2 {
		t.Fatalf("NextStep = %d, want 2", got)
	}
}

func TestRecordTourStep_NeverMovesBackwards(t *testing.T) {
	ctx := context.Background()
	t.Setenv("FESTIVAL_HOME", t.TempDir())

	if err := RecordTourStep(ctx, 2); err != nil {
		t.Fatalf("RecordTourStep(2): %v", err)
	}
	if err := RecordTourStep(ctx, 0); err != nil {
		t.Fatalf("RecordTourStep(0): %v", err)
	}
	tour, err := LoadTour(ctx)
	if err != nil {
		t.Fatalf("LoadTour: %v", err)
	}
	if got := tour.NextStep(); got != 3 {
		t.Fatalf("NextStep = %d, want 3: an earlier step must not undo progress", got)
	}
}

func TestLoadTour_CorruptProgressReadsAsNoProgress(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		value string
	}{
		{name: "not a number", value: "halfway"},
		{name: "negative", value: "-3"},
		{name: "empty", value: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("FESTIVAL_HOME", t.TempDir())
			if err := SetHubState(ctx, state.HubStateTourStep, tc.value); err != nil {
				t.Fatalf("SetHubState: %v", err)
			}
			tour, err := LoadTour(ctx)
			if err != nil {
				t.Fatalf("LoadTour: %v", err)
			}
			if got := tour.NextStep(); got != 0 {
				t.Fatalf("NextStep = %d, want 0: a corrupt row must not strand the tour", got)
			}
		})
	}
}

func TestLoadTour_ProgressPastTheEndMarksEverythingDone(t *testing.T) {
	ctx := context.Background()
	t.Setenv("FESTIVAL_HOME", t.TempDir())
	if err := SetHubState(ctx, state.HubStateTourStep, strconv.Itoa(99)); err != nil {
		t.Fatalf("SetHubState: %v", err)
	}
	tour, err := LoadTour(ctx)
	if err != nil {
		t.Fatalf("LoadTour: %v", err)
	}
	if !tour.Complete() {
		t.Fatal("a stored step past the end must read as a finished tour, not a panic")
	}
}

func TestLoadTour_ReadsTheDismissedFlag(t *testing.T) {
	ctx := context.Background()
	t.Setenv("FESTIVAL_HOME", t.TempDir())
	if err := SetHubState(ctx, state.HubStateTourDismissed, "true"); err != nil {
		t.Fatalf("SetHubState: %v", err)
	}
	tour, err := LoadTour(ctx)
	if err != nil {
		t.Fatalf("LoadTour: %v", err)
	}
	if !tour.Dismissed {
		t.Fatal("Dismissed must be true after the flag is written")
	}
}
