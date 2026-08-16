package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Obedience-Corp/festival-installer/internal/launch"
)

func keyMsg(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func captureModel() model {
	m := newModel(Options{SkipBoot: true})
	m.screen = screenChildOutput
	m.captureTitle = "fest list"
	m.captureMode = launch.ModeOneShot
	m.captureVP = viewport.New(80, 20)
	return m
}

func TestCaptureChunkAppendsAndRearms(t *testing.T) {
	m := captureModel()
	m.capture = &launch.Capture{}
	next, cmd := m.Update(childChunkMsg("hello\n"))
	got := next.(model)
	if string(got.captureOut) != "hello\n" {
		t.Fatalf("captureOut = %q", got.captureOut)
	}
	if cmd == nil {
		t.Fatal("expected re-armed captureNext cmd after chunk")
	}
}

func TestCaptureScrollbackBounded(t *testing.T) {
	m := captureModel()
	m.capture = &launch.Capture{}
	m.captureOut = make([]byte, captureMaxBytes)
	next, _ := m.Update(childChunkMsg("tail"))
	got := next.(model)
	if len(got.captureOut) != captureMaxBytes {
		t.Fatalf("scrollback grew past bound: %d", len(got.captureOut))
	}
	if !strings.HasSuffix(string(got.captureOut), "tail") {
		t.Fatal("bounded scrollback dropped the newest output")
	}
}

func TestCaptureExitThenQuitReturnsToLaunchpad(t *testing.T) {
	m := captureModel()
	m.capture = &launch.Capture{}
	next, _ := m.Update(childExitMsg(launch.Result{Started: true, ExitCode: 0}))
	got := next.(model)
	if got.capture != nil || got.captureRes == nil {
		t.Fatalf("exit not recorded: capture=%v res=%v", got.capture, got.captureRes)
	}
	after, _ := got.Update(keyMsg("q"))
	final := after.(model)
	if final.screen != screenLaunchpad {
		t.Fatalf("q after exit should return to launchpad, got screen %d", final.screen)
	}
	if final.captureRes != nil || final.captureOut != nil {
		t.Fatal("capture state not reset on close")
	}
}

func TestCaptureViewShowsStatusAndOutput(t *testing.T) {
	m := captureModel()
	m.captureOut = []byte("v0.4.0\n")
	m.captureVP.SetContent(string(m.captureOut))
	res := launch.Result{Started: true, ExitCode: 0}
	m.captureRes = &res
	view := m.View()
	if !strings.Contains(view, "fest list") || !strings.Contains(view, "done") {
		t.Fatalf("view missing title/status:\n%s", view)
	}
	if !strings.Contains(view, "v0.4.0") {
		t.Fatalf("view missing captured output:\n%s", view)
	}
}

func TestCaptureEntryDispatchesInsideHub(t *testing.T) {
	m := newModel(Options{SkipBoot: true})
	m.screen = screenLaunchpad
	m.launchEntries = []launch.Entry{{
		Label: "echo",
		Spec:  launch.Spec{Tool: "sh", Args: []string{"-c", "echo hi"}, Title: "echo"},
		Mode:  launch.ModeOneShot,
	}}
	m.cursor = 0
	next, cmd := m.launchSelected()
	got := next.(model)
	if got.screen != screenChildOutput {
		t.Fatalf("capture entry should open child-output screen, got %d", got.screen)
	}
	if got.pendingLaunch != nil {
		t.Fatal("capture entry must not set pendingLaunch")
	}
	if got.capture == nil || cmd == nil {
		t.Fatal("capture entry should start a capture and arm captureNext")
	}
	got.capture.Stop()
}

func TestCaptureRenderSanitizes(t *testing.T) {
	raw := []byte("\x1b[2J\x1b[1;31mred\x1b[0m line\nprogress 10%\rprogress 99%\n" + strings.Repeat("w", 200) + "\n")
	out := captureRender(raw, 40)
	if strings.Contains(out, "\x1b") {
		t.Fatalf("ANSI escapes survived: %q", out)
	}
	lines := strings.Split(out, "\n")
	if lines[0] != "red line" {
		t.Fatalf("styled line mangled: %q", lines[0])
	}
	if lines[1] != "progress 99%" {
		t.Fatalf("CR overwrite not collapsed: %q", lines[1])
	}
	if len(lines[2]) > 40 {
		t.Fatalf("wide line not truncated: %d chars", len(lines[2]))
	}
}
