//go:build windows

package app

import (
	"context"
	"io"
	"os/exec"
	"syscall"
	"time"
)

// runNSISUninstaller runs the uninstaller with a raw command line. Go would
// quote an argument holding a space, but _?= must not contain quotes.
func runNSISUninstaller(ctx context.Context, uninstaller, dir string, out io.Writer) (appCommandResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, uninstaller)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: nsisUninstallCmdLine(uninstaller, dir)}
	return executeAppCommand(ctx, cmd, out, uninstaller)
}
