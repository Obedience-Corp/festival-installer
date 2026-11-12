//go:build !windows

package app

import (
	"context"
	"testing"
	"time"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

func TestAppCommandRunnerReportsExitCode(t *testing.T) {
	ctx := context.Background()
	res, err := runAppCommand(ctx, 10*time.Second, "", nil, "/bin/sh", "-c", "echo out; echo err >&2; exit 3")
	if err != nil || res.ExitCode != 3 || res.Stdout != "out\n" || res.Stderr != "err\n" {
		t.Fatalf("got %+v %v", res, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := runAppCommand(cancelled, 10*time.Second, "", nil, "/bin/sh", "-c", "sleep 5"); errpkg.Code(err) != "E_INSTALL_CTX" {
		t.Fatalf("cancelled: %v", err)
	}
	if _, err := runAppCommand(ctx, 100*time.Millisecond, "", nil, "/bin/sh", "-c", "sleep 5"); errpkg.Code(err) != "E_INSTALL_CTX" {
		t.Fatalf("timeout: %v", err)
	}
	res, err = runAppCommand(ctx, 10*time.Second, "/", nil, "/bin/sh", "-c", "pwd")
	if err != nil || res.Stdout != "/\n" {
		t.Fatalf("dir: %+v %v", res, err)
	}
}
