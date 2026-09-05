package cli_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/cli"
)

func TestBareInvocation_FirstRunNamesTheThreeSteps(t *testing.T) {
	t.Setenv("FESTIVAL_HOME", t.TempDir())
	t.Setenv("OBEY_INSTALLER_HOME", "")
	t.Setenv("PATH", t.TempDir())
	t.Setenv("SHELL", "/bin/zsh")

	var out, errOut bytes.Buffer
	helpCalled := false
	err := cli.BareInvocation(context.Background(), &out, &errOut, func() error {
		helpCalled = true
		return nil
	})
	if err != nil {
		t.Fatalf("BareInvocation: %v", err)
	}
	if helpCalled {
		t.Fatalf("a first run must not fall through to the help dump:\n%s", out.String())
	}
	for _, want := range []string{
		"festival is not set up on this machine yet",
		"festival install festival",
		`eval "$(festival shell-init zsh)"`,
		"festival browse",
		"festival doctor",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("guidance missing %q:\n%s", want, out.String())
		}
	}
}

func TestBareInvocation_SetUpHomePrintsHelp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	t.Setenv("OBEY_INSTALLER_HOME", "")
	t.Setenv("PATH", t.TempDir())
	writeCleanReceipt(t, home)

	var out, errOut bytes.Buffer
	helpCalled := false
	err := cli.BareInvocation(context.Background(), &out, &errOut, func() error {
		helpCalled = true
		return nil
	})
	if err != nil {
		t.Fatalf("BareInvocation: %v", err)
	}
	if !helpCalled {
		t.Fatal("a set-up home keeps the help dump")
	}
	if strings.Contains(out.String(), "not set up on this machine") {
		t.Fatalf("a set-up home must not get onboarding:\n%s", out.String())
	}
	if !strings.Contains(errOut.String(), "no interactive terminal detected") {
		t.Fatalf("expected the no-terminal note on stderr:\n%s", errOut.String())
	}
}

func TestFirstRunGuidance_FishGetsAFishLine(t *testing.T) {
	got := cli.FirstRunGuidance("fish")
	if !strings.Contains(got, "festival shell-init fish | source") {
		t.Fatalf("fish users need a line they can paste:\n%s", got)
	}
	if strings.Contains(got, "eval ") {
		t.Fatalf("fish has no eval:\n%s", got)
	}
}

func TestShellFromEnv(t *testing.T) {
	tests := []struct {
		env  string
		want string
	}{
		{"/bin/zsh", "zsh"},
		{"/usr/local/bin/bash", "bash"},
		{"/opt/homebrew/bin/fish", "fish"},
		{"/usr/bin/nu", "zsh"},
		{"", "zsh"},
	}
	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			t.Setenv("SHELL", tt.env)
			if got := cli.ShellFromEnv(); got != tt.want {
				t.Fatalf("ShellFromEnv()=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestShellInitAppend_YesFlagIsRequiredWithAppend(t *testing.T) {
	t.Setenv("FESTIVAL_HOME", t.TempDir())
	t.Setenv("OBEY_INSTALLER_HOME", "")
	_, _, err := runInstaller(t, "shell-init", "zsh", "--yes")
	if err == nil {
		t.Fatal("--yes without --append must be rejected")
	}
}

func TestShellInitAppend_NoTerminalWritesNothing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FESTIVAL_HOME", filepath.Join(home, "installer"))
	t.Setenv("OBEY_INSTALLER_HOME", "")
	t.Setenv("PATH", t.TempDir())

	out, _, err := runInstaller(t, "shell-init", "zsh", "--append")
	if err != nil {
		t.Fatalf("shell-init --append: %v", err)
	}
	if !strings.Contains(out, "no terminal to ask on") {
		t.Fatalf("expected the no-terminal explanation:\n%s", out)
	}
	if !strings.Contains(out, "not appending") {
		t.Fatalf("expected the paste-it-yourself fallback:\n%s", out)
	}
	if _, statErr := os.Stat(filepath.Join(home, ".zshrc")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("an rc file must not be created without consent, err=%v", statErr)
	}
}

func TestShellInitAppend_YesWritesOnceAndIsIdempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FESTIVAL_HOME", filepath.Join(home, "installer"))
	t.Setenv("OBEY_INSTALLER_HOME", "")
	t.Setenv("PATH", t.TempDir())
	rc := filepath.Join(home, ".zshrc")

	if _, _, err := runInstaller(t, "shell-init", "zsh", "--append", "--yes"); err != nil {
		t.Fatalf("first append: %v", err)
	}
	first := readFileString(t, rc)
	if strings.Count(first, ">>> festival shell integration >>>") != 1 {
		t.Fatalf("expected one guard marker:\n%s", first)
	}
	if !strings.Contains(first, "export PATH=") {
		t.Fatalf("the appended block must carry the PATH line:\n%s", first)
	}

	out, _, err := runInstaller(t, "shell-init", "zsh", "--append", "--yes")
	if err != nil {
		t.Fatalf("second append: %v", err)
	}
	if !strings.Contains(out, "already present") {
		t.Fatalf("a second run must say the block is already there:\n%s", out)
	}
	if second := readFileString(t, rc); second != first {
		t.Fatalf("a second append changed the rc file:\nbefore:\n%s\nafter:\n%s", first, second)
	}
}

func TestShellInitAppend_MarkerMatchesInstallScript(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FESTIVAL_HOME", filepath.Join(home, "installer"))
	t.Setenv("OBEY_INSTALLER_HOME", "")
	t.Setenv("PATH", t.TempDir())

	out, _, err := runInstaller(t, "shell-init", "bash", "--append", "--yes")
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	body := readFileString(t, filepath.Join(home, ".bashrc"))
	for _, marker := range []string{
		"# >>> festival shell integration >>>",
		"# <<< festival shell integration <<<",
	} {
		if !strings.Contains(body, marker) {
			t.Fatalf("rc block missing %q:\n%s", marker, body)
		}
		if !strings.Contains(out, marker) {
			t.Fatalf("the block shown to the user must match what is written:\n%s", out)
		}
	}
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
