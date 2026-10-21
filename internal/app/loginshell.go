package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Obedience-Corp/festival-installer/internal/shellsetup"
)

// Keep the executable as well as its dialect. A GUI's PATH may resolve a
// different Bash, or may not contain the user's Homebrew shell at all.
type integrationShellTarget struct{ Name, Path string }

func shellTarget(path string) (integrationShellTarget, error) {
	name := strings.TrimSuffix(strings.ToLower(filepath.Base(path)), ".exe")
	target := integrationShellTarget{Name: name, Path: path}
	if !shellsetup.Supported(name) {
		return target, nil
	}
	resolved, err := exec.LookPath(path)
	if err != nil {
		return target, fmt.Errorf("shell executable %q is unavailable: %w", path, err)
	}
	target.Path, err = filepath.Abs(resolved)
	if err != nil {
		return target, err
	}
	if err := shellsetup.ValidateExecutable(target.Path); err != nil {
		return target, fmt.Errorf("shell executable %q is unavailable: %w", path, err)
	}
	return target, nil
}

func integrationShells(ctx context.Context, requested string) ([]integrationShellTarget, error) {
	if requested != "" {
		target, err := shellTarget(requested)
		return []integrationShellTarget{target}, err
	}
	if shell := strings.TrimSpace(os.Getenv("SHELL")); shell != "" {
		return integrationShells(ctx, shell)
	}
	if runtime.GOOS == "windows" {
		paths := windowsIntegrationShells(exec.LookPath, os.Getenv)
		if len(paths) == 0 {
			return nil, fmt.Errorf("no installed PowerShell was found; choose a shell executable with --shell")
		}
		var targets []integrationShellTarget
		for _, path := range paths {
			target, err := shellTarget(path)
			if err != nil {
				return targets, err
			}
			targets = append(targets, target)
		}
		return targets, nil
	}
	u, err := user.Current()
	if err != nil {
		return nil, err
	}
	var output string
	switch runtime.GOOS {
	case "linux":
		output, err = shellsetup.Run(ctx, "/usr/bin/getent", "passwd", u.Uid)
		if err == nil {
			fields := strings.Split(strings.TrimSpace(output), ":")
			if len(fields) == 7 {
				return integrationShells(ctx, fields[6])
			}
		}
	case "darwin":
		output, err = shellsetup.Run(ctx, "/usr/bin/dscl", ".", "-read", "/Users/"+u.Username, "UserShell")
		if err == nil {
			fields := strings.Fields(output)
			if len(fields) == 2 && fields[0] == "UserShell:" {
				return integrationShells(ctx, fields[1])
			}
		}
	}
	return nil, fmt.Errorf("cannot detect the login shell; choose one with --shell")
}

// Windows has no single login shell. Cover both installed console editions;
// each has its own profile and must pass its own executable's startup probe.
func windowsIntegrationShells(lookup func(string) (string, error), getenv func(string) string) []string {
	var paths []string
	for _, edition := range []struct{ name, root, relative string }{
		{"pwsh.exe", "ProgramFiles", filepath.Join("PowerShell", "7", "pwsh.exe")},
		{"powershell.exe", "SystemRoot", filepath.Join("System32", "WindowsPowerShell", "v1.0", "powershell.exe")},
	} {
		path, err := lookup(edition.name)
		if err != nil && getenv(edition.root) != "" {
			path, err = lookup(filepath.Join(getenv(edition.root), edition.relative))
		}
		if err == nil {
			paths = append(paths, path)
		}
	}
	return paths
}
