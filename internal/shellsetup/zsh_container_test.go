//go:build container_fs

package shellsetup

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestZshCompletionPolicyAndLegacyHelper(t *testing.T) {
	shell, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh unavailable")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ZDOTDIR", home)
	t.Setenv("FESTIVAL_INTEGRATION_PROBE", "")
	// An autoloadable compinit records the security policy requested by the
	// generated helper, then initializes the real completion implementation.
	real, err := Run(context.Background(), shell, "-fc", "print -r -- $^fpath/compinit(N[1])")
	if err != nil {
		t.Fatal(err)
	}
	real = strings.TrimSpace(real)
	if !filepath.IsAbs(real) {
		t.Fatalf("compinit path: %q", real)
	}
	shim := "print -r -- POLICY:$*\nsource " + Quote("zsh", real) + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(home, "compinit"), []byte(shim), 0600); err != nil {
		t.Fatal(err)
	}
	rc := "fpath=(" + Quote("zsh", home) + " $fpath)\n" + Header("zsh", nil) + "fgo() { :; }\n_fest_complete() { :; }\ncomplete -F _fest_complete fest\n"
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte(rc), 0600); err != nil {
		t.Fatal(err)
	}
	for _, probe := range []bool{false, true} {
		run := Run
		want := "POLICY:-D\n"
		if probe {
			run = RunProbe
			want = "POLICY:-i -D\n"
		}
		out, err := run(context.Background(), shell, "-ic", "(( $+functions[compdef] )) || exit 24; typeset -f fgo >/dev/null || exit 25; [[ ${_comps[fest]} = _bash_complete* ]] || exit 26; echo LEGACY_READY")
		if err != nil || !strings.Contains(out, want) || !strings.Contains(out, "LEGACY_READY") {
			t.Fatalf("probe=%v output=%q err=%v", probe, out, err)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".zcompdump")); !os.IsNotExist(err) {
		t.Fatalf("unexpected compdump: %v", err)
	}
}

func TestAppToolDirectorySurvivesShellStartup(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			binary, err := exec.LookPath(shell)
			if err != nil {
				t.Skip(shell + " unavailable")
			}
			home := t.TempDir()
			tools := filepath.Join(home, "camp parent")
			selected := filepath.Join(home, "selected fest parent")
			shim := filepath.Join(home, "app exact tools")
			for _, dir := range []string{tools, selected, shim} {
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			for file, result := range map[string]string{
				filepath.Join(tools, "camp"):    "camp",
				filepath.Join(tools, "fest"):    "stale-fest",
				filepath.Join(selected, "fest"): "selected-fest",
			} {
				if err := os.WriteFile(file, []byte("#!/bin/sh\necho "+result+"\n"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			for name, target := range map[string]string{"camp": filepath.Join(tools, "camp"), "fest": filepath.Join(selected, "fest")} {
				if err := os.Symlink(target, filepath.Join(shim, name)); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("HOME", home)
			t.Setenv("ZDOTDIR", home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
			t.Setenv("PATH", shim+":/usr/bin:/bin")
			profile := filepath.Join(home, "."+shell+"rc")
			if shell == "fish" {
				profile = filepath.Join(home, ".config", "fish", "config.fish")
			}
			if err := os.MkdirAll(filepath.Dir(profile), 0700); err != nil {
				t.Fatal(err)
			}
			helper := filepath.Join(home, "helpers")
			if err := os.WriteFile(helper, []byte(Header(shell, []string{filepath.Join(tools, "camp"), filepath.Join(selected, "fest")})), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(profile, []byte(Source(shell, helper)), 0600); err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct{ override, want string }{
				{shim, "selected-fest"}, {"", "stale-fest"}, {"relative/path", "stale-fest"}, {filepath.Join(home, "absent"), "stale-fest"},
			} {
				t.Setenv("FESTIVAL_APP_TOOL_BIN", tc.override)
				out, err := Run(context.Background(), binary, "-ic", "fest")
				if err != nil || strings.TrimSpace(out) != tc.want {
					t.Fatalf("override=%q: output=%q err=%v", tc.override, out, err)
				}
			}
		})
	}
}
