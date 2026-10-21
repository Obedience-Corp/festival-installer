package shellsetup

import (
	"bytes"
	"context"
	"fmt"
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
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...) //nolint:gosec // resolved local tool with explicit arguments
	prepareCommand(cmd)
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
