//go:build !windows && container_fs

package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPwshAppendPreservesHostPathSeparator(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FESTIVAL_HOME", filepath.Join(home, "festival"))
	t.Setenv("OBEY_INSTALLER_HOME", "")
	t.Setenv("SHELL", "/usr/bin/pwsh")
	t.Setenv("PATH", t.TempDir())
	if shell := ShellFromEnv(); shell != "pwsh" {
		t.Fatalf("ShellFromEnv = %q, want pwsh", shell)
	}
	plan, err := PlanShellRCAppend(context.Background(), ShellFromEnv())
	if err != nil {
		t.Fatal(err)
	}
	if plan.File != filepath.Join(home, ".config", "powershell", "profile.ps1") || plan.UserPath {
		t.Fatalf("non-Windows pwsh must use its profile: %+v", plan)
	}
	if !strings.Contains(plan.Block, `[System.IO.Path]::PathSeparator`) || strings.Contains(plan.Block, `+ ';' +`) {
		t.Fatalf("pwsh profile would corrupt the host PATH: %s", plan.Block)
	}
	if err := ApplyShellRCAppend(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(plan.File)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `[System.IO.Path]::PathSeparator`) || strings.Contains(string(data), `+ ';' +`) {
		t.Fatalf("persisted pwsh profile would corrupt the host PATH: %s", data)
	}
	again, err := PlanShellRCAppend(context.Background(), "pwsh")
	if err != nil || !again.Present {
		t.Fatalf("second plan must see the profile block: %+v, %v", again, err)
	}
}
