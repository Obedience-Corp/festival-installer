//go:build windows

package launch

import (
	"errors"
	"os/exec"
)

func classifyExit(err error) (exitCode int, signalName string, started bool) {
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		return -1, "", false
	}
	return ee.ExitCode(), "", true
}
