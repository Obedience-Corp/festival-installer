package launch

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// execCommand is a seam for tests (defaults to exec.Command).
var execCommand = exec.Command

func TestRun_Success(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "tool")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FESTIVAL_HOME", t.TempDir())
	t.Setenv("PATH", dir)

	res := Run(context.Background(), Spec{Tool: "tool", Args: nil, Title: "tool"})
	if res.Err != nil {
		t.Fatalf("Run: %v", res.Err)
	}
	if !res.Started {
		t.Fatal("expected Started")
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit %d", res.ExitCode)
	}
	if filepath.Base(res.Path) != "tool" {
		t.Fatalf("path %q", res.Path)
	}
}

func TestRun_NonZeroExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "tool")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FESTIVAL_HOME", t.TempDir())
	t.Setenv("PATH", dir)

	res := Run(context.Background(), Spec{Tool: "tool"})
	if !res.Started {
		t.Fatal("expected Started on non-zero exit")
	}
	if res.ExitCode != 3 {
		t.Fatalf("want exit 3, got %d err=%v", res.ExitCode, res.Err)
	}
	if res.Signal != "" {
		t.Fatalf("unexpected signal %q", res.Signal)
	}
}

func TestRun_HubEnvAndManagedPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture")
	}
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	// Script on PATH (also written into managed bin so resolve prefers managed).
	script := filepath.Join(bin, "tool")
	body := "#!/bin/sh\nprintf 'HUB=%s\\n' \"$FESTIVAL_HUB\"\nprintf 'PATH0=%s\\n' \"${PATH%%:*}\"\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FESTIVAL_HOME", home)
	// Put a decoy first on PATH; managed bin should still be prepended for the child.
	decoy := t.TempDir()
	t.Setenv("PATH", decoy+string(os.PathListSeparator)+bin)

	// Capture stdout via a wrapper that writes to a file. Run uses os.Stdout.
	// Instead assert by running a tool that exits non-zero with message in stderr...
	// Simpler: use Spec with a script that writes marker file.
	marker := filepath.Join(home, "env.out")
	script2 := filepath.Join(bin, "envtool")
	body2 := "#!/bin/sh\nprintf 'HUB=%s\\n' \"$FESTIVAL_HUB\" > \"$1\"\nprintf 'PATH0=%s\\n' \"${PATH%%:*}\" >> \"$1\"\n"
	if err := os.WriteFile(script2, []byte(body2), 0o755); err != nil {
		t.Fatal(err)
	}
	res := Run(context.Background(), Spec{Tool: "envtool", Args: []string{marker}})
	if res.Err != nil {
		t.Fatalf("Run: %v", res.Err)
	}
	raw, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	out := string(raw)
	if !strings.Contains(out, "HUB=1") {
		t.Fatalf("FESTIVAL_HUB not set in child:\n%s", out)
	}
	if !strings.Contains(out, "PATH0="+bin) {
		t.Fatalf("managed bin not prepended to PATH:\n%s", out)
	}
}

func TestRun_SignalInterrupt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix signals")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	// Reliable self-SIGINT (shell kill-$$ is inconsistent across sh implementations).
	const body = `package main
import ("os"; "syscall"; "time")
func main() {
	_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	time.Sleep(2 * time.Second)
}
`
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := filepath.Join(dir, "tool")
	build := execCommand("go", "build", "-o", tool, src)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build helper: %v\n%s", err, out)
	}
	t.Setenv("FESTIVAL_HOME", t.TempDir())
	t.Setenv("PATH", dir)

	res := Run(context.Background(), Spec{Tool: "tool"})
	if !res.Started {
		t.Fatalf("signalled child must count as Started, err=%v", res.Err)
	}
	if res.ExitCode < 0 {
		t.Fatalf("signalled exit must not use start-failure sentinel -1, got %d", res.ExitCode)
	}
	if res.Signal == "" && res.ExitCode != 130 {
		t.Fatalf("want signal metadata or exit 130, got exit=%d signal=%q err=%v", res.ExitCode, res.Signal, res.Err)
	}
}

func TestRun_ResolveFailureNotStarted(t *testing.T) {
	t.Setenv("FESTIVAL_HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	res := Run(context.Background(), Spec{Tool: "definitely-missing-tool-xyz"})
	if res.Started {
		t.Fatal("resolve failure must not set Started")
	}
	if res.ExitCode != -1 {
		t.Fatalf("exit %d", res.ExitCode)
	}
	if res.Err == nil {
		t.Fatal("expected error")
	}
}

func TestDetectCampaignRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".campaign"), 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "projects", "x")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	got := DetectCampaignRoot(nested)
	if got != root {
		t.Fatalf("DetectCampaignRoot=%q want %q", got, root)
	}
	if DetectCampaignRoot(t.TempDir()) != "" {
		t.Fatal("expected empty outside campaign")
	}
}

func TestWantsCampaignRoot(t *testing.T) {
	if !WantsCampaignRoot(Spec{Tool: "camp", Args: []string{"wi"}}) {
		t.Fatal("camp wi")
	}
	if WantsCampaignRoot(Spec{Tool: "camp", Args: []string{"version"}}) {
		t.Fatal("camp version should not force campaign root")
	}
	if !WantsCampaignRoot(Spec{Tool: "fest", Args: []string{"list"}}) {
		t.Fatal("fest list")
	}
}

func TestPrependPath(t *testing.T) {
	env := []string{"PATH=/usr/bin", "HOME=/tmp"}
	got := prependPath(env, "/managed/bin")
	if got[0] != "PATH=/managed/bin"+string(os.PathListSeparator)+"/usr/bin" {
		t.Fatalf("got %q", got[0])
	}
}
