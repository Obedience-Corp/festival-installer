package launch

import (
	"context"
	"os"
	"os/exec"
	"os/signal"
	"strings"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
	"github.com/Obedience-Corp/obey-installer/internal/state"
)

// HubEnvKey is set in the child environment so products may detect hub launch.
const HubEnvKey = "FESTIVAL_HUB"

// Run resolves Spec.Tool, runs it with Spec.Args on the current TTY, and waits
// until the child exits. Stdin/stdout/stderr are the process stdio so interactive
// camp/fest TUIs work. The caller must leave the hub alt-screen before Run.
//
// While the child runs the hub ignores SIGINT so Ctrl+C aborts the child (same
// foreground process group) without killing festival — design AC: quit child →
// back in hub. Interactive handoff does not use CommandContext (which would
// SIGKILL the child on context cancel).
func Run(ctx context.Context, spec Spec) Result {
	if err := ctx.Err(); err != nil {
		return Result{ExitCode: -1, Err: err}
	}
	if strings.TrimSpace(spec.Tool) == "" {
		return Result{ExitCode: -1, Err: errpkg.New("E_LAUNCH_TOOL", "empty tool name")}
	}

	path, err := Resolve(ctx, spec.Tool)
	if err != nil {
		return Result{ExitCode: -1, Err: err}
	}

	args := append([]string(nil), spec.Args...)
	// Prefer explicit Spec.Dir; else campaign root for campaign-scoped tools.
	dir := spec.Dir
	if dir == "" && WantsCampaignRoot(spec) {
		if root := DetectCampaignRoot(""); root != "" {
			dir = root
		}
	}

	cmd := exec.Command(path, args...) //nolint:gosec // path from Resolve; args from curated catalog
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = childEnv()

	// Catch SIGINT in the parent so Ctrl+C does not kill festival while the
	// child runs. Prefer Notify over Ignore: SIG_IGN is inherited across exec
	// and would silence Ctrl+C in the child too; Notify handlers reset to
	// default in the child image (POSIX execve).
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)

	err = cmd.Run()
	// Drain a SIGINT that arrived while waiting so it does not hit the hub later.
	select {
	case <-sigCh:
	default:
	}
	res := Result{Path: path}
	if err == nil {
		res.Started = true
		return res
	}
	code, sig, started := classifyExit(err)
	res.ExitCode = code
	res.Signal = sig
	res.Started = started
	if started {
		res.Err = err
		return res
	}
	res.ExitCode = -1
	res.Err = errpkg.Wrap("E_LAUNCH_RUN", err, "run "+spec.Tool)
	return res
}

func childEnv() []string {
	env := os.Environ()
	// Prefer managed bin so the child suite matches the hub's install.
	if binDir, err := state.BinDir(context.Background()); err == nil {
		env = prependPath(env, binDir)
	}
	env = setEnv(env, HubEnvKey, "1")
	return env
}

func prependPath(env []string, dir string) []string {
	const prefix = "PATH="
	for i, e := range env {
		if strings.HasPrefix(e, prefix) {
			env[i] = prefix + dir + string(os.PathListSeparator) + strings.TrimPrefix(e, prefix)
			return env
		}
	}
	return append(env, prefix+dir)
}

func setEnv(env []string, key, val string) []string {
	p := key + "="
	for i, e := range env {
		if strings.HasPrefix(e, p) {
			env[i] = p + val
			return env
		}
	}
	return append(env, p+val)
}
