//go:build windows && container_fs

package cli_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/state"
)

// The printed first-run line must be executable in Windows PowerShell, not
// merely contain the right shell name. The proof container carries the real
// festival.exe alongside this test binary.
func TestWindowsFirstRunPowerShellCommand(t *testing.T) {
	ctx := context.Background()
	home := filepath.Join(t.TempDir(), "installer home")
	t.Setenv("FESTIVAL_HOME", home)
	t.Setenv("OBEY_INSTALLER_HOME", "")
	t.Setenv("SHELL", "")
	binary := filepath.Join(filepath.Dir(os.Args[0]), "festival.exe")
	bare := exec.Command(binary)
	out, err := bare.CombinedOutput()
	if err != nil {
		t.Fatalf("bare festival first run failed: %v\n%s", err, out)
	}
	var line string
	for _, text := range strings.Split(string(out), "\n") {
		if _, value, ok := strings.Cut(text, "Wire your PATH:"); ok {
			line = strings.TrimSpace(value)
			break
		}
	}
	if line != "festival shell-init powershell | Invoke-Expression" {
		t.Fatalf("first-run PATH command = %q:\n%s", line, out)
	}
	binDir, err := state.BinDir(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ps := `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`
	// A user can invoke an unpacked installer by path before it is on PATH.
	// Define the command for this PowerShell session without making that
	// unpacked copy look like a managed or package installation to the child.
	quotedBinary := strings.ReplaceAll(binary, "'", "''")
	script := "function festival { & '" + quotedBinary + "' @args }; " + line + "; [Console]::Out.Write($env:PATH)"
	cmd := exec.Command(ps, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pasting first-run line into Windows PowerShell failed: %v\n%s", err, output)
	}
	first, _, _ := strings.Cut(strings.TrimSpace(string(output)), ";")
	if !strings.EqualFold(filepath.Clean(first), filepath.Clean(binDir)) {
		t.Fatalf("first-run command did not prepend managed bin %q: %q", binDir, output)
	}
}
