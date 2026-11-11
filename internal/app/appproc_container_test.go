//go:build container_fs && linux

package app

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

// startSleeper copies the test binary to exePath and runs it in "sleep" helper
// mode. It is killed by the pid it started, never by name.
func startSleeper(t *testing.T, exePath string, extraEnv ...string) *exec.Cmd {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(exePath), 0o755); err != nil {
		t.Fatal(err)
	}
	src, err := os.Open(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	dst, err := os.OpenFile(exePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		t.Fatal(err)
	}
	_ = src.Close()
	if err := dst.Close(); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exePath, "-test.run=^$")
	cmd.Env = append(os.Environ(), append([]string{envAppTestHelper + "=sleep"}, extraEnv...)...)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
	return cmd
}

// waitRunning polls until the scan reports the child, so exec has finished.
func waitRunning(t *testing.T, scope appRunScope, pid int) error {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		err := refuseRunningApp(context.Background(), scope)
		if err != nil && strings.Contains(err.Error(), "pid "+strconv.Itoa(pid)+",") {
			return err
		}
		if time.Now().After(deadline) {
			t.Fatalf("scan never reported pid %d (last result: %v)", pid, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestAppProcessScanFindsChildInLocation(t *testing.T) {
	bundle := filepath.Join(t.TempDir(), "Festival App.app")
	scope := appRunScope{Dirs: []string{bundle}}
	cmd := startSleeper(t, filepath.Join(bundle, "Contents", "MacOS", "festival-activity-engine"))
	err := waitRunning(t, scope, cmd.Process.Pid)
	if code := codeOf(err); code != "E_APP_RUNNING" {
		t.Fatalf("code = %s: %v", code, err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_, _ = cmd.Process.Wait()
	if err := refuseRunningApp(context.Background(), scope); err != nil {
		t.Fatalf("scan still reports a killed child: %v", err)
	}
}

func TestAppProcessScanAppImage(t *testing.T) {
	root := t.TempDir()
	image := filepath.Join(root, ".local", "bin", "festival-app")
	scope := appRunScope{Exes: []string{image}}
	byExe := startSleeper(t, image)
	_ = waitRunning(t, scope, byExe.Process.Pid)
	_ = byExe.Process.Kill()
	_, _ = byExe.Process.Wait()
	if err := refuseRunningApp(context.Background(), scope); err != nil {
		t.Fatalf("still running after kill: %v", err)
	}

	byEnv := startSleeper(t, filepath.Join(root, "mount", "usr", "bin", "runtime"), "APPIMAGE="+image)
	_ = waitRunning(t, scope, byEnv.Process.Pid)
}

func TestAppProcessScanDeletedExecutable(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "bin", "festival-app")
	scope := appRunScope{Exes: []string{exe}}
	cmd := startSleeper(t, exe)
	_ = waitRunning(t, scope, cmd.Process.Pid)
	if err := os.Remove(exe); err != nil {
		t.Fatal(err)
	}
	if err := refuseRunningApp(context.Background(), scope); codeOf(err) != "E_APP_RUNNING" {
		t.Fatalf("deleted executable no longer matched: %v", err)
	}
}

func TestAppDebProbeWithoutPackage(t *testing.T) {
	d, err := probeDeb(context.Background(), runDpkgQuery)
	if err != nil || d.Installed {
		t.Fatalf("probeDeb = %+v %v, want not installed", d, err)
	}
}

func codeOf(err error) string { return errpkg.Code(err) }
