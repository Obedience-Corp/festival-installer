//go:build unix

package launch

import (
	"errors"
	"os/exec"
	"syscall"
)

// classifyExit maps a failed cmd.Run error to exit code / signal / started.
func classifyExit(err error) (exitCode int, signalName string, started bool) {
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		return -1, "", false
	}
	if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		sig := ws.Signal()
		// Conventional shell encoding so ExitCode stays non-negative and distinct
		// from the "never started" sentinel (-1).
		return 128 + int(sig), sig.String(), true
	}
	return ee.ExitCode(), "", true
}
