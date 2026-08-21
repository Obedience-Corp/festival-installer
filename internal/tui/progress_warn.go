package tui

import (
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	"github.com/Obedience-Corp/festival-installer/internal/source"
)

var _ io.Writer = (*progressStream)(nil)

// tuiVerifyOptions is DefaultVerifyOptions for TUI-driven operations.
// A nil writer becomes io.Discard so unsigned-content warnings never paint
// os.Stderr over the alt-screen. Install/update pass the progressStream so
// the warning is rendered in the working screen.
func tuiVerifyOptions(w io.Writer, allowUnverified bool) source.VerifyOptions {
	if w == nil {
		w = io.Discard
	}
	return source.DefaultVerifyOptions(w, allowUnverified)
}

// Write implements io.Writer. Verification warnings are accumulated on the
// stream (so completion can still show them if the refresh event is dropped)
// and reported as a warn-stage event to trigger a TUI repaint.
func (s *progressStream) Write(p []byte) (int, error) {
	if s == nil {
		return len(p), nil
	}
	text := strings.TrimSpace(string(p))
	if text == "" {
		return len(p), nil
	}
	s.mu.Lock()
	s.warn = appendWarn(s.warn, text)
	s.mu.Unlock()
	s.report(app.ProgressEvent{Stage: warnStage, Message: text, Percent: -1})
	return len(p), nil
}

func (s *progressStream) warning() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.warn
}

func appendWarn(existing, next string) string {
	next = strings.TrimSpace(next)
	if next == "" {
		return existing
	}
	if existing == "" {
		return next
	}
	return existing + "\n" + next
}

func withCapturedWarn(body, warn string) string {
	warn = strings.TrimSpace(warn)
	if warn == "" {
		return body
	}
	if strings.TrimSpace(body) == "" {
		return warn
	}
	return warn + "\n\n" + body
}

func warnFrom(stream *progressStream, fallback string) string {
	if stream != nil {
		if w := stream.warning(); w != "" {
			return w
		}
	}
	return fallback
}

func (m model) applyProgress(msg progressMsg) (model, tea.Cmd) {
	if m.progressStream == nil || msg.stream != m.progressStream {
		return m, nil
	}
	if msg.ev.Stage != warnStage && m.busy {
		m.progress = msg.ev
	}
	m.warnText = msg.stream.warning()
	return m, waitProgress(m.progressStream)
}

func (m model) applyOpDone(msg opDoneMsg) (model, tea.Cmd) {
	m.busy = false
	m.opCancel = nil
	if msg.stream == m.progressStream {
		m.progressStream = nil
	}
	m.screen = screenResult
	m.resultTitle = msg.title
	m.resultBody = withCapturedWarn(msg.body, warnFrom(msg.stream, m.warnText))
	m.resultOK = msg.success
	m.resultRestart = msg.restart
	m.err = msg.err
	return m, m.loadStatus()
}
