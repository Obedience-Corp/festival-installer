package app

import (
	"fmt"
	"io"
	"math"
	"time"
)

const (
	textProgressInterval    = 200 * time.Millisecond
	textProgressUnknownStep = 10_000_000
)

// NewTextProgress renders App download progress on w (stderr in human mode).
// On a terminal it rewrites one line at most every 200ms; otherwise it prints a
// line at each 10 percent step (every 10 MB when the size is unknown).
func NewTextProgress(w io.Writer, terminal bool) ProgressFunc {
	return newTextProgress(w, terminal, time.Now)
}

func newTextProgress(w io.Writer, terminal bool, now func() time.Time) ProgressFunc {
	var (
		lastPrint  time.Time
		lastStep   = int64(-1)
		drewLine   bool
		finished   bool
		totalBytes int64
	)
	return func(ev ProgressEvent) {
		if ev.Stage == "verify" {
			if terminal && drewLine && !finished {
				_, _ = fmt.Fprint(w, "\n")
			}
			finished = true
			return
		}
		if ev.Stage != "download" || finished {
			return
		}
		if ev.Percent > 0 {
			totalBytes = int64(math.Round(float64(ev.Bytes) / ev.Percent))
		}
		line := textProgressLine(ev, totalBytes)
		if terminal {
			t := now()
			if drewLine && t.Sub(lastPrint) < textProgressInterval {
				return
			}
			lastPrint = t
			drewLine = true
			_, _ = fmt.Fprint(w, "\r"+line+"\x1b[K")
			return
		}
		step := ev.Bytes / textProgressUnknownStep
		if ev.Percent >= 0 {
			step = int64(ev.Percent * 10)
		}
		if step == lastStep {
			return
		}
		lastStep = step
		_, _ = fmt.Fprintln(w, line)
	}
}

func textProgressLine(ev ProgressEvent, total int64) string {
	if ev.Percent < 0 {
		return fmt.Sprintf("%s: %s", ev.Message, megabytes(ev.Bytes))
	}
	return fmt.Sprintf("%s: %s of %s (%d%%)", ev.Message, megabytes(ev.Bytes), megabytes(total), int(ev.Percent*100))
}

func megabytes(n int64) string {
	return fmt.Sprintf("%.1f MB", float64(n)/1e6)
}
