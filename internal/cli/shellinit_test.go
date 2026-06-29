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
	want := `export PATH="` + filepath.Join(home, "bin") + `:$PATH"`
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
	out, _, err := runInstaller(t, "shell-init", "fish")
	if err != nil {
		t.Fatalf("shell-init fish: %v", err)
	}
	if !strings.Contains(out, "fish_add_path") || !strings.Contains(out, filepath.Join(home, "bin")) {
		t.Fatalf("fish snippet wrong:\n%s", out)
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
