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
	if nm2.screen != screenLaunchpad {
		t.Fatalf("digit 9 screen=%v want launchpad", nm2.screen)
	}
}
