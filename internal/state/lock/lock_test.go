package lock_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Obedience-Corp/obey-installer/internal/state/lock"
)

// envHelperMode is set when the test binary re-executes itself to play
// the role of a separate lock-holding process.
const envHelperMode = "OBEY_LOCK_TEST_HELPER"

func TestMain(m *testing.M) {
	if mode := os.Getenv(envHelperMode); mode != "" {
		runHelper(mode, os.Getenv("OBEY_LOCK_TEST_HOME"))
		return
	}
	os.Exit(m.Run())
}

// runHelper is called when the test binary is re-executed as a subprocess.
// "hold" acquires the lock and sleeps; "crash" acquires then exits without
// releasing.
func runHelper(mode, home string) {
	l, err := lock.NewFileLock(home)
	if err != nil {
		fmt.Fprintln(os.Stderr, "helper NewFileLock:", err)
		os.Exit(2)
	}
	ctx := context.Background()
	rel, err := l.Acquire(ctx, 5*time.Second)
	if err != nil {
		fmt.Fprintln(os.Stderr, "helper Acquire:", err)
		os.Exit(3)
	}
	fmt.Println("acquired")
	switch mode {
	case "hold":
		time.Sleep(500 * time.Millisecond)
		_ = rel()
		os.Exit(0)
	case "crash":
		os.Exit(1) // never call rel; kernel must release
	default:
		fmt.Fprintln(os.Stderr, "unknown helper mode:", mode)
		os.Exit(4)
	}
}

func spawnHelper(t *testing.T, mode, home string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestMain$")
	cmd.Env = append(os.Environ(),
		envHelperMode+"="+mode,
		"OBEY_LOCK_TEST_HOME="+home,
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	// Wait for the helper to print "acquired".
	buf := make([]byte, 64)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		n, _ := stdout.Read(buf)
		if n > 0 && strings.Contains(string(buf[:n]), "acquired") {
			return cmd
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	t.Fatal("helper never printed 'acquired'")
	return nil
}

func TestAcquireRelease_HappyPath(t *testing.T) {
	home := t.TempDir()
	l, err := lock.NewFileLock(home)
	if err != nil {
		t.Fatalf("NewFileLock: %v", err)
	}
	ctx := context.Background()

	rel, err := l.Acquire(ctx, time.Second)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if err := rel(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if err := rel(); err != nil {
		t.Fatalf("second Release: %v", err)
	}
}

func TestSecondAcquireBlocksUntilRelease(t *testing.T) {
	home := t.TempDir()
	cmd := spawnHelper(t, "hold", home)
	defer func() { _ = cmd.Process.Kill() }()

	l, err := lock.NewFileLock(home)
	if err != nil {
		t.Fatalf("NewFileLock: %v", err)
	}

	if _, err := l.Acquire(context.Background(), 100*time.Millisecond); !errors.Is(err, lock.ErrLockTimeout) {
		t.Fatalf("expected ErrLockTimeout while helper holds lock, got %v", err)
	}

	if err := cmd.Wait(); err != nil {
		t.Fatalf("helper exited non-zero: %v", err)
	}

	rel, err := l.Acquire(context.Background(), 2*time.Second)
	if err != nil {
		t.Fatalf("Acquire after helper release: %v", err)
	}
	_ = rel()
}

func TestAcquire_ContextCancel(t *testing.T) {
	home := t.TempDir()
	cmd := spawnHelper(t, "hold", home)
	defer func() { _ = cmd.Process.Kill() }()
	defer func() { _ = cmd.Wait() }()

	l, err := lock.NewFileLock(home)
	if err != nil {
		t.Fatalf("NewFileLock: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	if _, err := l.Acquire(ctx, 5*time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestCrashRecovery_KernelReleasesLock(t *testing.T) {
	home := t.TempDir()
	cmd := spawnHelper(t, "crash", home)
	// Helper calls os.Exit(1) immediately after acquiring. Wait for that.
	_ = cmd.Wait()

	l, err := lock.NewFileLock(home)
	if err != nil {
		t.Fatalf("NewFileLock: %v", err)
	}
	rel, err := l.Acquire(context.Background(), 2*time.Second)
	if err != nil {
		t.Fatalf("expected lock available after helper crash, got %v", err)
	}
	_ = rel()
}

func TestFilePermissions(t *testing.T) {
	home := t.TempDir()
	l, err := lock.NewFileLock(home)
	if err != nil {
		t.Fatalf("NewFileLock: %v", err)
	}
	ctx := context.Background()

	rel, err := l.Acquire(ctx, time.Second)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer func() { _ = rel() }()

	locksDir := filepath.Join(home, "locks")
	dirInfo, err := os.Stat(locksDir)
	if err != nil {
		t.Fatalf("stat locks dir: %v", err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("locks dir mode = %o, want 0700", dirInfo.Mode().Perm())
	}

	fileInfo, err := os.Stat(filepath.Join(locksDir, "installer.lock"))
	if err != nil {
		t.Fatalf("stat lock file: %v", err)
	}
	if fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("lock file mode = %o, want 0600", fileInfo.Mode().Perm())
	}
}
