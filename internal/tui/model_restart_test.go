package tui

import (
	"os"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Obedience-Corp/festival-installer/internal/app"
)

func TestUpdateOpDoneMsg_SelfReplacedSetsRestart(t *testing.T) {
	msg := updateOpDoneMsg(nil, app.UpdateResult{Action: "upgraded", Version: "1.2.3", From: "1.2.2", SelfReplaced: true}, "")
	if !msg.restart {
		t.Fatal("expected restart true")
	}
	if !strings.Contains(msg.body, "restart to use the new version") {
		t.Fatalf("body missing restart line: %s", msg.body)
	}
}

func TestUpdateOpDoneMsg_NotSelfReplacedNoRestart(t *testing.T) {
	msg := updateOpDoneMsg(nil, app.UpdateResult{Action: "upgraded", Version: "1.2.3", From: "1.2.2", SelfReplaced: false}, "")
	if msg.restart {
		t.Fatal("expected restart false")
	}
	if strings.Contains(msg.body, "restart to use the new version") {
		t.Fatalf("unexpected restart line: %s", msg.body)
	}
}

func TestUpdateOpDoneMsg_CurrentActionNeverRestarts(t *testing.T) {
	msg := updateOpDoneMsg(nil, app.UpdateResult{Action: "current", Version: "1.2.3"}, "")
	if msg.restart {
		t.Fatal("expected restart false for an already-current result")
	}
}

func TestResult_SelfReplacedShowsRestartLine(t *testing.T) {
	m := newModel(Options{Version: "test"})
	next, _ := m.Update(opDoneMsg{
		title:   "Updated",
		body:    "action: upgraded\nversion: 1.2.3\n\nfestival was updated to 1.2.3; restart to use the new version\n",
		success: true,
		restart: true,
	})
	nm := next.(model)
	if nm.screen != screenResult {
		t.Fatalf("screen = %v, want screenResult", nm.screen)
	}
	if !nm.resultRestart {
		t.Fatal("expected resultRestart true")
	}
	if !strings.Contains(nm.viewResult(), "restart to use the new version") {
		t.Fatalf("result screen missing restart line: %s", nm.viewResult())
	}
	// Critical constraint: arriving at the result screen with a restart
	// offer must never itself launch anything. Only an explicit r keypress
	// (TestResult_RestartKeySetsPendingLaunch) may set pendingLaunch.
	if nm.pendingLaunch != nil {
		t.Fatalf("opDoneMsg alone must not set pendingLaunch, got %+v", nm.pendingLaunch)
	}
}

func TestResult_NotSelfReplacedNoRestartLine(t *testing.T) {
	m := newModel(Options{Version: "test"})
	next, _ := m.Update(opDoneMsg{
		title:   "Updated",
		body:    "action: upgraded\nversion: 1.2.3\n",
		success: true,
		restart: false,
	})
	nm := next.(model)
	if nm.resultRestart {
		t.Fatal("expected resultRestart false")
	}
	if strings.Contains(nm.viewResult(), "restart to use the new version") {
		t.Fatalf("unexpected restart line: %s", nm.viewResult())
	}
}

func TestResult_RestartKeySetsPendingLaunch(t *testing.T) {
	dir := t.TempDir()
	festival := dir + "/festival"
	if err := os.WriteFile(festival, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("FESTIVAL_HOME", t.TempDir())

	m := newModel(Options{Version: "test"})
	m.screen = screenResult
	m.resultRestart = true

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	nm := next.(model)
	if nm.pendingLaunch == nil {
		t.Fatalf("expected pendingLaunch, err=%v", nm.err)
	}
	if nm.pendingLaunch.Tool != "festival" {
		t.Fatalf("tool = %q, want festival", nm.pendingLaunch.Tool)
	}
	if !reflect.DeepEqual(nm.pendingLaunch.Args, os.Args[1:]) {
		t.Fatalf("args = %v, want %v", nm.pendingLaunch.Args, os.Args[1:])
	}
	if cmd == nil {
		t.Fatal("expected tea.Quit cmd")
	}
}

func TestResult_OtherKeyDoesNotSetPendingLaunch(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.screen = screenResult
	m.resultRestart = true

	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(model)
	if nm.pendingLaunch != nil {
		t.Fatalf("expected no pendingLaunch, got %+v", nm.pendingLaunch)
	}
}

func TestResult_RestartFooterHintOnlyWhenOffered(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.screen = screenResult
	m.resultRestart = false
	m.width = 80
	if strings.Contains(m.View(), "r restart") {
		t.Fatal("footer should not hint restart when none is offered")
	}
	m.resultRestart = true
	if !strings.Contains(m.View(), "r restart") {
		t.Fatal("footer should hint restart when one is offered")
	}
}
