package tui

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	"github.com/Obedience-Corp/festival-installer/internal/metadata"
)

func TestTuiVerifyOptionsNeverUsesStderr(t *testing.T) {
	vo := tuiVerifyOptions(nil, true)
	if vo.WarnWriter == os.Stderr {
		t.Fatal("nil TUI warn writer must not fall back to stderr")
	}
	if vo.WarnWriter != io.Discard {
		t.Fatalf("nil TUI warn writer = %T, want io.Discard", vo.WarnWriter)
	}
	if !vo.AllowUnverified {
		t.Fatal("allowUnverified must propagate")
	}

	ps := newProgressStream()
	vo = tuiVerifyOptions(ps, true)
	if vo.WarnWriter != io.Writer(ps) {
		t.Fatal("install/update must pass the progress stream as WarnWriter")
	}
}

func TestProgressStreamWriteCapturesUnverifiedWarning(t *testing.T) {
	ps := newProgressStream()
	vo := tuiVerifyOptions(ps, true)
	if err := metadata.EnforceUnverifiedPolicy(metadata.IngestOptions{
		Policy:      metadata.PolicyWarnAllow,
		WarnWriter:  vo.WarnWriter,
		SourceLabel: "acme/fest-demo",
	}); err != nil {
		t.Fatalf("warn-allow policy: %v", err)
	}
	if !strings.Contains(ps.warning(), "UNVERIFIED") || !strings.Contains(ps.warning(), "acme/fest-demo") {
		t.Fatalf("stream warning = %q, want UNVERIFIED naming the source", ps.warning())
	}

	msg := drain(t, waitProgress(ps))
	pm, ok := msg.(progressMsg)
	if !ok {
		t.Fatalf("msg = %T, want progressMsg", msg)
	}
	if pm.ev.Stage != warnStage {
		t.Fatalf("stage = %q, want %q so the bar is not overwritten", pm.ev.Stage, warnStage)
	}
	if !strings.Contains(pm.ev.Message, "UNVERIFIED") {
		t.Fatalf("event message = %q, want UNVERIFIED", pm.ev.Message)
	}
}

func TestUnverifiedWarningDoesNotReplaceProgressBar(t *testing.T) {
	ps := newProgressStream()
	m := newModel(Options{Version: "test"})
	m.reduced = true
	m.width = 80
	m.height = 24
	m.busy = true
	m.screen = screenProgress
	m.progressStream = ps
	base := app.ProgressEvent{Stage: "download", Percent: 0.25, Message: "downloading 1.2.0"}
	m.progress = base

	warn := "WARNING: installing UNVERIFIED content from acme/fest-demo (no signature)\n"
	if _, err := ps.Write([]byte(warn)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	next, cmd := m.Update(drain(t, waitProgress(ps)))
	nm := next.(model)
	if cmd == nil {
		t.Fatal("warn event must re-arm the drain loop")
	}
	if nm.progress != base {
		t.Fatalf("progress = %+v, want bar left at %+v", nm.progress, base)
	}
	if !strings.Contains(nm.warnText, "UNVERIFIED") {
		t.Fatalf("warnText = %q, want captured warning", nm.warnText)
	}
	view := nm.View()
	if !strings.Contains(view, "UNVERIFIED") {
		t.Fatalf("working screen missing warning:\n%s", view)
	}
	if !strings.Contains(view, "25%") {
		t.Fatalf("working screen lost the progress bar:\n%s", view)
	}
}

func TestOpDoneCarriesWarningOntoResult(t *testing.T) {
	ps := newProgressStream()
	if _, err := ps.Write([]byte("WARNING: installing UNVERIFIED content from source (no signature)\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	m := newModel(Options{Version: "test"})
	m.reduced = true
	m.width = 80
	m.height = 24
	m.busy = true
	m.screen = screenProgress
	m.progressStream = ps
	m.warnText = "stale"

	t.Setenv("FESTIVAL_HOME", t.TempDir())
	next, _ := m.Update(opDoneMsg{stream: ps, title: "Install complete", body: "installed festival 1.2.0\n", success: true})
	nm := next.(model)
	if nm.screen != screenResult {
		t.Fatalf("screen = %v, want result", nm.screen)
	}
	if !strings.Contains(nm.resultBody, "UNVERIFIED") {
		t.Fatalf("result body missing warning: %q", nm.resultBody)
	}
	if !strings.Contains(nm.resultBody, "installed festival 1.2.0") {
		t.Fatalf("result body lost the install summary: %q", nm.resultBody)
	}
	if view := nm.View(); !strings.Contains(view, "UNVERIFIED") {
		t.Fatalf("result screen missing warning:\n%s", view)
	}
}

func TestWarnWriteDoesNotBlockWhenBufferIsFull(t *testing.T) {
	ps := newProgressStream()
	for i := 0; i < progressBuf; i++ {
		ps.report(app.ProgressEvent{Stage: "download", Percent: 0.25})
	}

	done := make(chan struct{})
	go func() {
		_, _ = ps.Write([]byte("WARNING: installing UNVERIFIED content from source (no signature)\n"))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Write blocked on a full buffer; the install goroutine would stall")
	}
	if got := ps.warning(); !strings.Contains(got, "UNVERIFIED") {
		t.Fatalf("warning lost when the event buffer overflowed: %q", got)
	}
}

func TestBeginProgressClearsCapturedWarning(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.warnText = "WARNING: leftover"
	m, _ = m.beginProgress(app.ProgressEvent{Stage: "resolve", Percent: 0})
	if m.warnText != "" {
		t.Fatalf("beginProgress left warnText = %q", m.warnText)
	}
}

func TestStaleWarnEventDoesNotPaintCurrentBar(t *testing.T) {
	live := newProgressStream()
	stale := newProgressStream()
	_, _ = stale.Write([]byte("WARNING: installing UNVERIFIED content from stale (no signature)\n"))

	m := newModel(Options{Version: "test"})
	m.busy = true
	m.screen = screenProgress
	m.progressStream = live
	m.progress = app.ProgressEvent{Stage: "extract", Percent: 0.7}

	next, cmd := m.Update(drain(t, waitProgress(stale)))
	nm := next.(model)
	if cmd != nil {
		t.Fatal("stale warn must not re-arm the drain loop")
	}
	if nm.warnText != "" {
		t.Fatalf("stale warning leaked onto the live operation: %q", nm.warnText)
	}
	if nm.progress.Stage != "extract" {
		t.Fatalf("progress stage = %q, want extract", nm.progress.Stage)
	}
}

func TestAppendWarn(t *testing.T) {
	if got := appendWarn("", "  a  "); got != "a" {
		t.Fatalf("append empty = %q", got)
	}
	if got := appendWarn("a", "b"); got != "a\nb" {
		t.Fatalf("append second = %q", got)
	}
	if got := appendWarn("a", "  "); got != "a" {
		t.Fatalf("append blank = %q", got)
	}
}
