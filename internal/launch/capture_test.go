package launch

import (
	"context"
	"strings"
	"testing"
	"time"
)

// drain pumps Next until the child exits, returning all output and the result.
func drain(t *testing.T, c *Capture) (string, Result) {
	t.Helper()
	var out strings.Builder
	deadline := time.After(15 * time.Second)
	type step struct {
		chunk []byte
		res   *Result
	}
	for {
		ch := make(chan step, 1)
		go func() {
			b, r := c.Next()
			ch <- step{chunk: b, res: r}
		}()
		select {
		case s := <-ch:
			if s.res != nil {
				return out.String(), *s.res
			}
			out.Write(s.chunk)
		case <-deadline:
			t.Fatal("capture did not finish before deadline")
		}
	}
}

func TestStartCapture_DeliversOutputThenExit(t *testing.T) {
	c, err := StartCapture(context.Background(), Spec{
		Tool: "sh",
		Args: []string{"-c", "echo out; echo err 1>&2; exit 3"},
	})
	if err != nil {
		t.Fatalf("StartCapture: %v", err)
	}
	out, res := drain(t, c)
	if !strings.Contains(out, "out") || !strings.Contains(out, "err") {
		t.Fatalf("combined output missing streams: %q", out)
	}
	if !res.Started {
		t.Fatalf("expected Started, got %+v", res)
	}
	if res.ExitCode != 3 {
		t.Fatalf("exit code = %d, want 3 (%+v)", res.ExitCode, res)
	}
}

func TestStartCapture_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := StartCapture(ctx, Spec{Tool: "sh", Args: []string{"-c", "true"}}); err == nil {
		t.Fatal("expected error for canceled context")
	}
}

func TestStartCapture_EmptyTool(t *testing.T) {
	if _, err := StartCapture(context.Background(), Spec{Tool: "  "}); err == nil {
		t.Fatal("expected error for empty tool")
	}
}

func TestCapture_StopInterruptsChild(t *testing.T) {
	c, err := StartCapture(context.Background(), Spec{
		Tool: "sh",
		Args: []string{"-c", "echo ready; sleep 30"},
	})
	if err != nil {
		t.Fatalf("StartCapture: %v", err)
	}
	// Wait for the child to signal readiness so Stop races nothing.
	chunk, res := c.Next()
	if res != nil {
		t.Fatalf("child exited before Stop: %+v", *res)
	}
	if !strings.Contains(string(chunk), "ready") {
		t.Fatalf("unexpected first chunk %q", chunk)
	}
	c.Stop()
	_, final := drain(t, c)
	if final.Signal == "" && final.ExitCode == 0 {
		t.Fatalf("expected signalled or non-zero exit after Stop, got %+v", final)
	}
}

func TestCatalog_Modes(t *testing.T) {
	want := map[string]Mode{
		"camp wi":             ModeTUI,
		"camp intent explore": ModeTUI,
		"fest list":           ModeOneShot,
		"fest watch":          ModeStream,
		"camp version":        ModeOneShot,
		"fest version":        ModeOneShot,
	}
	for _, e := range Catalog() {
		mode, ok := want[e.Spec.Title]
		if !ok {
			t.Errorf("unexpected catalog entry %q", e.Spec.Title)
			continue
		}
		if e.Mode != mode {
			t.Errorf("%q mode = %q, want %q", e.Spec.Title, e.Mode, mode)
		}
		if e.Mode.IsCapture() != (mode == ModeOneShot || mode == ModeStream) {
			t.Errorf("%q IsCapture mismatch", e.Spec.Title)
		}
	}
}
