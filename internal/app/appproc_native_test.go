//go:build app_native && (darwin || windows)

package app

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Obedience-Corp/festival-installer/internal/apptest"
)

// TestAppProcessScanNative starts a child from the platform's App layout and
// checks the real process scan finds it, then stops finding it once killed.
func TestAppProcessScanNative(t *testing.T) {
	apptest.RequireHostedRunner(t)
	root := t.TempDir()
	var scope appRunScope
	var exe string
	switch runtime.GOOS {
	case "darwin":
		bundle := filepath.Join(root, "Festival App.app")
		exe = filepath.Join(bundle, "Contents", "MacOS", "festival-app")
		scope = appRunScope{Dirs: []string{bundle}}
	default:
		dir := filepath.Join(root, "Festival App")
		exe = filepath.Join(dir, "festival-app.exe")
		scope = appRunScope{Dirs: []string{dir}}
	}
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	src, err := os.Open(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	dst, err := os.OpenFile(exe, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		t.Fatal(err)
	}
	_ = src.Close()
	_ = dst.Close()
	cmd := exec.Command(exe, "-test.run=^$")
	cmd.Env = append(os.Environ(), envAppTestHelper+"=sleep")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })

	deadline := time.Now().Add(15 * time.Second)
	for {
		err := refuseRunningApp(context.Background(), scope)
		if err != nil && strings.Contains(err.Error(), "pid "+strconv.Itoa(cmd.Process.Pid)+",") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("scan never found pid %d: %v", cmd.Process.Pid, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
	if err := refuseRunningApp(context.Background(), scope); err != nil {
		t.Fatalf("scan still reports a killed child: %v", err)
	}
}
