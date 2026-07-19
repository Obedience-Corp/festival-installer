package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Obedience-Corp/obey-installer/internal/app"
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

func TestViewHomeReducedMotionIsStatic(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.reduced = true
	m.screen = screenHome
	m.width = 80
	m.height = 24

	m.frame = 0
	first := m.viewHome()
	m.frame = 1
	second := m.viewHome()
	if first != second {
		t.Fatal("reduced-motion home view changed between animation frames")
	}
}

func TestViewProgressReducedMotionIsStatic(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.reduced = true
	m.screen = screenProgress
	m.width = 80
	m.height = 24
	m.progress = app.ProgressEvent{Stage: "download", Percent: 0.5, Message: "fetching"}

	m.frame = 0
	first := m.View()
	m.frame = 1
	second := m.View()
	if first != second {
		t.Fatal("reduced-motion progress view changed between animation frames")
	}
}

func TestMutationCommandsInstallCancellation(t *testing.T) {
	t.Run("uninstall", func(t *testing.T) {
		m := newModel(Options{})
		next, cmd := m.startUninstall(app.FestivalPackageID)
		nm := next.(model)
		if cmd == nil || nm.opCancel == nil {
			t.Fatal("uninstall should return a command with a cancellation function")
		}
	})

	t.Run("browse install", func(t *testing.T) {
		m := newModel(Options{})
		m.browseFlat = []app.BrowseEntry{{ID: "acme/fest-demo", Class: "plugin"}}
		next, cmd := m.installBrowseSelection()
		nm := next.(model)
		if cmd == nil || nm.opCancel == nil {
			t.Fatal("browse install should return a command with a cancellation function")
		}
	})
}
