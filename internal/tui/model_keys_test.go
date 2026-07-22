package tui

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// typeRunes feeds each rune of s as a separate key press through handleKey,
// threading the returned model forward the way bubbletea does.
func typeRunes(t *testing.T, m model, s string) model {
	t.Helper()
	var mod tea.Model = m
	for _, r := range s {
		mod, _ = mod.(model).handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return mod.(model)
}

func TestMarketplaceAddCapturesGlobalKeys(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.screen = screenMarketplace
	m.marketMode = "add"
	m.addInput.Focus()

	// URL contains both "q" (quit shortcut) and "?" (help shortcut).
	const url = "https://example.com/quarry.git?ref=x"
	nm := typeRunes(t, m, url)

	if got := nm.addInput.Value(); got != url {
		t.Fatalf("typed value = %q, want %q", got, url)
	}
	if nm.marketMode != "add" {
		t.Fatalf("marketMode = %q, want add (q/? must not exit the field)", nm.marketMode)
	}
	if nm.help {
		t.Fatal("? typed into the field must not toggle help")
	}
}

func TestQuestionKeyCapturedInAddMode(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.screen = screenMarketplace
	m.marketMode = "add"
	m.addInput.Focus()

	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	nm := next.(model)
	if nm.help {
		t.Fatal("? in add mode must type into the field, not toggle help")
	}
	if nm.addInput.Value() != "?" {
		t.Fatalf("addInput = %q, want %q", nm.addInput.Value(), "?")
	}
}

func TestMarketplaceAddEscCancels(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.screen = screenMarketplace
	m.marketMode = "add"
	m.addInput.Focus()
	m.addInput.SetValue("https://example.com/x.git")

	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if nm := next.(model); nm.marketMode != "list" {
		t.Fatalf("esc should cancel add mode, marketMode=%q", nm.marketMode)
	}
}

func TestMarketplaceAddEnterSubmits(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.screen = screenMarketplace
	m.marketMode = "add"
	m.addInput.Focus()
	m.addInput.SetValue("https://example.com/x.git")

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(model)
	if nm.marketMode != "list" {
		t.Fatalf("enter should submit and return to list, marketMode=%q", nm.marketMode)
	}
	if cmd == nil {
		t.Fatal("enter with a URL should return the marketplace-add command")
	}
}

func TestMarketplaceAddCtrlCQuits(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.screen = screenMarketplace
	m.marketMode = "add"
	m.addInput.Focus()

	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("ctrl+c in add mode should quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("ctrl+c in add mode should return a quit message")
	}
}

func TestHomeQKeyQuits(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.screen = screenHome

	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("q on home should return a quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("q on home should quit")
	}
}

func TestQuestionKeyTogglesHelpOnHome(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.screen = screenHome

	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if !next.(model).help {
		t.Fatal("? on home should toggle help on")
	}
}

func TestNewModelDefaultsContext(t *testing.T) {
	if newModel(Options{}).ctx == nil {
		t.Fatal("newModel must set a non-nil context")
	}
}

func TestOpContextDerivesFromModelContext(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	m := newModel(Options{})
	m.ctx = parent

	opCtx, cancelOp := m.opContext()
	defer cancelOp()

	select {
	case <-opCtx.Done():
		t.Fatal("op context should be live before the program context is cancelled")
	default:
	}

	cancelParent()
	select {
	case <-opCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("cancelling the program context must cancel the op context")
	}
}
