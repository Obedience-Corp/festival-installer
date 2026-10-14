//go:build windows && container_fs

package tui

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/app"
)

func TestWindowsTourPathConsentProfileAndReadback(t *testing.T) {
	ctx := context.Background()
	t.Setenv("FESTIVAL_HOME", t.TempDir())
	t.Setenv("OBEY_INSTALLER_HOME", "")
	t.Setenv("SHELL", "")
	basePath := os.Getenv("PATH")
	binDir := filepath.Join(os.Getenv("FESTIVAL_HOME"), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(basePath), strings.ToLower(binDir)) {
		t.Fatalf("test PATH already contains managed bin: %q", basePath)
	}
	profile, err := app.ShellRCFile("powershell")
	if err != nil {
		t.Fatal(err)
	}
	ps := `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`
	profileCmd := exec.Command(ps, "-NoProfile", "-NonInteractive", "-Command", "[Console]::Out.Write($PROFILE.CurrentUserAllHosts)")
	actualProfile, err := profileCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("query PowerShell profile: %v\n%s", err, actualProfile)
	}
	if !strings.EqualFold(filepath.Clean(profile), filepath.Clean(strings.TrimSpace(string(actualProfile)))) {
		t.Fatalf("append target %q differs from PowerShell profile %q", profile, actualProfile)
	}
	original, readErr := os.ReadFile(profile)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		t.Fatal(readErr)
	}
	t.Cleanup(func() {
		if errors.Is(readErr, os.ErrNotExist) {
			_ = os.Remove(profile)
		} else {
			_ = os.WriteFile(profile, original, 0o644)
		}
	})

	m := tourAt(t, app.TourStepPath)
	next, _ := m.handleEnter()
	confirm := next.(model)
	if confirm.screen != screenConfirm || confirm.confirmArg != "powershell" || !strings.Contains(confirm.confirmMsg, profile) {
		t.Fatalf("tour confirmation did not identify PowerShell profile: %+v", confirm)
	}
	if !strings.Contains(confirm.confirmMsg, "$env:PATH = ") {
		t.Fatalf("tour confirmation omitted executable PowerShell PATH line: %q", confirm.confirmMsg)
	}
	if before, err := os.ReadFile(profile); (err == nil && string(before) != string(original)) || (err != nil && !errors.Is(err, readErr)) {
		t.Fatalf("showing confirmation changed profile: %v", err)
	}
	confirm.confirmYes = false
	_, noCmd := confirm.handleEnter()
	if noCmd != nil {
		t.Fatal("declining PowerShell profile append returned a write command")
	}
	if before, err := os.ReadFile(profile); (err == nil && string(before) != string(original)) || (err != nil && !errors.Is(err, readErr)) {
		t.Fatalf("declining changed profile: %v", err)
	}

	next, _ = m.handleEnter()
	confirm = next.(model)
	confirm.confirmYes = true
	_, command := confirm.handleEnter()
	if command == nil {
		t.Fatal("accepting PowerShell profile append returned no command")
	}
	message, ok := command().(tourMsg)
	if !ok || message.err != nil {
		t.Fatalf("PowerShell profile append failed: %#v", message)
	}
	body, err := os.ReadFile(profile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(body), ">>> festival shell integration >>>") != 1 {
		t.Fatalf("profile did not contain exactly one appended block:\n%s", body)
	}
	step, _ := message.tour.Step(app.TourStepPath)
	if step.State != app.TourStepPending {
		t.Fatalf("tour PATH readback before new shell = %q, want pending", step.State)
	}
	load := exec.Command(ps, "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", "[Console]::Out.Write($env:PATH)")
	loadedPath, err := load.CombinedOutput()
	if err != nil {
		t.Fatalf("new PowerShell could not load appended profile: %v\n%s", err, loadedPath)
	}
	first, _, _ := strings.Cut(strings.TrimSpace(string(loadedPath)), ";")
	if !strings.EqualFold(filepath.Clean(first), filepath.Clean(binDir)) {
		t.Fatalf("new PowerShell did not load managed bin %q from profile: %q", binDir, loadedPath)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+basePath)
	afterRestart, err := app.LoadTour(ctx)
	if err != nil {
		t.Fatal(err)
	}
	step, _ = afterRestart.Step(app.TourStepPath)
	if step.State != app.TourStepDone {
		t.Fatalf("tour PATH readback after new shell = %q, want done", step.State)
	}
}
