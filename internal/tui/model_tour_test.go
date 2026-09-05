package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Obedience-Corp/festival-installer/internal/app"
)

func tourModel(t *testing.T, done ...bool) model {
	t.Helper()
	m := newModel(Options{Version: "test"})
	m.reduced = true
	m.screen = screenTour
	steps := app.TourSteps()
	for i := range steps {
		if i < len(done) {
			steps[i].Done = done[i]
		}
	}
	m.tour = app.Tour{Steps: steps}
	return m
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
		name string
		done []bool
		want int
	}{
		{name: "nothing done", want: 0},
		{name: "first two done", done: []bool{true, true}, want: 2},
		{name: "everything done parks on the last step", done: []bool{true, true, true, true}, want: 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			steps := app.TourSteps()
			for i := range steps {
				if i < len(tc.done) {
					steps[i].Done = tc.done[i]
				}
			}
			m := newModel(Options{Version: "test"})
			m.screen = screenTour
			next, _ := m.Update(tourMsg{tour: app.Tour{Steps: steps}})
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
	steps := app.TourSteps()
	steps[0].Done = true
	next, _ := m.Update(tourMsg{tour: app.Tour{Steps: steps}})
	if got := next.(model).cursor; got != 5 {
		t.Fatalf("home cursor = %d, want 5: a background tour load must not move it", got)
	}
}

func TestTour_ViewShowsEveryStepAndMarksProgress(t *testing.T) {
	m := tourModel(t, true, true)
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
		m := tourModel(t, true)
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
