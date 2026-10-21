package shellsetup

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// cappedOutput keeps arbitrary user startup output from exhausting memory.
type cappedOutput struct{ bytes.Buffer }

func (b *cappedOutput) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := (1 << 20) - b.Len(); remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.Buffer.Write(p)
	}
	return n, nil
}

func Run(ctx context.Context, binary string, args ...string) (string, error) {
	return RunInput(ctx, binary, "", args...)
}

// RunInput can exercise the ordinary interactive PowerShell startup path by
// feeding commands at its prompt, without -Command or -NonInteractive.
func RunInput(ctx context.Context, binary, input string, args ...string) (string, error) {
	return runInput(ctx, binary, input, nil, args...)
}

// RunProbe marks only the child readiness shell, keeping probe-only completion
// policy out of ordinary user terminals and tool-generation subprocesses.
func RunProbe(ctx context.Context, binary string, args ...string) (string, error) {
	return runInput(ctx, binary, "", []string{"FESTIVAL_INTEGRATION_PROBE=1"}, args...)
}

func runInput(ctx context.Context, binary, input string, env []string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...) //nolint:gosec // resolved local tool with explicit arguments
	prepareCommand(cmd)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
	cmd.WaitDelay = time.Second
	var out, stderr cappedOutput
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		detail := strings.TrimSpace(stderr.String())
		if len(detail) > 1024 {
			detail = detail[:1024]
		}
		return "", fmt.Errorf("%s: %w: %s", binary, err, detail)
	}
	if out.Len() >= 1<<20 {
		return "", fmt.Errorf("%s output exceeded 1 MiB", binary)
	}
	return out.String(), nil
}
