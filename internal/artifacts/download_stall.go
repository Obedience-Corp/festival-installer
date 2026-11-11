package artifacts

import (
	"context"
	"errors"
	"io"
	"time"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

// stallWatch cancels a request when no bytes arrive for the stall timeout. A
// zero timeout disables it and leaves the context untouched.
type stallWatch struct {
	ctx     context.Context
	cancel  context.CancelCauseFunc
	timer   *time.Timer
	timeout time.Duration
}

func newStallWatch(ctx context.Context, timeout time.Duration) *stallWatch {
	w := &stallWatch{ctx: ctx, timeout: timeout}
	if timeout <= 0 {
		return w
	}
	w.ctx, w.cancel = context.WithCancelCause(ctx)
	w.timer = time.AfterFunc(timeout, func() { w.cancel(ErrDownloadStalled) })
	return w
}

func (w *stallWatch) stopTimer() {
	if w.timer != nil {
		w.timer.Stop()
	}
}

func (w *stallWatch) stop() {
	w.stopTimer()
	if w.cancel != nil {
		w.cancel(nil)
	}
}

// classify returns the stall error when the watchdog fired, and fallback otherwise.
func (w *stallWatch) classify(err error, url string, fallback error) error {
	if w.cancel != nil && errors.Is(context.Cause(w.ctx), ErrDownloadStalled) {
		return errpkg.Wrap("E_ARTIFACT_STALLED", ErrDownloadStalled, "no bytes for "+w.timeout.String()+" from "+url)
	}
	return fallback
}

// wrap resets the timer and reports progress on every read that returns data.
func (w *stallWatch) wrap(body io.Reader, contentLength int64, progress func(written, total int64)) io.Reader {
	if w.timer == nil && progress == nil {
		return body
	}
	total := contentLength
	if total < 0 {
		total = -1
	}
	return &watchedReader{r: body, w: w, total: total, progress: progress}
}

type watchedReader struct {
	r        io.Reader
	w        *stallWatch
	total    int64
	written  int64
	progress func(written, total int64)
}

func (r *watchedReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if n > 0 {
		if r.w.timer != nil {
			r.w.timer.Reset(r.w.timeout)
		}
		r.written += int64(n)
		if r.progress != nil {
			r.progress(r.written, r.total)
		}
	}
	return n, err
}
