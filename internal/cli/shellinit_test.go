package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellInit_ZshBashPrependPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	// Single-quoted bin dir is shell-safe; "$PATH" expands when the rc runs.
	binDir := filepath.Join(home, "bin")
	want := "export PATH='" + binDir + "':\"$PATH\""
	for _, shell := range []string{"zsh", "bash"} {
		out, _, err := runInstaller(t, "shell-init", shell)
		if err != nil {
			t.Fatalf("shell-init %s: %v", shell, err)
		}
		if !strings.Contains(out, want) {
			t.Fatalf("shell-init %s missing %q:\n%s", shell, want, out)
		}
	}
}

func TestShellInit_FishPrependPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	binDir := filepath.Join(home, "bin")
	out, _, err := runInstaller(t, "shell-init", "fish")
	if err != nil {
		t.Fatalf("shell-init fish: %v", err)
	}
	want := "fish_add_path --prepend --global '" + binDir + "'"
	if !strings.Contains(out, want) {
		t.Fatalf("fish snippet missing quoted path %q:\n%s", want, out)
	}
}

func TestShellInit_QuotesHomeWithSpaces(t *testing.T) {
	home := t.TempDir()
	// Nested dir with spaces — FESTIVAL_HOME can be any absolute path.
	spaced := filepath.Join(home, "festival home")
	if err := os.MkdirAll(filepath.Join(spaced, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FESTIVAL_HOME", spaced)
	out, _, err := runInstaller(t, "shell-init", "zsh")
	if err != nil {
		t.Fatalf("shell-init zsh: %v", err)
	}
	want := "export PATH='" + filepath.Join(spaced, "bin") + "':\"$PATH\""
	if !strings.Contains(out, want) {
		t.Fatalf("expected quoted spaced path %q in:\n%s", want, out)
	}
}

func TestShellInit_UnsupportedShell(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	if _, _, err := runInstaller(t, "shell-init", "powershell"); err == nil {
		t.Fatal("expected error for unsupported shell")
	} else if !strings.Contains(err.Error(), "unsupported shell") {
		t.Fatalf("expected unsupported-shell error, got %v", err)
	}
}

func TestShellInit_CheckOnPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	binDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Setenv("PATH", binDir)
	out, _, err := runInstaller(t, "shell-init", "zsh", "--check")
	if err != nil {
		t.Fatalf("shell-init --check: %v", err)
	}
	if !strings.Contains(out, "is on PATH") {
		t.Fatalf("expected on-PATH report, got:\n%s", out)
	}
}

func TestShellInit_CheckOffPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	t.Setenv("PATH", t.TempDir())
	out, _, err := runInstaller(t, "shell-init", "zsh", "--check")
	if err != nil {
		t.Fatalf("shell-init --check: %v", err)
	}
	if !strings.Contains(out, "NOT on PATH") || !strings.Contains(out, "shell-init zsh") {
		t.Fatalf("expected off-PATH report with remediation, got:\n%s", out)
	}
}
