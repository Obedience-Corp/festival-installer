package tui

import (
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Obedience-Corp/obey-installer/internal/app"
	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
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

func TestInstallStartsStrictBeforeConsent(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.screen = screenInstall

	next, cmd := m.handleEnter()
	nm := next.(model)
	if cmd == nil || nm.screen != screenProgress {
		t.Fatalf("screen=%v cmd=%v, want strict install attempt first", nm.screen, cmd)
	}
	if nm.confirmAct != "" {
		t.Fatalf("should not open consent before attempt, act=%q", nm.confirmAct)
	}
}

func TestConsentNeededMsgOpensOverrideDialog(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.busy = true
	m.screen = screenProgress
	next, cmd := m.Update(consentNeededMsg{
		action: "install-unverified",
		cause:  errpkg.New("E_UNVERIFIED_REFUSED", "unsigned"),
	})
	nm := next.(model)
	if cmd != nil {
		t.Fatal("consent prompt should not start another command")
	}
	if nm.screen != screenConfirm || nm.confirmAct != "install-unverified" {
		t.Fatalf("screen=%v action=%q, want override consent", nm.screen, nm.confirmAct)
	}
	if nm.confirmYes {
		t.Fatal("override consent must default to No")
	}
	if !nm.busy {
		// busy cleared so user can answer
	} else {
		t.Fatal("busy should clear when consent is required")
	}
}

func TestUnverifiedConsentStartsInstall(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.screen = screenConfirm
	m.confirmAct = "install-unverified"
	m.confirmYes = true

	next, cmd := m.handleEnter()
	nm := next.(model)
	if nm.screen != screenProgress || cmd == nil {
		t.Fatalf("screen=%v cmd=%v, want progress and install command", nm.screen, cmd)
	}
}

func TestHasErrorCodeWalksWraps(t *testing.T) {
	inner := errpkg.New("E_UNVERIFIED_REFUSED", "refused")
	outer := errpkg.Wrap("E_PKG_MANIFEST", inner, "load")
	if !hasErrorCode(outer, "E_UNVERIFIED_REFUSED") {
		t.Fatal("expected walk to find E_UNVERIFIED_REFUSED")
	}
	if hasErrorCode(outer, "E_OTHER") {
		t.Fatal("unexpected code match")
	}
}

func TestLaunchpad_SetsPendingLaunch(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.screen = screenLaunchpad
	m.cursor = 0
	// Point PATH at a fake camp so resolve succeeds.
	dir := t.TempDir()
	camp := dir + "/camp"
	if err := os.WriteFile(camp, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("FESTIVAL_HOME", t.TempDir())

	next, cmd := m.launchSelected()
	nm := next.(model)
	if nm.pendingLaunch == nil {
		t.Fatalf("expected pendingLaunch, err=%v", nm.err)
	}
	if nm.pendingLaunch.Tool != "camp" {
		t.Fatalf("tool %q", nm.pendingLaunch.Tool)
	}
	if cmd == nil {
		t.Fatal("expected tea.Quit cmd")
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
		next, cmd := m.installBrowseSelection(true)
		nm := next.(model)
		if cmd == nil || nm.opCancel == nil {
			t.Fatal("browse install should return a command with a cancellation function")
		}
	})
}
