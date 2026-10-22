package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/shellsetup"
)

func integrationFixture(t *testing.T, shell string) (string, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Unix shell fixture")
	}
	shellPath, err := exec.LookPath(shell)
	if err != nil {
		t.Skip(shell + " unavailable")
	}
	home := t.TempDir()
	managed := filepath.Join(home, "installer with ' quotes")
	t.Setenv("HOME", home)
	t.Setenv("FESTIVAL_HOME", managed)
	t.Setenv("OBEY_INSTALLER_HOME", "")
	t.Setenv("SHELL", shell)
	t.Setenv("ZDOTDIR", "")
	if err := os.Unsetenv("ZDOTDIR"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	bin := filepath.Join(managed, "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"camp", "fest", "obey"} {
		name := "cgo"
		if tool == "fest" {
			name = "fgo"
		}
		snippet := fmt.Sprintf("%s() { :; }\n", name)
		if shell == "fish" {
			snippet = fmt.Sprintf("function %s\n true\nend\n", name)
		}
		if tool == "obey" {
			snippet = ""
			for _, p := range []string{"grok", "claude", "codex"} {
				if shell == "fish" {
					snippet += fmt.Sprintf("if not functions -q %s\nfunction %s\n command obey wrap %s $argv\nend\nend\n", p, p, p)
				} else {
					snippet += fmt.Sprintf("if ! typeset -f %s >/dev/null 2>&1 && ! alias %s >/dev/null 2>&1; then\nfunction %s { command obey wrap %s \"$@\"; }\nfi\n", p, p, p, p)
				}
			}
		}
		script := "#!/bin/sh\ncase \"$*\" in\n*shell-init*) cat <<'HELPERS'\n" + snippet + "HELPERS\n;;\n"
		if tool == "obey" {
			marker := shellsetup.Quote("sh", filepath.Join(home, "hooks-current"))
			script += "'bridge install all --scope user') touch " + marker + ";;\n'bridge status --json')\nif [ -f " + marker + " ]; then state=current; installed=true; else state=absent; installed=false; fi\nprintf '[{\"provider\":\"claude-code\",\"installed\":%s,\"install_status\":\"%s\"},{\"provider\":\"codex\",\"installed\":%s,\"install_status\":\"%s\"},{\"provider\":\"grok\",\"installed\":%s,\"install_status\":\"%s\"}]\\n' \"$installed\" \"$state\" \"$installed\" \"$state\" \"$installed\" \"$state\"\n;;\n"
		}
		script += "*) echo '" + tool + " version 9.0.0';;\nesac\n"
		if err := os.WriteFile(filepath.Join(bin, tool), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	// Managed resolution still works when a desktop process has no suite PATH.
	// Bash and Zsh live in /bin. Homebrew fish does not, so keep that executable
	// visible without putting the rest of Homebrew on PATH.
	t.Setenv("PATH", "/usr/bin:/bin")
	if _, err := exec.LookPath(shell); err != nil {
		shellBin := filepath.Join(home, "shell-bin")
		if err := os.MkdirAll(shellBin, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(shellPath, filepath.Join(shellBin, filepath.Base(shell))); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", "/usr/bin:/bin:"+shellBin)
	}
	return home, managed
}

func TestIntegrationsFreshRepairAndOptOut(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			home, managed := integrationFixture(t, shell)
			ctx := context.Background()
			check, err := ReconcileIntegrations(ctx, IntegrationOptions{Shell: shell, Check: true})
			if err != nil || check.Ready || check.Changed {
				t.Fatalf("fresh check: %+v %v", check, err)
			}
			if _, err := os.Stat(filepath.Join(managed, "integrations.json")); !os.IsNotExist(err) {
				t.Fatal("read-only check wrote state")
			}
			if _, err := os.Stat(filepath.Join(home, "hooks-current")); !os.IsNotExist(err) {
				t.Fatal("read-only check installed hooks")
			}
			first, err := ReconcileIntegrations(ctx, IntegrationOptions{Shell: shell})
			if err != nil || !first.Ready || !first.Changed || !first.NewTerminal {
				t.Fatalf("apply: %+v %v", first, err)
			}
			second, err := ReconcileIntegrations(ctx, IntegrationOptions{Shell: shell})
			if err != nil || !second.Ready || second.Changed {
				t.Fatalf("repeat: %+v %v", second, err)
			}
			check, err = ReconcileIntegrations(ctx, IntegrationOptions{Shell: shell, Check: true})
			if err != nil || !check.Ready {
				t.Fatalf("postcheck: %+v %v", check, err)
			}
			profiles, _ := integrationProfiles(shell)
			if err := os.WriteFile(profiles[0], []byte("# user setting\n"+shellsetup.Block("# old PATH-only install")), 0o640); err != nil {
				t.Fatal(err)
			}
			repair, err := ReconcileIntegrations(ctx, IntegrationOptions{Shell: shell})
			if err != nil || !repair.Ready || !repair.Changed {
				t.Fatalf("repair: %+v %v", repair, err)
			}
			body, _ := os.ReadFile(profiles[0])
			if !strings.HasPrefix(string(body), "# user setting\n") || strings.Count(string(body), shellsetup.Begin) != 1 {
				t.Fatal("damaged profile")
			}
			disabled, err := ReconcileIntegrations(ctx, IntegrationOptions{Shell: shell, Disable: true})
			if err != nil || !disabled.Disabled || disabled.Ready {
				t.Fatalf("disable: %+v %v", disabled, err)
			}
			body, _ = os.ReadFile(profiles[0])
			if string(body) != "# user setting\n" {
				t.Fatalf("removal changed user content %q", body)
			}
			again, err := ReconcileIntegrations(ctx, IntegrationOptions{Shell: shell})
			if err != nil || !again.Disabled || again.Changed {
				t.Fatalf("lost opt out: %+v %v", again, err)
			}
		})
	}
}

func TestIntegrationsPreserveCustomProviderAndReportConflict(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		for _, kind := range []string{"function", "alias"} {
			t.Run(shell+"/"+kind, func(t *testing.T) {
				integrationFixture(t, shell)
				profile, _ := ShellRCFile(shell)
				custom := "claude() { echo custom-keychain-launch; }\n"
				if shell == "fish" {
					custom = "function claude\n echo custom-keychain-launch\nend\n"
				}
				if kind == "alias" {
					custom = "alias claude='echo custom-keychain-launch'\n"
				}
				// Startup themes may print terminal controls without a final newline.
				custom += "printf '\\033]11;#2F5755\\007'\n"
				if err := os.MkdirAll(filepath.Dir(profile), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(profile, []byte(custom), 0o600); err != nil {
					t.Fatal(err)
				}
				if shell == "bash" {
					// Bash login profiles commonly source the interactive rc file.
					login := filepath.Join(filepath.Dir(profile), ".bash_profile")
					if err := os.WriteFile(login, []byte(". \"$HOME/.bashrc\"\n"), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				for _, check := range []bool{false, true, false} {
					r, err := ReconcileIntegrations(context.Background(), IntegrationOptions{Shell: shell, Check: check})
					if err != nil || r.Ready {
						t.Fatalf("custom conflict not reported: %+v %v", r, err)
					}
					conflict, fresh := false, false
					for _, c := range r.Checks {
						if c.ID == "provider_wrapper" && c.Status == "conflict" && strings.Contains(c.Message, "obey wrap claude") && !c.Repairable {
							conflict = true
							continue
						}
						if c.Status != "ready" && c.Status != "not_required" {
							t.Fatalf("unexpected setup failure: %+v", c)
						}
						fresh = fresh || c.ID == "fresh_shell" && c.Status == "ready"
					}
					if !conflict || !fresh {
						t.Fatalf("missing conflict or successful probe: %+v", r)
					}
					body, _ := os.ReadFile(profile)
					if !strings.HasPrefix(string(body), custom) || strings.Count(string(body), shellsetup.Begin) != 1 {
						t.Fatal("custom launcher or managed block changed")
					}
				}
			})
		}
	}
}

func TestIntegrationsCustomProviderDoesNotHideBrokenStartup(t *testing.T) {
	for _, failure := range []string{"return\n", "exit 7\n"} {
		t.Run(strings.TrimSpace(failure), func(t *testing.T) {
			integrationFixture(t, "zsh")
			profile, _ := ShellRCFile("zsh")
			custom := "claude() { echo custom-keychain-launch; }\n" + failure
			if err := os.WriteFile(profile, []byte(custom), 0o600); err != nil {
				t.Fatal(err)
			}
			r, err := ReconcileIntegrations(context.Background(), IntegrationOptions{Shell: "zsh"})
			if err != nil || r.Ready {
				t.Fatalf("startup failure not reported: %+v %v", r, err)
			}
			for _, c := range r.Checks {
				if c.ID == "fresh_shell" && c.Status == "failed" && c.Repairable {
					return
				}
			}
			t.Fatalf("missing failed startup check: %+v", r)
		})
	}
}

func TestIntegrationProfilesRespectCustomLocations(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ZDOTDIR", filepath.Join(home, "zsh"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	for shell, want := range map[string]string{"zsh": filepath.Join(home, "zsh", ".zshrc"), "fish": filepath.Join(home, "config", "fish", "config.fish")} {
		files, err := integrationProfiles(shell)
		if err != nil || len(files) != 1 || files[0] != want {
			t.Fatalf("%s: %v %v", shell, files, err)
		}
	}
}

func TestIntegrationCleanupAfterRemovingComponents(t *testing.T) {
	_, managed := integrationFixture(t, "bash")
	ctx := context.Background()
	if report, err := ReconcileIntegrations(ctx, IntegrationOptions{}); err != nil || !report.Ready {
		t.Fatalf("setup: %+v %v", report, err)
	}
	for _, tool := range []string{"camp", "fest", "obey"} {
		if err := os.Remove(filepath.Join(managed, "bin", tool)); err != nil {
			t.Fatal(err)
		}
	}
	// Exclude any host installation when proving complete removal.
	commands := t.TempDir()
	for _, tool := range []string{"bash", "cat"} {
		path, err := exec.LookPath(tool)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(path, filepath.Join(commands, tool)); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", commands)
	if report, err := ReconcileIntegrations(ctx, IntegrationOptions{}); err != nil || !report.Ready {
		t.Fatalf("cleanup: %+v %v", report, err)
	}
	files, _ := integrationProfiles("bash")
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil || strings.Contains(string(data), shellsetup.Begin) {
			t.Fatalf("owned block retained: %s %v", data, err)
		}
	}
}

func TestIntegrationUsesZshenvRedirectFromGUILaunch(t *testing.T) {
	home, _ := integrationFixture(t, "zsh")
	custom := filepath.Join(home, "custom zsh")
	if err := os.Mkdir(custom, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".zshenv"), []byte("ZDOTDIR="+shellsetup.Quote("zsh", custom)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := ReconcileIntegrations(context.Background(), IntegrationOptions{})
	if err != nil || !report.Ready {
		t.Fatalf("redirect: %+v %v", report, err)
	}
	if _, err := os.Stat(filepath.Join(custom, ".zshrc")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".zshrc")); !os.IsNotExist(err) {
		t.Fatal("wrote inactive profile")
	}
}

func TestUnsupportedShellCanOptOutAndRemoveTrackedBlocks(t *testing.T) {
	_, _ = integrationFixture(t, "bash")
	ctx := context.Background()
	if report, err := ReconcileIntegrations(ctx, IntegrationOptions{}); err != nil || !report.Ready {
		t.Fatalf("setup: %+v %v", report, err)
	}
	t.Setenv("SHELL", "/bin/nu")
	report, err := ReconcileIntegrations(ctx, IntegrationOptions{Disable: true})
	if err != nil || !report.Disabled {
		t.Fatalf("disable: %+v %v", report, err)
	}
	for _, check := range report.Checks {
		if check.Status != "ready" && check.Status != "not_required" && check.Status != "disabled" {
			t.Fatalf("opt-out blocked: %+v", check)
		}
	}
	files, _ := integrationProfiles("bash")
	for _, path := range files {
		data, _ := os.ReadFile(path)
		if strings.Contains(string(data), shellsetup.Begin) {
			t.Fatal("tracked block remained")
		}
	}
}
