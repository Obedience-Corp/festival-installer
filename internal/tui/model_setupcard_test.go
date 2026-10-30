package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestHomeReflectsSuiteState(t *testing.T) {
	for _, tc := range []struct {
		name, action, want string
		unknown            bool
	}{
		{"fresh", "absent", "Festival suite not installed", false},
		{"managed", "managed", "Festival suite 0.3.26 · installed", false},
		{"package", "package", "installed via", false},
		{"unreadable", "absent", "Suite status unavailable", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newModel(Options{})
			m.status = app.StatusSummary{Action: tc.action, Version: "0.3.26", Setup: app.SetupState{SignalsIncomplete: tc.unknown}}
			out := m.View()
			if !strings.Contains(out, tc.want) {
				t.Fatalf("missing %q:\n%s", tc.want, out)
			}
			if (tc.action == "managed" || tc.action == "package" || tc.unknown) && strings.Contains(out, "Install Festival suite") {
				t.Fatal("existing or unknown installation offered reinstall")
			}
			if !strings.Contains(out, "Obey agent sessions") {
				t.Fatal("Obey missing from home")
			}
			if tc.unknown {
				m.cursor = m.homeIndexOf(homeInstall)
				next, _ := m.openHomeItem()
				if next.(model).screen != screenDoctor {
					t.Fatal("unknown status must diagnose, not install")
				}
			}
		})
	}
}

func TestHomeFitsAndScrollsWithSelection(t *testing.T) {
	for _, size := range [][2]int{{48, 18}, {80, 24}, {120, 32}} {
		m := newModel(Options{})
		m.width, m.height = size[0], size[1]
		for _, cursor := range []int{0, len(m.homeMenu()) - 1} {
			m.cursor = cursor
			out := m.View()
			if lipgloss.Height(out) > m.height {
				t.Fatalf("%v: height %d\n%s", size, lipgloss.Height(out), out)
			}
			for _, line := range strings.Split(out, "\n") {
				if lipgloss.Width(line) > m.width {
					t.Fatalf("%v: wide line %q", size, line)
				}
			}
			item, _ := m.homeItemAt(cursor)
			if !strings.Contains(out, item.label) {
				t.Fatalf("selection %q hidden", item.label)
			}
		}
	}
}

func TestHomeStatusDoesNotMoveUserSelection(t *testing.T) {
	m := newModel(Options{})
	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(model)
	cursor := m.cursor
	next, _ = m.Update(statusMsg{sum: app.StatusSummary{Action: "managed", Version: "0.3.25"}})
	m = next.(model)
	next, _ = m.Update(updatesMsg{generation: m.updatesGeneration, result: app.UpdatesResult{Packages: []app.PackageUpdate{{Package: app.FestivalPackageID, Latest: "0.3.26", Status: app.UpdateAvailable}}}})
	m = next.(model)
	if m.cursor != cursor {
		t.Fatal("asynchronous status moved the selected action")
	}
}

func TestObeyUnknownStatusCannotInstall(t *testing.T) {
	m := newModel(Options{})
	next, _ := m.openObey()
	m = next.(model)
	if _, cmd := m.startObey(false); cmd != nil {
		t.Fatal("install started while loading")
	}
	next, _ = m.Update(obeyInventoryMsg{id: m.obeyLoadID, err: errors.New("receipt read failed")})
	m = next.(model)
	if _, cmd := m.startObey(false); cmd != nil {
		t.Fatal("install started after failed read")
	}
	if !strings.Contains(m.viewObey(), "Installation status unavailable") {
		t.Fatal("error presented as missing package")
	}
	next, _ = m.openObey()
	m = next.(model)
	next, _ = m.Update(obeyInventoryMsg{id: m.obeyLoadID - 1})
	m = next.(model)
	if !m.obeyLoading {
		t.Fatal("stale inventory replaced new request")
	}
}

func TestObeyInstalledAndMissingActions(t *testing.T) {
	m := newModel(Options{})
	m.screen = screenObey
	if !strings.Contains(m.viewObey(), "Enter to install Obey") {
		t.Fatal("missing install action")
	}
	m.obeyInventory = app.ListResult{Packages: []app.ListEntry{{PackageID: app.ObeyPackageID, Version: "0.4.0"}}}
	out := m.viewObey()
	if !strings.Contains(out, "Installed · 0.4.0") || !strings.Contains(out, "Enter to check for updates") {
		t.Fatal(out)
	}
	if !strings.Contains(out, "restart is deferred") {
		t.Fatal("running-session policy missing")
	}
	next, cmd := m.startObey(false)
	n := next.(model)
	if cmd == nil || n.opCancel == nil || n.screen != screenProgress {
		t.Fatal("update must use cancellable progress")
	}
	n.opCancel()
}

func TestObeyScreenFitsShortTerminals(t *testing.T) {
	for _, size := range [][2]int{{48, 18}, {80, 24}} {
		m := newModel(Options{})
		m.width, m.height = size[0], size[1]
		m.screen = screenObey
		if lipgloss.Height(m.View()) > m.height {
			t.Fatalf("%v: clipped Obey view\n%s", size, m.View())
		}
	}
}

func TestResultWarningsRemainReachable(t *testing.T) {
	m := newModel(Options{})
	m.screen = screenResult
	m.width, m.height = 48, 18
	m.resultOK = true
	m.resultBody = strings.Repeat("A long setup warning that wraps in a narrow terminal.\n", 20) + "obey service restart"
	if lipgloss.Height(m.View()) > m.height {
		t.Fatal("result overflow")
	}
	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnd})
	m = next.(model)
	if !strings.Contains(m.View(), "obey service restart") {
		t.Fatal("restart instruction unreachable")
	}
}
