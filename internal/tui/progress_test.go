package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Obedience-Corp/obey-installer/internal/app"
	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
)

// drain runs the cmd waitProgress produced and fails if no message arrives.
func drain(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("no drain command to run")
	}
	out := make(chan tea.Msg, 1)
	go func() { out <- cmd() }()
	select {
	case msg := <-out:
		return msg
	case <-time.After(2 * time.Second):
		t.Fatal("drain command blocked")
		return nil
	}
}

func TestReportDoesNotBlockWhenBufferIsFull(t *testing.T) {
	ps := newProgressStream()
	for i := 0; i < progressBuf; i++ {
		ps.report(app.ProgressEvent{Stage: "download", Percent: 0.25})
	}

	done := make(chan struct{})
	go func() {
		ps.report(app.ProgressEvent{Stage: "activate", Percent: 0.85, Message: "newest"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("report blocked on a full buffer; the install goroutine would stall")
	}

	if got := len(ps.events); got != progressBuf {
		t.Fatalf("buffered %d events, want %d: overflow must drop, not grow", got, progressBuf)
	}
	var last app.ProgressEvent
	for len(ps.events) > 0 {
		last = <-ps.events
	}
	if last.Message != "newest" {
		t.Fatalf("newest event lost on overflow, tail = %+v", last)
	}
}

func TestProgressMsgIgnoredWhenStreamIsStaleOrAbsent(t *testing.T) {
	live := newProgressStream()
	stale := newProgressStream()
	base := app.ProgressEvent{Stage: "resolve", Percent: 0.1, Message: "starting"}

	tests := []struct {
		name    string
		current *progressStream
		msg     progressMsg
	}{
		{
			name:    "no operation in flight",
			current: nil,
			msg:     progressMsg{stream: stale, ev: app.ProgressEvent{Stage: "extract", Percent: 0.7}},
		},
		{
			name:    "superseded operation",
			current: live,
			msg:     progressMsg{stream: stale, ev: app.ProgressEvent{Stage: "extract", Percent: 0.7}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newModel(Options{Version: "test"})
			m.busy = true
			m.screen = screenProgress
			m.progress = base
			m.progressStream = tc.current

			next, cmd := m.Update(tc.msg)
			nm := next.(model)
			if cmd != nil {
				t.Fatal("stale progress must not re-arm the drain loop")
			}
			if nm.progress != base {
				t.Fatalf("progress = %+v, want it untouched at %+v", nm.progress, base)
			}
		})
	}
}

func TestProgressClosedClearsOnlyItsOwnStream(t *testing.T) {
	live := newProgressStream()
	stale := newProgressStream()

	t.Run("stale close leaves the live stream", func(t *testing.T) {
		m := newModel(Options{Version: "test"})
		m.progressStream = live
		next, _ := m.Update(progressClosedMsg{stream: stale})
		if next.(model).progressStream != live {
			t.Fatal("a finished operation cleared the stream of the one that replaced it")
		}
	})

	t.Run("own close ends the drain loop", func(t *testing.T) {
		m := newModel(Options{Version: "test"})
		m.progressStream = live
		next, cmd := m.Update(progressClosedMsg{stream: live})
		if cmd != nil {
			t.Fatal("closed stream must not re-arm the drain loop")
		}
		if next.(model).progressStream != nil {
			t.Fatal("progressStream should be nil once the stream closes")
		}
	})
}

func TestClosedStreamDeliversTerminalMessage(t *testing.T) {
	ps := newProgressStream()
	ps.close()
	msg := drain(t, waitProgress(ps))
	closed, ok := msg.(progressClosedMsg)
	if !ok {
		t.Fatalf("msg = %T, want progressClosedMsg", msg)
	}
	if closed.stream != ps {
		t.Fatal("terminal message must identify its own stream")
	}
}

func TestReportedEventsDriveTheBar(t *testing.T) {
	tests := []struct {
		name     string
		events   []app.ProgressEvent
		wantPct  float64
		wantView string
	}{
		{
			name:     "mid install",
			events:   []app.ProgressEvent{{Stage: "download", Percent: 0.25, Message: "downloading 1.2.0"}},
			wantPct:  0.25,
			wantView: "25%",
		},
		{
			name: "advances across stages",
			events: []app.ProgressEvent{
				{Stage: "download", Percent: 0.25, Message: "downloading 1.2.0"},
				{Stage: "verify", Percent: 0.5, Message: "verifying checksum"},
			},
			wantPct:  0.5,
			wantView: "50%",
		},
		{
			name: "terminal event completes the bar",
			events: []app.ProgressEvent{
				{Stage: "activate", Percent: 0.85, Message: "lighting camp & fest"},
				{Stage: "done", Percent: 1, Message: "festival suite ready"},
			},
			wantPct:  1,
			wantView: "100%",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ps := newProgressStream()
			m := newModel(Options{Version: "test"})
			m.reduced = true
			m.busy = true
			m.screen = screenProgress
			m.progressStream = ps

			cmd := waitProgress(ps)
			for _, ev := range tc.events {
				ps.report(ev)
				msg := drain(t, cmd)
				pm, ok := msg.(progressMsg)
				if !ok {
					t.Fatalf("msg = %T, want progressMsg", msg)
				}
				var next tea.Model
				next, cmd = m.Update(pm)
				m = next.(model)
				if cmd == nil {
					t.Fatal("update must re-arm the drain loop after an event")
				}
			}

			if m.progress.Percent != tc.wantPct {
				t.Fatalf("percent = %v, want %v", m.progress.Percent, tc.wantPct)
			}
			last := tc.events[len(tc.events)-1]
			if m.progress.Stage != last.Stage || m.progress.Message != last.Message {
				t.Fatalf("progress = %+v, want stage/message from %+v", m.progress, last)
			}
			if view := m.View(); !strings.Contains(view, tc.wantView) {
				t.Fatalf("view missing %q:\n%s", tc.wantView, view)
			}
		})
	}
}

func TestOpDoneReleasesItsOwnStream(t *testing.T) {
	live := newProgressStream()
	stale := newProgressStream()

	t.Run("own completion clears the stream without waiting for close", func(t *testing.T) {
		m := newModel(Options{Version: "test"})
		m.busy = true
		m.progressStream = live
		next, _ := m.Update(opDoneMsg{stream: live, title: "Install complete", success: true})
		nm := next.(model)
		if nm.progressStream != nil {
			t.Fatal("completed operation left its stream attached; cleanup depends on progressClosedMsg ordering")
		}
		if nm.busy {
			t.Fatal("completed operation left the model busy")
		}
	})

	t.Run("superseded completion leaves the live stream", func(t *testing.T) {
		m := newModel(Options{Version: "test"})
		m.busy = true
		m.progressStream = live
		next, _ := m.Update(opDoneMsg{stream: stale, title: "Install failed"})
		if next.(model).progressStream != live {
			t.Fatal("a superseded operation's completion cleared the live stream")
		}
	})
}

func TestUninstallReleasesItsProgressStream(t *testing.T) {
	t.Setenv("FESTIVAL_HOME", t.TempDir())
	m := newModel(Options{Version: "test"})
	next, cmd := m.startUninstall("acme/fest-demo")
	nm := next.(model)
	if nm.progressStream == nil {
		t.Fatal("uninstall started without a progress stream")
	}
	first := drain(t, cmd)
	batch, ok := first.(tea.BatchMsg)
	if !ok {
		t.Fatalf("cmd yields %T, want tea.BatchMsg with the op and its drain", first)
	}

	msgs := make(chan tea.Msg, len(batch))
	for _, c := range batch {
		go func(c tea.Cmd) { msgs <- c() }(c)
	}
	var done opDoneMsg
	var closed progressClosedMsg
	var haveDone, haveClosed bool
	for range batch {
		select {
		case msg := <-msgs:
			switch v := msg.(type) {
			case opDoneMsg:
				done, haveDone = v, true
			case progressClosedMsg:
				closed, haveClosed = v, true
			default:
				t.Fatalf("unexpected message %T from uninstall batch", msg)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("uninstall batch did not complete")
		}
	}
	if !haveDone || !haveClosed {
		t.Fatalf("batch delivered done=%v closed=%v, want both", haveDone, haveClosed)
	}
	if !done.success {
		t.Fatalf("uninstall of an absent package should report success with a note, got %+v", done)
	}

	// Deliver completion first: the stream must be released even when the
	// close message has not arrived yet.
	next, _ = nm.Update(done)
	nm = next.(model)
	if nm.progressStream != nil {
		t.Fatal("progressStream still set after uninstall completed")
	}
	if nm.busy {
		t.Fatal("model still busy after uninstall completed")
	}
	next, _ = nm.Update(closed)
	if next.(model).progressStream != nil {
		t.Fatal("late close message re-attached a stream")
	}
}

func TestOperationsOpenAProgressStream(t *testing.T) {
	tests := []struct {
		name  string
		start func(model) (tea.Model, tea.Cmd)
	}{
		{
			name:  "install",
			start: func(m model) (tea.Model, tea.Cmd) { m.screen = screenInstall; return m.startInstall(true) },
		},
		{
			name:  "update",
			start: func(m model) (tea.Model, tea.Cmd) { return m.startUpdate(true) },
		},
		{
			name: "browse install",
			start: func(m model) (tea.Model, tea.Cmd) {
				m.browseFlat = []app.BrowseEntry{{ID: "acme/fest-demo", Class: "plugin"}}
				return m.installBrowseSelection(true)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("FESTIVAL_HOME", t.TempDir())
			m := newModel(Options{Version: "test"})
			m.err = errpkg.New("E_UNVERIFIED_REFUSED", "unsigned")
			next, cmd := tc.start(m)
			nm := next.(model)
			if cmd == nil {
				t.Fatal("expected an operation command")
			}
			if nm.err != nil {
				t.Fatalf("progress screen still shows the overridden refusal: %v", nm.err)
			}
			if nm.progressStream == nil {
				t.Fatal("operation started without a progress stream; the bar would freeze")
			}
			if nm.opCancel == nil {
				t.Fatal("operation started without a cancel func")
			}

			// The stream reaches the model: a reported event repaints the bar.
			nm.progressStream.report(app.ProgressEvent{Stage: "verify", Percent: 0.5, Message: "verifying checksum"})
			msg := drain(t, waitProgress(nm.progressStream))
			after, _ := nm.Update(msg)
			if got := after.(model).progress.Percent; got != 0.5 {
				t.Fatalf("percent = %v, want 0.5 from the reported event", got)
			}
			nm.opCancel()
		})
	}
}
