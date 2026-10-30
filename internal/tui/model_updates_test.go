package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Obedience-Corp/festival-installer/internal/app"
)

func TestUpdatesIgnoreStaleResultsAndPreserveCursor(t *testing.T) {
	m := newModel(Options{})
	m.screen, m.cursor = screenHome, 5
	message := updatesMsg{generation: m.updatesGeneration, result: app.UpdatesResult{CheckedAt: time.Now(), Packages: []app.PackageUpdate{{Package: "one/camp-app", Installed: "1.0.0", Latest: "1.1.0", Status: app.UpdateAvailable}}}}
	next, _ := m.Update(message)
	m = next.(model)
	if m.cursor != 5 || m.updatesChecking || !strings.Contains(m.updatesLabel(), "1 available") {
		t.Fatalf("unexpected background update: %+v", m)
	}
	message.generation--
	message.err = errors.New("old failed request")
	next, _ = m.Update(message)
	if next.(model).updatesErr != nil {
		t.Fatal("stale discovery replaced current result")
	}
}

func TestUpdatesViewAndExplicitActions(t *testing.T) {
	m := newModel(Options{})
	m.screen = screenUpdates
	m.updates = app.UpdatesResult{CheckedAt: time.Now(), Packages: []app.PackageUpdate{{Package: "obedience-corp/camp-app", Installed: "0.1.0", Latest: "0.1.1", Status: app.UpdateAvailable}, {Package: "obedience-corp/obey", Installed: "0.2.0", Status: app.UpdateCheckFailed, Detail: "network offline"}}}
	m.cursor = 1
	_, cmd := m.selectUpdate()
	if cmd != nil {
		t.Fatal("started mutation while discovery was pending")
	}
	m.updatesChecking = false
	m.cursor = 2
	if !strings.Contains(m.viewUpdates(), "network offline") || !strings.Contains(m.updatesLabel(), "check failed") {
		t.Fatal("offline package hidden")
	}
	_, cmd = m.selectUpdate()
	if cmd != nil {
		t.Fatal("failed check was eligible for update")
	}
	m.cursor = 1
	next, cmd := m.selectUpdate()
	if cmd == nil || next.(model).screen != screenProgress {
		t.Fatal("explicit package selection did not start update")
	}
	next.(model).opCancel()
}
