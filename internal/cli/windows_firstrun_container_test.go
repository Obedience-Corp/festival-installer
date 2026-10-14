//go:build windows && container_fs

package cli_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/cli"
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
	basePath := os.Getenv("PATH")
	t.Setenv("PATH", filepath.Dir(os.Args[0])+string(os.PathListSeparator)+basePath)

	var out, errOut bytes.Buffer
	if err := cli.BareInvocation(ctx, &out, &errOut, func() error {
		t.Fatal("first run unexpectedly called help")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := cli.ShellFromEnv(); got != "powershell" {
		t.Fatalf("Windows default shell = %q, want powershell", got)
	}
	var line string
	for _, text := range strings.Split(out.String(), "\n") {
		if _, value, ok := strings.Cut(text, "Wire your PATH:"); ok {
			line = strings.TrimSpace(value)
			break
		}
	}
	if line != "festival shell-init powershell | Invoke-Expression" {
		t.Fatalf("first-run PATH command = %q:\n%s", line, out.String())
	}
	binDir, err := state.BinDir(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ps := `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`
	cmd := exec.Command(ps, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", line+"; [Console]::Out.Write($env:PATH)")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pasting first-run line into Windows PowerShell failed: %v\n%s", err, output)
	}
	first, _, _ := strings.Cut(strings.TrimSpace(string(output)), ";")
	if !strings.EqualFold(filepath.Clean(first), filepath.Clean(binDir)) {
		t.Fatalf("first-run command did not prepend managed bin %q: %q", binDir, output)
	}
}
