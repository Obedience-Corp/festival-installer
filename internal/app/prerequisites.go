package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/Obedience-Corp/festival-installer/internal/shellsetup"
)

type PrerequisiteReport struct {
	Ready   bool   `json:"ready"`
	Pending bool   `json:"pending"`
	Message string `json:"message"`
}

// prerequisiteCommand uses fixed system executables and arguments. No
// marketplace or shell text is ever passed to a privileged interpreter.
func prerequisiteCommand(goos string, exists func(string) bool) ([]string, bool, error) {
	switch goos {
	case "darwin":
		return []string{"/usr/bin/xcode-select", "--install"}, true, nil
	case "windows":
		return []string{"winget", "install", "--id", "Git.Git", "--exact", "--source", "winget", "--accept-source-agreements", "--accept-package-agreements"}, false, nil
	case "linux":
		if !exists("/usr/bin/pkexec") {
			return nil, false, fmt.Errorf("git installation needs the desktop authorization helper (pkexec)")
		}
		for _, command := range [][]string{
			{"/usr/bin/apt-get", "install", "--yes", "git"},
			{"/usr/bin/dnf", "install", "--assumeyes", "git"},
			{"/usr/bin/pacman", "-S", "--needed", "--noconfirm", "git"},
			{"/usr/bin/zypper", "--non-interactive", "install", "git"},
		} {
			if exists(command[0]) {
				return append([]string{"/usr/bin/pkexec"}, command...), false, nil
			}
		}
	}
	return nil, false, fmt.Errorf("automatic Git installation is unavailable on this platform")
}

func gitReady(ctx context.Context) bool {
	path, err := exec.LookPath("git")
	if err != nil {
		return false
	}
	_, err = shellsetup.Run(ctx, path, "--version")
	return err == nil
}

// SetupPrerequisites invokes the OS installer. The OS owns authorization UI;
// Festival never asks for, captures, or stores an administrator password.
func SetupPrerequisites(ctx context.Context) PrerequisiteReport {
	if gitReady(ctx) {
		return PrerequisiteReport{Ready: true, Message: "Git is available."}
	}
	exists := func(path string) bool {
		info, err := os.Stat(path)
		return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
	}
	args, pending, err := prerequisiteCommand(runtime.GOOS, exists)
	if err != nil {
		return PrerequisiteReport{Message: err.Error()}
	}
	// Package-manager work may take minutes; user authorization is part of
	// this operation, unlike the short bounded setup probes.
	runCtx, cancel := context.WithTimeout(ctx, 9*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(runCtx, args[0], args[1:]...) //nolint:gosec // fixed OS installer command
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		return PrerequisiteReport{Message: "Git installation did not finish. Approve the system installer and retry setup: " + err.Error()}
	}
	if gitReady(ctx) {
		return PrerequisiteReport{Ready: true, Message: "Git is installed."}
	}
	if pending {
		return PrerequisiteReport{Pending: true, Message: "Complete the system developer-tools installer, then check setup again."}
	}
	return PrerequisiteReport{Message: "Git was installed but is not yet available to the app. Reopen Festival App to refresh its environment."}
}
