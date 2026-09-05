package app

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/state"
)

// tourHome points every home-shaped environment variable at throwaway
// directories and empties PATH, so a test observes only what it sets up and
// never the developer's own machine.
func tourHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	t.Setenv("OBEY_INSTALLER_HOME", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("PATH", t.TempDir())
	return home
}

// fakeCamp puts a camp on PATH that prints body and exits with code. It uses
// echo rather than cat because these tests empty PATH, so a script that shells
// out to a coreutil would silently print nothing and look like an empty list.
func fakeCamp(t *testing.T, body string, code int) {
	t.Helper()
	if strings.Contains(body, "'") {
		t.Fatalf("fakeCamp body cannot contain a single quote: %q", body)
	}
	dir := t.TempDir()
	script := "#!/bin/sh\necho '" + body + "'\nexit " + strconv.Itoa(code) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "camp"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake camp: %v", err)
	}
	t.Setenv("PATH", dir)
}

func TestLoadTour_Errors(t *testing.T) {
	t.Run("cancelled context is reported", func(t *testing.T) {
		tourHome(t)
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

	t.Run("an unknown step key is refused", func(t *testing.T) {
		tourHome(t)
		for _, fn := range []func(context.Context, TourStepKey) error{RecordTourStepDone, SkipTourStep} {
			if err := fn(context.Background(), TourStepKey("not-a-step")); err == nil {
				t.Fatal("expected an error for an unknown step key")
			} else if code := errpkg.Code(err); code != "E_TOUR_STEP" {
				t.Fatalf("code = %q, want E_TOUR_STEP", code)
			}
		}
	})
}

func TestTourSteps_AreFourCompiledInStepsWithStableKeys(t *testing.T) {
	steps := TourSteps()
	if len(steps) != 4 {
		t.Fatalf("len(TourSteps) = %d, want 4", len(steps))
	}
	wantKeys := []TourStepKey{TourStepInstall, TourStepPath, TourStepCampInit, TourStepFestNext}
	seen := map[TourStepKey]bool{}
	for i, st := range steps {
		if st.Key != wantKeys[i] {
			t.Fatalf("step %d key = %q, want %q", i, st.Key, wantKeys[i])
		}
		if seen[st.Key] {
			t.Fatalf("duplicate step key %q", st.Key)
		}
		seen[st.Key] = true
		if st.Title == "" || st.Detail == "" {
			t.Fatalf("step %q is missing title or detail", st.Key)
		}
		if st.State != TourStepTodo {
			t.Fatalf("step %q starts as %q, want todo", st.Key, st.State)
		}
	}
}

// TestTourStepFestNext_SaysWhatDoneMeans pins the promise made to a user who
// ran fest next outside the hub and wonders why the step is still open.
func TestTourStepFestNext_SaysWhatDoneMeans(t *testing.T) {
	steps := TourSteps()
	last := steps[len(steps)-1]
	if last.Key != TourStepFestNext {
		t.Fatalf("last step is %q, want %q", last.Key, TourStepFestNext)
	}
	if !strings.Contains(last.Detail, "launched") {
		t.Fatalf("step detail must say completion means launching from here, got %q", last.Detail)
	}
}

func TestTour_NextStepAndComplete(t *testing.T) {
	tests := []struct {
		name         string
		states       []TourStepState
		wantNext     int
		wantComplete bool
	}{
		{
			name:     "nothing done",
			states:   []TourStepState{TourStepTodo, TourStepTodo, TourStepTodo, TourStepTodo},
			wantNext: 0,
		},
		{
			name:     "first done",
			states:   []TourStepState{TourStepDone, TourStepTodo, TourStepTodo, TourStepTodo},
			wantNext: 1,
		},
		{
			name:     "a pending step is not the next thing to do",
			states:   []TourStepState{TourStepDone, TourStepPending, TourStepTodo, TourStepTodo},
			wantNext: 2,
		},
		{
			name:     "a skipped step is passed over",
			states:   []TourStepState{TourStepDone, TourStepSkipped, TourStepTodo, TourStepTodo},
			wantNext: 2,
		},
		{
			name:         "all done",
			states:       []TourStepState{TourStepDone, TourStepDone, TourStepDone, TourStepDone},
			wantNext:     4,
			wantComplete: true,
		},
		{
			name:     "a skipped step means the tour is not complete",
			states:   []TourStepState{TourStepDone, TourStepDone, TourStepDone, TourStepSkipped},
			wantNext: 4,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			steps := TourSteps()
			for i := range steps {
				steps[i].State = tc.states[i]
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
	tourHome(t)
	tour, err := LoadTour(context.Background())
	if err != nil {
		t.Fatalf("LoadTour: %v", err)
	}
	if len(tour.Steps) != 4 {
		t.Fatalf("len(Steps) = %d, want 4", len(tour.Steps))
	}
	for _, st := range tour.Steps {
		if st.State != TourStepTodo {
			t.Fatalf("step %q is %q on an empty home, want todo", st.Key, st.State)
		}
	}
	if tour.Dismissed {
		t.Fatal("a fresh home must not report the tour as dismissed")
	}
}

func TestLoadTour_CampStepAsksCamp(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		exit     int
		absent   bool
		wantDone bool
		wantNote string
	}{
		{name: "camp reports a camp", body: `[{"name":"demo","path":"/tmp/demo"}]`, wantDone: true},
		{name: "camp reports none", body: "[]"},
		{name: "camp reports null", body: "null"},
		{name: "camp reports nothing at all", body: ""},
		{name: "camp fails", body: "[]", exit: 1},
		{name: "camp answers with something unreadable", body: "not json at all"},
		{name: "camp is not installed", absent: true, wantNote: "camp is not installed yet, finish step 1 first"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tourHome(t)
			if !tc.absent {
				fakeCamp(t, tc.body, tc.exit)
			}
			tour, err := LoadTour(context.Background())
			if err != nil {
				t.Fatalf("LoadTour: %v", err)
			}
			step, ok := tour.Step(TourStepCampInit)
			if !ok {
				t.Fatal("tour has no camp step")
			}
			if got := step.Done(); got != tc.wantDone {
				t.Fatalf("camp step done = %v, want %v", got, tc.wantDone)
			}
			if step.Note != tc.wantNote {
				t.Fatalf("camp step note = %q, want %q", step.Note, tc.wantNote)
			}
		})
	}
}

// TestLoadTour_PathStepPendingWhenRCWritten covers the case the task called
// out: the rc file has the block but this process's PATH predates it.
func TestLoadTour_PathStepPendingWhenRCWritten(t *testing.T) {
	ctx := context.Background()
	tourHome(t)

	plan, err := PlanShellRCAppend(ctx, "zsh")
	if err != nil {
		t.Fatalf("PlanShellRCAppend: %v", err)
	}
	if err := os.WriteFile(plan.File, []byte(plan.Block), 0o644); err != nil {
		t.Fatalf("write rc file: %v", err)
	}

	tour, err := LoadTour(ctx)
	if err != nil {
		t.Fatalf("LoadTour: %v", err)
	}
	step, _ := tour.Step(TourStepPath)
	if step.State != TourStepPending {
		t.Fatalf("path step = %q, want pending", step.State)
	}
	if step.Done() {
		t.Fatal("a written but inactive PATH line must not report as done")
	}
	if !strings.Contains(step.Note, plan.File) {
		t.Fatalf("note %q must name the rc file", step.Note)
	}
}

func TestLoadTour_PathStepDoneWhenManagedBinIsOnPath(t *testing.T) {
	ctx := context.Background()
	home := tourHome(t)
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	t.Setenv("PATH", bin)

	tour, err := LoadTour(ctx)
	if err != nil {
		t.Fatalf("LoadTour: %v", err)
	}
	step, _ := tour.Step(TourStepPath)
	if step.State != TourStepDone {
		t.Fatalf("path step = %q, want done", step.State)
	}
}

// TestLoadTour_AgreesWithTheHomeSetupCard pins that the tour's first two steps
// and Ring 1's setup card are the same two facts, read from one SetupState.
func TestLoadTour_AgreesWithTheHomeSetupCard(t *testing.T) {
	ctx := context.Background()
	home := tourHome(t)
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	t.Setenv("PATH", bin)

	setup, err := ResolveSetupState(ctx)
	if err != nil {
		t.Fatalf("ResolveSetupState: %v", err)
	}
	tour, err := LoadTour(ctx)
	if err != nil {
		t.Fatalf("LoadTour: %v", err)
	}
	install, _ := tour.Step(TourStepInstall)
	path, _ := tour.Step(TourStepPath)
	if install.Done() != setup.HasReceipts {
		t.Fatalf("install step done = %v, setup card HasReceipts = %v", install.Done(), setup.HasReceipts)
	}
	if path.Done() != setup.ManagedBinOnPath {
		t.Fatalf("path step done = %v, setup card ManagedBinOnPath = %v", path.Done(), setup.ManagedBinOnPath)
	}
}

// TestLoadTour_ProgressSurvivesARestart is the resumption guarantee: what a
// second LoadTour sees is what the first one recorded, with no process memory
// in between.
func TestLoadTour_ProgressSurvivesARestart(t *testing.T) {
	ctx := context.Background()
	tourHome(t)

	if err := RecordTourStepDone(ctx, TourStepFestNext); err != nil {
		t.Fatalf("RecordTourStepDone: %v", err)
	}
	tour, err := LoadTour(ctx)
	if err != nil {
		t.Fatalf("LoadTour: %v", err)
	}
	step, _ := tour.Step(TourStepFestNext)
	if !step.Done() {
		t.Fatal("the recorded step must be done after a reload")
	}
	if other, _ := tour.Step(TourStepInstall); other.Done() {
		t.Fatal("recording one step must not mark another done")
	}
}

func TestSkipTourStep_IsNeverReportedAsDone(t *testing.T) {
	ctx := context.Background()
	tourHome(t)

	if err := SkipTourStep(ctx, TourStepFestNext); err != nil {
		t.Fatalf("SkipTourStep: %v", err)
	}
	tour, err := LoadTour(ctx)
	if err != nil {
		t.Fatalf("LoadTour: %v", err)
	}
	step, _ := tour.Step(TourStepFestNext)
	if step.State != TourStepSkipped {
		t.Fatalf("step state = %q, want skipped", step.State)
	}
	if step.Done() {
		t.Fatal("a skipped step must never report as done")
	}
	if tour.Complete() {
		t.Fatal("a tour with a skipped step is not complete")
	}
}

// TestLoadTour_ObservationBeatsASkipRecord: skipping a step and then doing it
// anyway shows the truth, not the old answer.
func TestLoadTour_ObservationBeatsASkipRecord(t *testing.T) {
	ctx := context.Background()
	tourHome(t)
	if err := SkipTourStep(ctx, TourStepCampInit); err != nil {
		t.Fatalf("SkipTourStep: %v", err)
	}
	fakeCamp(t, `[{"name":"demo","path":"/tmp/demo"}]`, 0)

	tour, err := LoadTour(ctx)
	if err != nil {
		t.Fatalf("LoadTour: %v", err)
	}
	step, _ := tour.Step(TourStepCampInit)
	if step.State != TourStepDone {
		t.Fatalf("step state = %q, want done: observation must beat a stale skip", step.State)
	}
}

func TestDismissTour(t *testing.T) {
	ctx := context.Background()
	tourHome(t)

	tour, err := LoadTour(ctx)
	if err != nil {
		t.Fatalf("LoadTour: %v", err)
	}
	if tour.Dismissed {
		t.Fatal("a fresh tour is not dismissed")
	}
	if err := DismissTour(ctx); err != nil {
		t.Fatalf("DismissTour: %v", err)
	}
	tour, err = LoadTour(ctx)
	if err != nil {
		t.Fatalf("LoadTour after dismiss: %v", err)
	}
	if !tour.Dismissed {
		t.Fatal("Dismissed must survive a reload")
	}
}

func TestLoadTour_UnreadableRecordReadsAsTodo(t *testing.T) {
	ctx := context.Background()
	tourHome(t)
	if err := SetHubState(ctx, state.HubStateTourStepKey(string(TourStepFestNext)), "half-done"); err != nil {
		t.Fatalf("SetHubState: %v", err)
	}
	tour, err := LoadTour(ctx)
	if err != nil {
		t.Fatalf("LoadTour: %v", err)
	}
	step, _ := tour.Step(TourStepFestNext)
	if step.State != TourStepTodo {
		t.Fatalf("step state = %q, want todo: an unreadable record must not strand the tour", step.State)
	}
}

func TestTourStepKeysAreStableStrings(t *testing.T) {
	want := map[TourStepKey]string{
		TourStepInstall:  "install-suite",
		TourStepPath:     "path",
		TourStepCampInit: "camp-init",
		TourStepFestNext: "fest-next",
	}
	for key, str := range want {
		if string(key) != str {
			t.Fatalf("step key %q changed to %q: stored progress would be orphaned", str, key)
		}
		if got := state.HubStateTourStepKey(str); got != "tour.step."+str {
			t.Fatalf("hub_state key = %q, want tour.step.%s", got, str)
		}
	}
}

// TestSkipTourStep_WorksForEveryStep covers the finding that skip tests only
// exercised the one step with no observable signal, so a skip that never took
// effect on the other three would have passed unnoticed.
func TestSkipTourStep_WorksForEveryStep(t *testing.T) {
	for _, step := range TourSteps() {
		t.Run(string(step.Key), func(t *testing.T) {
			ctx := context.Background()
			tourHome(t)
			if err := SkipTourStep(ctx, step.Key); err != nil {
				t.Fatalf("SkipTourStep: %v", err)
			}
			tour, err := LoadTour(ctx)
			if err != nil {
				t.Fatalf("LoadTour: %v", err)
			}
			got, _ := tour.Step(step.Key)
			if got.State != TourStepSkipped {
				t.Fatalf("step %q is %q after a skip, want skipped", step.Key, got.State)
			}
			if got.Done() {
				t.Fatalf("step %q reported done after a skip", step.Key)
			}
		})
	}
}

// TestSkipTourStep_BeatsAPendingObservation is the PATH case: the rc line is
// written but this shell's PATH predates it, and the user says skip. Before the
// fix the skip was silently discarded on the next load.
func TestSkipTourStep_BeatsAPendingObservation(t *testing.T) {
	ctx := context.Background()
	tourHome(t)

	plan, err := PlanShellRCAppend(ctx, "zsh")
	if err != nil {
		t.Fatalf("PlanShellRCAppend: %v", err)
	}
	if err := os.WriteFile(plan.File, []byte(plan.Block), 0o644); err != nil {
		t.Fatalf("write rc file: %v", err)
	}

	before, err := LoadTour(ctx)
	if err != nil {
		t.Fatalf("LoadTour: %v", err)
	}
	if step, _ := before.Step(TourStepPath); step.State != TourStepPending {
		t.Fatalf("precondition: path step is %q, want pending", step.State)
	}

	if err := SkipTourStep(ctx, TourStepPath); err != nil {
		t.Fatalf("SkipTourStep: %v", err)
	}
	after, err := LoadTour(ctx)
	if err != nil {
		t.Fatalf("LoadTour: %v", err)
	}
	if step, _ := after.Step(TourStepPath); step.State != TourStepSkipped {
		t.Fatalf("path step is %q after a skip, want skipped", step.State)
	}
}

// TestLoadTour_ObservedDoneBeatsASkipForEveryObservableStep: a skip is the
// user's answer, but it must never outlive the thing being true.
func TestLoadTour_ObservedDoneBeatsASkipForEveryObservableStep(t *testing.T) {
	t.Run("path", func(t *testing.T) {
		ctx := context.Background()
		home := tourHome(t)
		bin := filepath.Join(home, "bin")
		if err := os.MkdirAll(bin, 0o755); err != nil {
			t.Fatalf("mkdir bin: %v", err)
		}
		if err := SkipTourStep(ctx, TourStepPath); err != nil {
			t.Fatalf("SkipTourStep: %v", err)
		}
		t.Setenv("PATH", bin)

		tour, err := LoadTour(ctx)
		if err != nil {
			t.Fatalf("LoadTour: %v", err)
		}
		if step, _ := tour.Step(TourStepPath); step.State != TourStepDone {
			t.Fatalf("path step is %q, want done", step.State)
		}
	})

	t.Run("camp", func(t *testing.T) {
		ctx := context.Background()
		tourHome(t)
		if err := SkipTourStep(ctx, TourStepCampInit); err != nil {
			t.Fatalf("SkipTourStep: %v", err)
		}
		fakeCamp(t, `[{"name":"demo","path":"/tmp/demo"}]`, 0)

		tour, err := LoadTour(ctx)
		if err != nil {
			t.Fatalf("LoadTour: %v", err)
		}
		if step, _ := tour.Step(TourStepCampInit); step.State != TourStepDone {
			t.Fatalf("camp step is %q, want done", step.State)
		}
	})
}

func TestHubStateValues_ReadsEveryKeyInOneOpen(t *testing.T) {
	ctx := context.Background()

	t.Run("an empty home returns no values and no error", func(t *testing.T) {
		tourHome(t)
		got, err := HubStateValues(ctx, "tour.step.path", "tour.dismissed")
		if err != nil {
			t.Fatalf("HubStateValues: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("got %v, want nothing on a home that was never written", got)
		}
	})

	t.Run("no keys is not an error", func(t *testing.T) {
		tourHome(t)
		got, err := HubStateValues(ctx)
		if err != nil || len(got) != 0 {
			t.Fatalf("HubStateValues() = (%v, %v), want an empty result and no error", got, err)
		}
	})

	t.Run("a cancelled context is reported", func(t *testing.T) {
		tourHome(t)
		cancelled, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := HubStateValues(cancelled, "tour.dismissed"); err == nil {
			t.Fatal("expected an error with a cancelled context")
		}
	})

	t.Run("written keys come back and missing ones stay absent", func(t *testing.T) {
		tourHome(t)
		if err := SetHubState(ctx, "tour.step.path", "skipped"); err != nil {
			t.Fatalf("SetHubState: %v", err)
		}
		got, err := HubStateValues(ctx, "tour.step.path", "tour.step.fest-next")
		if err != nil {
			t.Fatalf("HubStateValues: %v", err)
		}
		if got["tour.step.path"] != "skipped" {
			t.Fatalf("path = %q, want skipped", got["tour.step.path"])
		}
		if _, present := got["tour.step.fest-next"]; present {
			t.Fatal("a key that was never written must be absent, not empty")
		}
	})
}
