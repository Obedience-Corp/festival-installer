package launch

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

// captureStopGrace is how long Stop waits for the child to honor SIGINT
// before escalating to SIGKILL.
const captureStopGrace = 2 * time.Second

// Capture runs a launchpad entry with stdout+stderr piped into the hub
// (ModeOneShot and ModeStream) instead of handing over the TTY. Stdin is not
// connected: capture-mode children are non-interactive by contract;
// interactive tools use ModeTUI.
type Capture struct {
	cmd    *exec.Cmd
	chunks chan []byte
	done   chan Result
	exited chan struct{}
	stop   sync.Once
}

// StartCapture resolves spec.Tool and starts it with piped output. The child
// runs in its own process group so Stop can signal the whole tree without
// touching the hub.
func StartCapture(ctx context.Context, spec Spec) (*Capture, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(spec.Tool) == "" {
		return nil, errpkg.New("E_LAUNCH_TOOL", "empty tool name")
	}
	path, err := Resolve(ctx, spec.Tool)
	if err != nil {
		return nil, err
	}

	args := append([]string(nil), spec.Args...)
	dir := spec.Dir
	if dir == "" && WantsCampaignRoot(spec) {
		if root := DetectCampaignRoot(""); root != "" {
			dir = root
		}
	}

	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, errpkg.Wrap("E_LAUNCH_PIPE", err, "create capture pipe")
	}
	cmd := exec.Command(path, args...) //nolint:gosec // path from Resolve; args from curated catalog
	cmd.Stdout = pw
	cmd.Stderr = pw
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = childEnv()
	setCaptureSysProcAttr(cmd)

	if err := cmd.Start(); err != nil {
		_ = pr.Close()
		_ = pw.Close()
		return nil, errpkg.Wrap("E_LAUNCH_RUN", err, "start "+spec.Tool)
	}
	// The parent must close its copy of the write end or the reader never
	// sees EOF when the child exits.
	_ = pw.Close()

	c := &Capture{
		cmd:    cmd,
		chunks: make(chan []byte, 64),
		done:   make(chan Result, 1),
		exited: make(chan struct{}),
	}
	go c.pump(pr, path)
	return c, nil
}

// pump drains the pipe into chunks, then waits on the child and publishes the
// Result. chunks closes before done receives, so Next delivers all output
// before the exit result.
func (c *Capture) pump(pr *os.File, path string) {
	buf := make([]byte, 4096)
	for {
		n, err := pr.Read(buf)
		if n > 0 {
			out := make([]byte, n)
			copy(out, buf[:n])
			c.chunks <- out
		}
		if err != nil {
			break
		}
	}
	_ = pr.Close()
	close(c.chunks)

	res := Result{Path: path, Started: true}
	if err := c.cmd.Wait(); err != nil {
		code, sig, started := classifyExit(err)
		res.ExitCode = code
		res.Signal = sig
		res.Started = started
		res.Err = err
	}
	close(c.exited)
	c.done <- res
}

// Next blocks until the child produces output or exits. Exactly one return is
// set: a non-nil res means all output has been delivered and the capture is
// finished. Next must not be called again after it returns a non-nil res.
func (c *Capture) Next() (chunk []byte, res *Result) {
	if b, ok := <-c.chunks; ok {
		return b, nil
	}
	r := <-c.done
	return nil, &r
}

// Stop asks the child to exit: SIGINT to its process group, then SIGKILL
// after a grace period if it is still running. Safe to call more than once;
// the exit still arrives through Next.
func (c *Capture) Stop() {
	c.stop.Do(func() {
		stopProcess(c.cmd)
		go func() {
			timer := time.NewTimer(captureStopGrace)
			defer timer.Stop()
			select {
			case <-c.exited:
			case <-timer.C:
				killProcess(c.cmd)
			}
		}()
	})
}
