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

	"golang.org/x/sys/windows/registry"

	"github.com/Obedience-Corp/festival-installer/internal/app"
)

func TestWindowsTourPathConsentUserPathAndReadback(t *testing.T) {
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
	profileBefore, profileErr := os.ReadFile(profile)
	if profileErr != nil && !errors.Is(profileErr, os.ErrNotExist) {
		t.Fatal(profileErr)
	}
	assertProfileUnchanged := func(stage string) {
		t.Helper()
		body, err := os.ReadFile(profile)
		if errors.Is(profileErr, os.ErrNotExist) {
			if !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("%s created a PowerShell profile: %v", stage, err)
			}
		} else if err != nil || string(body) != string(profileBefore) {
			t.Fatalf("%s changed PowerShell profile: %v", stage, err)
		}
	}

	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = key.Close() })
	before, kind, readErr := key.GetStringValue("Path")
	if readErr != nil && !errors.Is(readErr, registry.ErrNotExist) {
		t.Fatal(readErr)
	}
	t.Cleanup(func() {
		if errors.Is(readErr, registry.ErrNotExist) {
			_ = key.DeleteValue("Path")
		} else if kind == registry.EXPAND_SZ {
			_ = key.SetExpandStringValue("Path", before)
		} else {
			_ = key.SetStringValue("Path", before)
		}
	})
	if strings.Contains(strings.ToLower(before), strings.ToLower(binDir)) {
		t.Fatalf("test user Path already contains managed bin: %q", before)
	}

	m := tourAt(t, app.TourStepPath)
	next, _ := m.handleEnter()
	confirm := next.(model)
	if confirm.screen != screenConfirm || confirm.confirmArg != "powershell" || !strings.Contains(confirm.confirmMsg, `HKEY_CURRENT_USER\Environment\Path`) {
		t.Fatalf("tour confirmation did not identify the user Path: %+v", confirm)
	}
	if !strings.Contains(confirm.confirmMsg, binDir) {
		t.Fatalf("tour confirmation omitted managed bin %q: %q", binDir, confirm.confirmMsg)
	}
	assertProfileUnchanged("showing confirmation")
	confirm.confirmYes = false
	_, noCmd := confirm.handleEnter()
	if noCmd != nil {
		t.Fatal("declining user Path change returned a write command")
	}
	if got, _, err := key.GetStringValue("Path"); err != readErr || got != before {
		t.Fatalf("declining changed user Path: %q, %v", got, err)
	}

	next, _ = m.handleEnter()
	confirm = next.(model)
	confirm.confirmYes = true
	_, command := confirm.handleEnter()
	if command == nil {
		t.Fatal("accepting user Path change returned no command")
	}
	message, ok := command().(tourMsg)
	if !ok || message.err != nil {
		t.Fatalf("user Path change failed: %#v", message)
	}
	value, valueKind, err := key.GetStringValue("Path")
	if err != nil {
		t.Fatal(err)
	}
	if valueKind != kind && !errors.Is(readErr, registry.ErrNotExist) {
		t.Fatalf("user Path registry type changed from %d to %d", kind, valueKind)
	}
	first, rest, _ := strings.Cut(value, ";")
	if !strings.EqualFold(first, binDir) || rest != before {
		t.Fatalf("user Path = %q, want managed bin before prior value %q", value, before)
	}
	assertProfileUnchanged("accepting")
	step, _ := message.tour.Step(app.TourStepPath)
	if step.State != app.TourStepPending {
		t.Fatalf("tour PATH readback before new session = %q, want pending", step.State)
	}
	plan, err := app.PlanShellRCAppend(ctx, "powershell")
	if err != nil || !plan.Present {
		t.Fatalf("second plan must see the persisted Path: %+v, %v", plan, err)
	}
	if err := app.ApplyShellRCAppend(ctx, plan); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if again, _, err := key.GetStringValue("Path"); err != nil || again != value {
		t.Fatalf("second apply duplicated user Path: %q, %v", again, err)
	}
	ps := `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`
	load := exec.Command(ps, "-NonInteractive", "-Command", "[Console]::Out.Write([Environment]::GetEnvironmentVariable('Path','User'))")
	loadedPath, err := load.CombinedOutput()
	if err != nil {
		t.Fatalf("fresh PowerShell could not read user Path: %v\n%s", err, loadedPath)
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(string(loadedPath))), strings.ToLower(binDir)) {
		t.Fatalf("fresh PowerShell user Path omitted managed bin: %q", loadedPath)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+basePath)
	afterRestart, err := app.LoadTour(ctx)
	if err != nil {
		t.Fatal(err)
	}
	step, _ = afterRestart.Step(app.TourStepPath)
	if step.State != app.TourStepDone {
		t.Fatalf("tour PATH readback after new session = %q, want done", step.State)
	}
}
