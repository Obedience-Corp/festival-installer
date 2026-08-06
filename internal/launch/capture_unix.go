//go:build !windows

package launch

import (
	"os/exec"
	"syscall"
)

// setCaptureSysProcAttr puts the capture child in its own process group so
// Stop can signal the whole tree without touching the hub.
func setCaptureSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func stopProcess(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
}

func killProcess(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
