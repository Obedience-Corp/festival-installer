package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

func tourModel(t *testing.T, states ...app.TourStepState) model {
	t.Helper()
	m := newModel(Options{Version: "test"})
	m.reduced = true
	m.screen = screenTour
	m.tour = app.Tour{Steps: tourStepsWith(states...)}
	return m
}

// tourStepsWith builds the four compiled-in steps with the given states applied
// in order; anything not named stays todo.
func tourStepsWith(states ...app.TourStepState) []app.TourStep {
	steps := app.TourSteps()
	for i := range steps {
		if i < len(states) && states[i] != "" {
			steps[i].State = states[i]
		}
	}
	return steps
}

func TestTour_DigitOneOpensTheTour(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.screen = screenHome
	m.cursor = 0

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	nm := next.(model)
	if nm.screen != screenTour {
		t.Fatalf("screen = %v, want screenTour", nm.screen)
	}
	if cmd == nil {
		t.Fatal("opening the tour must load its state through a command, not inside View")
	}
}

func TestTour_EscapeReturnsHome(t *testing.T) {
	m := tourModel(t)
	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if got := next.(model).screen; got != screenHome {
		t.Fatalf("screen after esc = %v, want screenHome", got)
	}
}

func TestTour_MaxCursorBounds(t *testing.T) {
	tests := []struct {
		name  string
		steps []app.TourStep
		want  int
	}{
		{name: "four steps", steps: app.TourSteps(), want: 3},
		{name: "no steps loaded yet", steps: nil, want: 0},
		{name: "one step", steps: app.TourSteps()[:1], want: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newModel(Options{Version: "test"})
			m.screen = screenTour
			m.tour = app.Tour{Steps: tc.steps}
			if got := m.maxCursor(); got != tc.want {
				t.Fatalf("maxCursor = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestTour_CursorCannotLeaveTheStepList(t *testing.T) {
	m := tourModel(t)
	for i := 0; i < 20; i++ {
		next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		m = next.(model)
	}
	if got, want := m.cursor, len(m.tour.Steps)-1; got != want {
		t.Fatalf("cursor after paging down = %d, want %d", got, want)
	}
	for i := 0; i < 20; i++ {
		next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		m = next.(model)
	}
	if m.cursor != 0 {
		t.Fatalf("cursor after paging up = %d, want 0", m.cursor)
	}
}

func TestTour_MessageParksCursorOnTheNextStep(t *testing.T) {
	tests := []struct {
		name   string
		states []app.TourStepState
		want   int
	}{
		{name: "nothing done", want: 0},
		{name: "first two done", states: []app.TourStepState{app.TourStepDone, app.TourStepDone}, want: 2},
		{
			name:   "everything done parks on the last step",
			states: []app.TourStepState{app.TourStepDone, app.TourStepDone, app.TourStepDone, app.TourStepDone},
			want:   3,
		},
		{
			name:   "a skipped step is passed over",
			states: []app.TourStepState{app.TourStepDone, app.TourStepSkipped},
			want:   2,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newModel(Options{Version: "test"})
			m.screen = screenTour
			next, _ := m.Update(tourMsg{tour: app.Tour{Steps: tourStepsWith(tc.states...)}})
			if got := next.(model).cursor; got != tc.want {
				t.Fatalf("cursor = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestTour_MessageDoesNotMoveTheCursorOnOtherScreens(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.screen = screenHome
	m.cursor = 5
	next, _ := m.Update(tourMsg{tour: app.Tour{Steps: tourStepsWith(app.TourStepDone)}})
	if got := next.(model).cursor; got != 5 {
		t.Fatalf("home cursor = %d, want 5: a background tour load must not move it", got)
	}
}

func TestTour_ViewShowsEveryStepAndMarksProgress(t *testing.T) {
	m := tourModel(t, app.TourStepDone, app.TourStepDone)
	m.width, m.height = 80, 24
	out := m.View()

	for _, st := range app.TourSteps() {
		if !strings.Contains(out, st.Title) {
			t.Fatalf("step %q missing from the tour view", st.Title)
		}
	}
	if !strings.Contains(out, "[x]") {
		t.Fatal("a finished step must be marked done")
	}
	if !strings.Contains(out, "[ ]") {
		t.Fatal("an unfinished step must be marked not done")
	}
	if !strings.Contains(out, "next") {
		t.Fatal("the view must point at the next step")
	}
	if !strings.Contains(out, "getting started") {
		t.Fatal("the header must name the screen")
	}
}

func TestTour_ViewFitsEightyColumns(t *testing.T) {
	for _, width := range []int{80, 120} {
		m := tourModel(t, app.TourStepDone)
		m.width, m.height = width, 24
		for _, line := range strings.Split(m.View(), "\n") {
			if got := lipgloss.Width(line); got > width {
				t.Fatalf("at %d columns a line is %d wide: %q", width, got, line)
			}
		}
	}
}

func TestTour_ViewWithNoStepsDoesNotPanic(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.reduced = true
	m.screen = screenTour
	m.width, m.height = 80, 24
	if out := m.View(); out == "" {
		t.Fatal("the tour view must render something before its state arrives")
	}
}

// TestTour_InitLoadsTheTourWhenResumingOntoIt is the regression test for a
// resume that came back with an empty step list after a child ran.
func TestTour_InitLoadsTheTourWhenResumingOntoIt(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.screen = screenTour
	if m.Init() == nil {
		t.Fatal("Init must return commands")
	}

	home := newModel(Options{Version: "test"})
	home.screen = screenHome
	if home.Init() == nil {
		t.Fatal("Init must return commands on home too")
	}
}

func TestTour_AFailedLoadKeepsTheStepsAlreadyOnScreen(t *testing.T) {
	m := tourModel(t, app.TourStepDone)
	before := len(m.tour.Steps)

	next, _ := m.Update(tourMsg{err: errTourTest})
	nm := next.(model)
	if got := len(nm.tour.Steps); got != before {
		t.Fatalf("steps after a failed load = %d, want %d", got, before)
	}
	if nm.err == nil {
		t.Fatal("the error must still be surfaced")
	}
	if step, _ := nm.tour.Step(app.TourStepInstall); !step.Done() {
		t.Fatal("a failed load must not undo a step already shown as done")
	}
}

var errTourTest = errpkg.New("E_TEST", "tour load failed")

// TestTour_LongTextWrapsInsteadOfClipping: the step list is the only text on
// this screen, so losing the end of a line loses the instruction.
func TestTour_LongTextWrapsInsteadOfClipping(t *testing.T) {
	const note = "written to /a/very/long/path/that/keeps/going/and/going/for/a/while/.zshrc, restart your shell"
	for _, width := range []int{60, 80, 120} {
		m := tourModel(t)
		m.width, m.height = width, 40
		m.tour.Steps[1].Note = note
		out := m.View()
		for _, line := range strings.Split(out, "\n") {
			if lipgloss.Width(line) > width {
				t.Fatalf("at %d columns a line is %d wide: %q", width, lipgloss.Width(line), line)
			}
		}
		flat := strings.Join(strings.Fields(stripLines(out)), " ")
		if !strings.Contains(flat, "restart your shell") {
			t.Fatalf("at %d columns the end of the note was lost:\n%s", width, out)
		}
	}
}

// stripLines joins wrapped output so a test can look for text that the renderer
// may have split across lines.
func stripLines(s string) string { return strings.ReplaceAll(s, "\n", " ") }

// TestTour_ShowsTheChildReturnBanner: pressing enter on a launch step and
// getting nothing back is the failure mode this guards against.
func TestTour_ShowsTheChildReturnBanner(t *testing.T) {
	m := tourModel(t)
	m.width, m.height = 80, 30
	m.banner = "returned from fest next (exit 1)"
	if out := m.View(); !strings.Contains(out, "returned from fest next") {
		t.Fatalf("the tour must show what happened to the child:\n%s", out)
	}
}

func TestTour_UnknownSignalsAreSaidOutLoud(t *testing.T) {
	m := tourModel(t)
	m.width, m.height = 80, 30

	if out := m.View(); strings.Contains(out, "could not read the installer home") {
		t.Fatal("a readable home must not carry the warning")
	}

	m.tour.SignalsUnknown = true
	out := m.View()
	if !strings.Contains(out, "could not read the installer home") {
		t.Fatalf("an unreadable home must say so on the tour:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if lipgloss.Width(line) > 80 {
			t.Fatalf("the warning pushed a line to %d columns: %q", lipgloss.Width(line), line)
		}
	}
	for _, st := range app.TourSteps() {
		if !strings.Contains(out, st.Title) {
			t.Fatalf("step %q disappeared when signals were unknown", st.Title)
		}
	}
}
