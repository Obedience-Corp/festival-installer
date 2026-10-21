package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestIntegrationUsesAbsoluteLoginShellOutsideGUIPath(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			executable, err := exec.LookPath(shell)
			if err != nil {
				t.Skip(shell + " unavailable")
			}
			executable, err = filepath.Abs(executable)
			if err != nil {
				t.Fatal(err)
			}
			home, _ := integrationFixture(t, shell)
			// A same-named shell on PATH must not substitute for the login executable.
			login := filepath.Join(home, "brew", "bin", shell)
			if err := os.MkdirAll(filepath.Dir(login), 0700); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(home, "login-shell-used")
			wrapper := "#!/bin/sh\nprintf used >> '" + marker + "'\nexec '" + executable + "' \"$@\"\n"
			if err := os.WriteFile(login, []byte(wrapper), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("SHELL", login)
			r, err := ReconcileIntegrations(context.Background(), IntegrationOptions{})
			if err != nil || !r.Ready {
				t.Fatalf("%+v %v", r, err)
			}
			if _, err := os.Stat(marker); err != nil {
				t.Fatal("did not execute login shell", err)
			}
			for _, c := range r.Checks {
				if c.ID == "fresh_shell" && c.Path != login {
					t.Fatalf("wrong shell: %+v", c)
				}
			}
		})
	}
}

func TestIntegrationMissingLoginExecutableDoesNotWriteProfilesOrHelper(t *testing.T) {
	home, managed := integrationFixture(t, "bash")
	t.Setenv("SHELL", filepath.Join(home, "uninstalled", "bash"))
	r, err := ReconcileIntegrations(context.Background(), IntegrationOptions{})
	if err != nil || r.Ready {
		t.Fatalf("%+v %v", r, err)
	}
	found := false
	for _, c := range r.Checks {
		if c.ID == "shell" && c.Status == "unavailable" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing executable not distinguished: %+v", r)
	}
	for _, path := range []string{filepath.Join(home, ".bashrc"), filepath.Join(managed, "shell", "festival.bash")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("wrote %s: %v", path, err)
		}
	}
}

func TestIntegrationRootNeverTouchesStateProfilesOrHooks(t *testing.T) {
	home, managed := integrationFixture(t, "bash")
	profile := filepath.Join(home, ".bashrc")
	original := "# user-owned rc\n"
	if err := os.WriteFile(profile, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	for _, opts := range []IntegrationOptions{{}, {Check: true}, {Disable: true}, {Enable: true}} {
		r, err := reconcileIntegrations(context.Background(), opts, 0)
		if err != nil || r.Ready || r.Changed || len(r.Checks) != 1 || !r.Checks[0].Repairable || !strings.Contains(r.Checks[0].Message, "without sudo") {
			t.Fatalf("%+v %v", r, err)
		}
	}
	got, _ := os.ReadFile(profile)
	if string(got) != original {
		t.Fatal("root rewrote profile")
	}
	for _, path := range []string{filepath.Join(managed, "integrations.json"), filepath.Join(managed, "shell"), filepath.Join(home, "hooks-current")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("root wrote %s: %v", path, err)
		}
	}
}

func TestWindowsAutoSetupFindsEachInstalledEditionOutsidePATH(t *testing.T) {
	roots := map[string]string{"ProgramFiles": filepath.Join(t.TempDir(), "Program Files"), "SystemRoot": filepath.Join(t.TempDir(), "Windows")}
	pwsh := filepath.Join(roots["ProgramFiles"], "PowerShell", "7", "pwsh.exe")
	legacy := filepath.Join(roots["SystemRoot"], "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	for _, available := range [][]string{{pwsh, legacy}, {pwsh}, {legacy}, {}} {
		lookup := func(path string) (string, error) {
			for _, installed := range available {
				if path == installed {
					return installed, nil
				}
			}
			return "", errors.New("not found")
		}
		got := windowsIntegrationShells(lookup, func(key string) string { return roots[key] })
		if strings.Join(got, "|") != strings.Join(available, "|") {
			t.Fatalf("got %v, want %v", got, available)
		}
	}
}

func TestMacGitShimNeverRunsWhenDeveloperToolsAreMissing(t *testing.T) {
	for _, selected := range []bool{false, true} {
		var calls []string
		run := func(_ context.Context, path string, args ...string) (string, error) {
			calls = append(calls, path+" "+strings.Join(args, " "))
			if path == "/usr/bin/xcode-select" {
				if !selected {
					return "", errors.New("no active developer directory")
				}
				return "/Library/Developer/CommandLineTools\n", nil
			}
			return "git version test", nil
		}
		if ready := gitExecutableReady(context.Background(), "darwin", "/usr/bin/git", run); ready != selected {
			t.Fatalf("ready=%v tools=%v", ready, selected)
		}
		want := []string{"/usr/bin/xcode-select -p"}
		if selected {
			want = append(want, "/usr/bin/git --version")
		}
		if !reflect.DeepEqual(calls, want) {
			t.Fatalf("ran prompting shim: %v", calls)
		}
	}
}
