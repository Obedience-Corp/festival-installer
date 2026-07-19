package launch

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
	"github.com/Obedience-Corp/obey-installer/internal/state"
)

// HubEnvKey is set in the child environment so products may detect hub launch.
const HubEnvKey = "FESTIVAL_HUB"

// Run resolves Spec.Tool, runs it with Spec.Args on the current TTY, and waits
// until the child exits. Stdin/stdout/stderr are the process stdio so interactive
// camp/fest TUIs work. The caller must leave the hub alt-screen before Run.
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

	cmd := exec.CommandContext(ctx, path, spec.Args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if spec.Dir != "" {
		cmd.Dir = spec.Dir
	}
	cmd.Env = childEnv()

	err = cmd.Run()
	res := Result{Path: path}
	if err == nil {
		return res
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		res.ExitCode = ee.ExitCode()
		// Non-zero exit from an interactive TUI is common (e.g. user abort);
		// still surface the error so the hub can show a soft banner if desired.
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
