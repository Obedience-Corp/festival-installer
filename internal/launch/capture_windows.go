//go:build windows

package launch

import "os/exec"

// Windows has no POSIX process groups; best effort is killing the direct child.
func setCaptureSysProcAttr(_ *exec.Cmd) {}

func stopProcess(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

func killProcess(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
