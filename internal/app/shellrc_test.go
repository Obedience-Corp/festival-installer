package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

func TestPlanShellRCAppend_CancelledContext(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := PlanShellRCAppend(ctx, "zsh"); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v, want context.Canceled in the chain", err)
	}
}

func TestApplyShellRCAppend_CancelledContext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	plan := ShellRCPlan{Shell: "zsh", File: filepath.Join(home, ".zshrc"), Block: "x\n", Appendable: true}
	if err := ApplyShellRCAppend(ctx, plan); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v, want context.Canceled in the chain", err)
	}
	if _, err := os.Stat(plan.File); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a cancelled append must not create the file, err=%v", err)
	}
}

func TestApplyShellRCAppend_RefusesAPlanWithNoEffect(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	plan := ShellRCPlan{Shell: "zsh", File: filepath.Join(home, ".zshrc"), Block: "# advice only\n"}

	err := ApplyShellRCAppend(context.Background(), plan)
	if err == nil {
		t.Fatal("a block that does nothing must be refused, not written")
	}
	if code := errpkg.Code(err); code != "E_SHELL_RC_NO_EFFECT" {
		t.Fatalf("code=%q, want E_SHELL_RC_NO_EFFECT", code)
	}
	if _, statErr := os.Stat(plan.File); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("nothing should have been written, err=%v", statErr)
	}
}

func TestApplyShellRCAppend_EmptyPlanIsRefused(t *testing.T) {
	if err := ApplyShellRCAppend(context.Background(), ShellRCPlan{Appendable: true}); err == nil {
		t.Fatal("a plan with no target file must be refused")
	}
}

func TestShellRCFile_UnsupportedShell(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if _, err := ShellRCFile("nushell"); !errors.Is(err, ErrUnsupportedShell) {
		t.Fatalf("err=%v, want ErrUnsupportedShell", err)
	}
}

func TestShellRCFile_PerShellPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	tests := []struct {
		shell string
		want  string
	}{
		{"zsh", filepath.Join(home, ".zshrc")},
		{"bash", filepath.Join(home, ".bashrc")},
		{"fish", filepath.Join(home, ".config", "fish", "config.fish")},
	}
	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			got, err := ShellRCFile(tt.shell)
			if err != nil {
				t.Fatalf("ShellRCFile(%q): %v", tt.shell, err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestSnippetHasEffect is the guard against stranding a user behind the marker.
// A leftover install renders advice only; writing that under the marker would
// make a later, real append decline as already present.
func TestSnippetHasEffect(t *testing.T) {
	tests := []struct {
		name    string
		snippet string
		want    bool
	}{
		{"empty", "", false},
		{"comments only", "# leftover camp/fest on PATH at /usr/local/bin\n# see https://docs\n", false},
		{"blank lines and comments", "\n\n   # note\n\n", false},
		{"zsh export", "# festival\nexport PATH='/x/bin':\"$PATH\"\n", true},
		{"fish add path", "# festival\nfish_add_path --prepend --global '/x/bin'\n", true},
		{"package source line", "# add:\nsource /usr/share/festival/shell/festival.zsh\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := snippetHasEffect(tt.snippet); got != tt.want {
				t.Fatalf("snippetHasEffect(%q)=%v, want %v", tt.snippet, got, tt.want)
			}
		})
	}
}

func TestPlanShellRCAppend_FreshHomeIsAppendableAndCarriesBothMarkers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FESTIVAL_HOME", filepath.Join(home, "installer"))
	t.Setenv("OBEY_INSTALLER_HOME", "")
	t.Setenv("PATH", t.TempDir())

	plan, err := PlanShellRCAppend(context.Background(), "zsh")
	if err != nil {
		t.Fatalf("PlanShellRCAppend: %v", err)
	}
	if plan.Present {
		t.Fatal("nothing is in the rc file yet")
	}
	if !plan.Appendable {
		t.Fatalf("a fresh home's snippet sets PATH, so it is appendable:\n%s", plan.Block)
	}
	for _, marker := range []string{shellRCMarkerBegin, shellRCMarkerEnd} {
		if !strings.Contains(plan.Block, marker) {
			t.Fatalf("block missing %q:\n%s", marker, plan.Block)
		}
	}

	if err := ApplyShellRCAppend(context.Background(), plan); err != nil {
		t.Fatalf("ApplyShellRCAppend: %v", err)
	}
	again, err := PlanShellRCAppend(context.Background(), "zsh")
	if err != nil {
		t.Fatalf("second plan: %v", err)
	}
	if !again.Present {
		t.Fatal("the marker must be seen on a second plan")
	}
	if err := ApplyShellRCAppend(context.Background(), again); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	body, err := os.ReadFile(again.File)
	if err != nil {
		t.Fatalf("read rc: %v", err)
	}
	if n := strings.Count(string(body), shellRCMarkerBegin); n != 1 {
		t.Fatalf("marker written %d times, want 1:\n%s", n, body)
	}
}

func TestApplyShellRCAppend_KeepsExistingContent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FESTIVAL_HOME", filepath.Join(home, "installer"))
	t.Setenv("OBEY_INSTALLER_HOME", "")
	t.Setenv("PATH", t.TempDir())
	rc := filepath.Join(home, ".zshrc")
	existing := "alias ll='ls -la'\nexport EDITOR=vim\n"
	if err := os.WriteFile(rc, []byte(existing), 0o644); err != nil {
		t.Fatalf("seed rc: %v", err)
	}

	plan, err := PlanShellRCAppend(context.Background(), "zsh")
	if err != nil {
		t.Fatalf("PlanShellRCAppend: %v", err)
	}
	if err := ApplyShellRCAppend(context.Background(), plan); err != nil {
		t.Fatalf("ApplyShellRCAppend: %v", err)
	}
	body, err := os.ReadFile(rc)
	if err != nil {
		t.Fatalf("read rc: %v", err)
	}
	if !strings.HasPrefix(string(body), existing) {
		t.Fatalf("append must not disturb what was already there:\n%s", body)
	}
}
