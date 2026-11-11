package app

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func dl(bytesDone int64, percent float64) ProgressEvent {
	return ProgressEvent{Stage: "download", Bytes: bytesDone, Percent: percent, Message: "Downloading Festival App 0.1.8"}
}

func TestTextProgressNonTerminalTenPercentSteps(t *testing.T) {
	var buf bytes.Buffer
	p := newTextProgress(&buf, false, time.Now)
	const total = 100_000_000
	for i := 0; i <= 100; i++ {
		p(dl(int64(i)*total/100, float64(i)/100))
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 11 {
		t.Fatalf("got %d lines, want 11:\n%s", len(lines), buf.String())
	}
	if !strings.HasSuffix(lines[10], "100.0 MB of 100.0 MB (100%)") {
		t.Fatalf("last line %q", lines[10])
	}
}

func TestTextProgressTerminalRewritesOneLine(t *testing.T) {
	var buf bytes.Buffer
	clock := time.Unix(1000, 0)
	p := newTextProgress(&buf, true, func() time.Time { return clock })
	p(dl(1_000_000, 0.1))
	clock = clock.Add(50 * time.Millisecond)
	p(dl(2_000_000, 0.2))
	if n := strings.Count(buf.String(), "\r"); n != 1 {
		t.Fatalf("expected one rewrite within 200ms, got %d: %q", n, buf.String())
	}
	clock = clock.Add(300 * time.Millisecond)
	p(dl(3_000_000, 0.3))
	if n := strings.Count(buf.String(), "\r"); n != 2 {
		t.Fatalf("expected a second rewrite after 200ms, got %d", n)
	}
	p(ProgressEvent{Stage: "verify"})
	if !strings.HasSuffix(buf.String(), "\n") || !strings.Contains(buf.String(), "\x1b[K") {
		t.Fatalf("output %q", buf.String())
	}
}

func TestTextProgressUnknownTotal(t *testing.T) {
	var buf bytes.Buffer
	p := newTextProgress(&buf, false, time.Now)
	p(dl(1_000_000, -1))
	p(dl(12_300_000, -1))
	p(dl(13_000_000, -1))
	want := "Downloading Festival App 0.1.8: 1.0 MB\nDownloading Festival App 0.1.8: 12.3 MB\n"
	if buf.String() != want {
		t.Fatalf("got %q want %q", buf.String(), want)
	}
}
