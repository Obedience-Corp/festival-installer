package tui

import (
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Obedience-Corp/festival-installer/internal/app"
)

// progressBuf bounds undelivered progress events. The installer goroutine must
// never wait on the UI, so a full buffer drops the oldest pending event
// instead of blocking the install.
const progressBuf = 32

// warnStage is a progress event that carries a verification warning rather
// than a bar update. Update must not assign it to m.progress.
const warnStage = "unverified-warn"

// progressStream carries app.ProgressEvent from an operation's goroutine into
// the bubbletea update loop, which drains it with waitProgress. It also
// implements io.Writer so TUI-driven verify options can route unverified-
// content warnings into the working screen instead of os.Stderr.
type progressStream struct {
	events chan app.ProgressEvent
	mu     sync.Mutex
	warn   string
}

func newProgressStream() *progressStream {
	return &progressStream{events: make(chan app.ProgressEvent, progressBuf)}
}

// report is the app.ProgressFunc handed to the app layer. It runs on the
// operation's goroutine and never blocks: when the buffer is full the oldest
// pending event is discarded so the bar advances to the newest stage instead
// of stalling the install.
func (s *progressStream) report(ev app.ProgressEvent) {
	if s == nil {
		return
	}
	select {
	case s.events <- ev:
		return
	default:
	}
	select {
	case <-s.events:
	default:
	}
	select {
	case s.events <- ev:
	default:
	}
}

// close ends the stream. Only the operation goroutine may call it, after the
// app call it fed has returned and can no longer report.
func (s *progressStream) close() {
	if s == nil {
		return
	}
	close(s.events)
}

// waitProgress delivers the next event to Update and is re-issued from the
// progressMsg branch, the same drain pattern captureNext uses for child output.
func waitProgress(s *progressStream) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-s.events
		if !ok {
			return progressClosedMsg{stream: s}
		}
		return progressMsg{stream: s, ev: ev}
	}
}

// beginProgress switches to the progress screen and opens the stream the
// operation feeds. Replacing the stream here means late events from a finished
// operation are ignored rather than repainting the new one's bar. The error
// that opened a consent dialog is cleared too, so the running operation is not
// painted under the refusal the user just overrode.
func (m model) beginProgress(ev app.ProgressEvent) (model, *progressStream) {
	m.busy = true
	m.screen = screenProgress
	m.err = nil
	m.warnText = ""
	m.progress = ev
	m.progressStream = newProgressStream()
	return m, m.progressStream
}
