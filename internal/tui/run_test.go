package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Obedience-Corp/festival-installer/internal/launch"
)

func TestFormatChildBanner(t *testing.T) {
	spec := launch.Spec{Tool: "camp", Title: "camp wi"}
	tests := []struct {
		name string
		res  launch.Result
		want string
	}{
		{
			name: "success",
			res:  launch.Result{Started: true, ExitCode: 0},
			want: "returned from camp wi, back in festival hub",
		},
		{
			name: "exit 3",
			res:  launch.Result{Started: true, ExitCode: 3, Err: errors.New("exit 3")},
			want: "returned from camp wi (exit 3)",
		},
		{
			name: "signal interrupt",
			res:  launch.Result{Started: true, ExitCode: 130, Signal: "interrupt", Err: errors.New("signal: interrupt")},
			want: "returned from camp wi (interrupt), back in festival hub",
		},
		{
			name: "resolve failure",
			res:  launch.Result{Started: false, ExitCode: -1, Err: errors.New("not found")},
			want: "could not launch camp wi: not found",
		},
		{
			name: "start failure no err",
			res:  launch.Result{Started: false, ExitCode: -1},
			want: "could not launch camp wi",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatChildBanner(spec, tc.res)
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
			if tc.res.Started && tc.res.Signal != "" && strings.Contains(got, "could not launch") {
				t.Fatalf("signalled child presented as launch failure: %q", got)
			}
		})
	}
}

// TestChildBanner_SessionLineOnlySurvivesACleanExit pins when a session's own
// banner replaces the generic one. The tour uses it to say a festival was
// created and the step still has to be run; on a child that failed or was
// interrupted that sentence would be a claim about work that did not happen.
func TestChildBanner_SessionLineOnlySurvivesACleanExit(t *testing.T) {
	const line = "festival created, run this step again to start fest next in it"
	spec := launch.Spec{Tool: "fest", Title: "fest create festival"}
	tests := []struct {
		name    string
		session SessionResult
		res     launch.Result
		want    string
	}{
		{
			name:    "clean exit keeps the session line",
			session: SessionResult{Banner: line},
			res:     launch.Result{Started: true, ExitCode: 0},
			want:    line,
		},
		{
			name:    "a non-zero exit falls back to the generic line",
			session: SessionResult{Banner: line},
			res:     launch.Result{Started: true, ExitCode: 1, Err: errors.New("exit 1")},
			want:    "returned from fest create festival (exit 1)",
		},
		{
			name:    "an interrupted child falls back to the generic line",
			session: SessionResult{Banner: line},
			res:     launch.Result{Started: true, ExitCode: 130, Signal: "interrupt"},
			want:    "returned from fest create festival (interrupt), back in festival hub",
		},
		{
			name:    "a child that never started falls back to the generic line",
			session: SessionResult{Banner: line},
			res:     launch.Result{Started: false, ExitCode: -1},
			want:    "could not launch fest create festival",
		},
		{
			name:    "an ordinary launch keeps the generic line",
			session: SessionResult{},
			res:     launch.Result{Started: true, ExitCode: 0},
			want:    "returned from fest create festival, back in festival hub",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := childBanner(tc.session, spec, tc.res); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

// TestCleanExit is the one definition of "the child did its job", used both to
// decide whether a chained second child runs and whether a tour step ticks. A
// signalled child is deliberately not clean: Ctrl+C out of a tool means the
// user stopped it, not that it succeeded.
func TestCleanExit(t *testing.T) {
	tests := []struct {
		name string
		res  launch.Result
		want bool
	}{
		{"started and returned zero", launch.Result{Started: true, ExitCode: 0}, true},
		{"non-zero exit", launch.Result{Started: true, ExitCode: 1}, false},
		{"interrupted", launch.Result{Started: true, ExitCode: 130, Signal: "interrupt"}, false},
		{"never started", launch.Result{Started: false, ExitCode: -1}, false},
		{"never started but zero", launch.Result{Started: false, ExitCode: 0}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := cleanExit(tc.res); got != tc.want {
				t.Fatalf("cleanExit = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestHomeDigitZeroQuits(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.reduced = true
	m.screen = screenHome
	m.cursor = 0
	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'0'}})
	if cmd == nil {
		t.Fatal("expected quit cmd from digit 0")
	}
	_ = next

	m2 := newModel(Options{Version: "test"})
	m2.screen = screenHome
	next2, _ := m2.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'9'}})
	nm2 := next2.(model)
	if nm2.screen != screenShell {
		t.Fatalf("digit 9 screen=%v want shell", nm2.screen)
	}
}

// TestLaunchpadHasNoDigit records a consequence of putting Getting started
// first: the home menu now has eleven entries, digits 1 through 9 address the
// first nine, 0 stays Quit, and Launchpad is reached with the arrow keys. The
// alternative was leaving the tour off the home screen entirely.
func TestLaunchpadHasNoDigit(t *testing.T) {
	m := newModel(Options{Version: "test"})
	if got := m.homeIndexOf(homeLaunchpad); got < 9 {
		t.Fatalf("launchpad index = %d: it is inside the digit range, so this note is stale", got)
	}
	if got, want := m.homeIndexOf(homeTour), 0; got != want {
		t.Fatalf("Getting started index = %d, want %d (first)", got, want)
	}
	if got, want := m.homeIndexOf(homeQuit), len(m.homeMenu())-1; got != want {
		t.Fatalf("Quit index = %d, want %d (last, so digit 0 keeps its meaning)", got, want)
	}
}
