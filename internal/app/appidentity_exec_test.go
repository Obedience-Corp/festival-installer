//go:build !windows

package app

import (
	"context"
	"errors"
	"io/fs"
	"os/exec"
	"testing"
)

func TestRunDpkgQueryReportsExitCodeAndMissingBinary(t *testing.T) {
	ctx := context.Background()
	out, err := runDpkgQuery(ctx, "/bin/sh", "-c", "printf ok; exit 1")
	var exit *exec.ExitError
	if out != "ok" || !errors.As(err, &exit) || exit.ExitCode() != 1 {
		t.Fatalf("got %q %v", out, err)
	}
	if _, err := runDpkgQuery(ctx, "/nonexistent/dpkg-query"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing binary: %v", err)
	}
	d, err := probeDeb(ctx, func(ctx context.Context, _ string, _ ...string) (string, error) {
		return runDpkgQuery(ctx, "/bin/sh", "-c", "exit 1")
	})
	if d.Installed || err != nil {
		t.Fatalf("a real exit 1 must mean not installed: %+v %v", d, err)
	}
}
