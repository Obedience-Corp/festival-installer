//go:build container_fs || app_native

package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/Obedience-Corp/festival-installer/internal/state/lock"
)

const envAppTestHelper = "FESTIVAL_APP_TEST_HELPER"

func TestMain(m *testing.M) {
	switch os.Getenv(envAppTestHelper) {
	case "":
		os.Exit(m.Run())
	case "lock-probe":
		os.Exit(appLockProbe(os.Getenv("FESTIVAL_HOME")))
	default:
		fmt.Fprintln(os.Stderr, "unknown helper mode")
		os.Exit(4)
	}
}

// appLockProbe exits 0 when it takes the installer lock within 200ms,
// 3 on lock.ErrLockTimeout, 2 on any other error.
func appLockProbe(home string) int {
	l, err := lock.NewFileLock(home)
	if err != nil {
		fmt.Fprintln(os.Stderr, "probe NewFileLock:", err)
		return 2
	}
	release, err := l.Acquire(context.Background(), 200*time.Millisecond)
	if errors.Is(err, lock.ErrLockTimeout) {
		return 3
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "probe Acquire:", err)
		return 2
	}
	_ = release()
	return 0
}

// runAppLockProbe re-executes the test binary as the probe and returns its exit code.
// The lock is per process, so only another process can tell whether it is free.
func runAppLockProbe(t *testing.T, home string) int {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), envAppTestHelper+"=lock-probe", "FESTIVAL_HOME="+home)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	t.Fatalf("run lock probe: %v\n%s", err, out)
	return -1
}
