package app

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Obedience-Corp/festival-installer/internal/shellsetup"
)

func integrationShell(ctx context.Context) (string, error) {
	if shell := strings.TrimSpace(os.Getenv("SHELL")); shell != "" {
		return strings.TrimSuffix(filepath.Base(shell), ".exe"), nil
	}
	if runtime.GOOS == "windows" {
		return "powershell", nil
	}
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	var output string
	switch runtime.GOOS {
	case "linux":
		output, err = shellsetup.Run(ctx, "/usr/bin/getent", "passwd", u.Uid)
		if err == nil {
			fields := strings.Split(strings.TrimSpace(output), ":")
			if len(fields) == 7 {
				return filepath.Base(fields[6]), nil
			}
		}
	case "darwin":
		output, err = shellsetup.Run(ctx, "/usr/bin/dscl", ".", "-read", "/Users/"+u.Username, "UserShell")
		if err == nil {
			fields := strings.Fields(output)
			if len(fields) == 2 && fields[0] == "UserShell:" {
				return filepath.Base(fields[1]), nil
			}
		}
	}
	return "", fmt.Errorf("cannot detect the login shell; choose one with --shell")
}
