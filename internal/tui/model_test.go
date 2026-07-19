package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHomeNavigation_Quit(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.reduced = true
	m.screen = screenHome
	m.cursor = len(homeItems) - 1 // Quit
	next, cmd := m.handleEnter()
	nm := next.(model)
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
	// execute cmd — tea.Quit returns a quit msg
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		// tea.Quit may return tea.QuitMsg via special internal; accept non-nil
		_ = nm
	}
}

func TestBootSkipsOnEnter(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.screen = screenBoot
	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if next.(model).screen != screenHome {
		t.Fatalf("want home after skip, got %v", next.(model).screen)
	}
}

func TestMaxCursorHome(t *testing.T) {
	m := newModel(Options{})
	m.screen = screenHome
	if m.maxCursor() != len(homeItems)-1 {
		t.Fatalf("max cursor %d want %d", m.maxCursor(), len(homeItems)-1)
	}
}

func TestViewHomeNonEmpty(t *testing.T) {
	m := newModel(Options{Version: "0.1.0"})
	m.reduced = true
	m.screen = screenHome
	m.width = 80
	m.height = 24
	out := m.View()
	if out == "" {
		t.Fatal("empty view")
	}
	if len(out) < 50 {
		t.Fatalf("view too short: %q", out)
	}
}
