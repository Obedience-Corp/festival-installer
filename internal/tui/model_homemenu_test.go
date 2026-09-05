package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Obedience-Corp/festival-installer/internal/app"
)

// homeEntryScreens is the screen each home entry must open, keyed by identity
// rather than position. A new entry that is not listed here fails the tests
// below rather than shipping unaddressed.
var homeEntryScreens = map[homeItemID]screen{
	homeTour:        screenTour,
	homeInstall:     screenInstall,
	homeUpdate:      screenProgress,
	homeList:        screenList,
	homeBrowse:      screenBrowse,
	homeUninstall:   screenUninstall,
	homeMarketplace: screenMarketplace,
	homeDoctor:      screenDoctor,
	homeShell:       screenShell,
	homeLaunchpad:   screenLaunchpad,
}

// TestOpenHomeItem_EveryEntryReachesItsScreen is the regression test for menu
// order. It walks the menu by position, but asserts against the entry's id, so
// inserting or moving an entry cannot quietly point a row at the wrong screen.
func TestOpenHomeItem_EveryEntryReachesItsScreen(t *testing.T) {
	want := homeEntryScreens

	base := newModel(Options{Version: "test"})
	menu := base.homeMenu()
	if len(menu) == 0 {
		t.Fatal("home menu is empty")
	}

	for i, item := range menu {
		t.Run(string(item.id), func(t *testing.T) {
			m := newModel(Options{Version: "test"})
			m.screen = screenHome
			m.cursor = i

			next, cmd := m.openHomeItem()
			nm := next.(model)

			if item.id == homeQuit {
				if cmd == nil {
					t.Fatal("Quit must return a command")
				}
				return
			}
			target, known := want[item.id]
			if !known {
				t.Fatalf("menu entry %q has no expected screen: add it to this test", item.id)
			}
			if nm.screen != target {
				t.Fatalf("entry %q at index %d opened screen %v, want %v", item.id, i, nm.screen, target)
			}
		})
	}

	for id := range want {
		if base.homeIndexOf(id) < 0 {
			t.Fatalf("expected entry %q is missing from the home menu", id)
		}
	}
}

func TestHomeItemAt_BoundsAreGuarded(t *testing.T) {
	m := newModel(Options{Version: "test"})
	tests := []struct {
		name   string
		cursor int
	}{
		{name: "negative", cursor: -1},
		{name: "past the end", cursor: len(m.homeMenu())},
		{name: "far past the end", cursor: 999},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := m.homeItemAt(tc.cursor); ok {
				t.Fatalf("cursor %d must not resolve to an entry", tc.cursor)
			}
		})
	}
	if _, ok := m.homeItemAt(0); !ok {
		t.Fatal("cursor 0 must resolve to an entry")
	}
}

func TestHomeBoothIndex_TracksTheSelectedEntry(t *testing.T) {
	m := newModel(Options{Version: "test"})
	for i, item := range m.homeMenu() {
		m.cursor = i
		if got := m.homeBoothIndex(); got != item.booth {
			t.Fatalf("entry %q booth = %d, want %d", item.id, got, item.booth)
		}
	}
	m.cursor = 999
	if got := m.homeBoothIndex(); got != 0 {
		t.Fatalf("out of range cursor booth = %d, want 0", got)
	}
}

func TestDigitKeys_AddressTheFirstNineEntries(t *testing.T) {
	digits := []rune{'1', '2', '3', '4', '5', '6', '7', '8', '9'}
	base := newModel(Options{Version: "test"})
	menu := base.homeMenu()

	for i, d := range digits {
		if i >= len(menu) {
			break
		}
		t.Run(string(d), func(t *testing.T) {
			want, known := homeEntryScreens[menu[i].id]
			if !known {
				t.Fatalf("menu entry %q has no expected screen: add it to homeEntryScreens", menu[i].id)
			}
			m := newModel(Options{Version: "test"})
			m.screen = screenHome
			m.cursor = 0
			next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{d}})
			if got := next.(model).screen; got != want {
				t.Fatalf("digit %q opened screen %v, want %v (%s)", string(d), got, want, menu[i].id)
			}
		})
	}
}

func TestDigitZero_SelectsQuitWhereverItSits(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.screen = screenHome
	m.cursor = 0
	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'0'}})
	nm := next.(model)
	if got, want := nm.cursor, m.homeIndexOf(homeQuit); got != want {
		t.Fatalf("digit 0 cursor = %d, want %d (Quit)", got, want)
	}
}

func TestDefaultHomeCursor_PrefersUpdateByIdentity(t *testing.T) {
	tests := []struct {
		name   string
		status app.StatusSummary
		wantID homeItemID
	}{
		{name: "managed home lands on update", status: app.StatusSummary{Action: "managed"}, wantID: homeUpdate},
		{name: "package home lands on the first entry", status: app.StatusSummary{Action: "package"}},
		{name: "absent home lands on the first entry", status: app.StatusSummary{Action: "absent"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newModel(Options{Version: "test"})
			m.status = tc.status
			got := m.defaultHomeCursor()
			want := 0
			if tc.wantID != "" {
				want = m.homeIndexOf(tc.wantID)
			}
			if got != want {
				t.Fatalf("defaultHomeCursor = %d, want %d", got, want)
			}
		})
	}
}
