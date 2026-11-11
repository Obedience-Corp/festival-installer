package app

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"time"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

const appCommandCap = 1 << 20

type appCommandResult struct {
	ExitCode       int
	Stdout, Stderr string
}

type appCommandRunner func(ctx context.Context, timeout time.Duration, dir string, out io.Writer, name string, args ...string) (appCommandResult, error)

// runAppCommand runs one command with a timeout, capped captured output and
// optional live output. A completed process with a non-zero status returns the
// result and a nil error. A start failure returns the exec error. A cancelled or
// expired context returns ctx.Err() wrapped as E_INSTALL_CTX.
func runAppCommand(ctx context.Context, timeout time.Duration, dir string, out io.Writer, name string, args ...string) (appCommandResult, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	return executeAppCommand(ctx, cmd, out, name)
}

// executeAppCommand runs a prepared command that was built with ctx.
func executeAppCommand(ctx context.Context, cmd *exec.Cmd, out io.Writer, name string) (appCommandResult, error) {
	if out == nil {
		out = io.Discard
	}
	stdout, stderr := &cappedBuffer{limit: appCommandCap}, &cappedBuffer{limit: appCommandCap}
	cmd.WaitDelay = time.Second
	cmd.Stdout = io.MultiWriter(stdout, out)
	cmd.Stderr = io.MultiWriter(stderr, out)
	err := cmd.Run()
	res := appCommandResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return res, errpkg.Wrap("E_INSTALL_CTX", ctxErr, "command "+name+" cancelled or timed out")
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		res.ExitCode = exit.ExitCode()
		return res, nil
	}
	return res, err
}
