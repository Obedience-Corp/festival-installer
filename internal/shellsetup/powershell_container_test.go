//go:build container_fs

package shellsetup

import (
	"context"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf16"
)

// This test writes actual PowerShell profiles. Run only in a disposable
// container: on Windows the profile follows the user's Documents known folder.
func TestPowerShellInteractiveProfileOnly(t *testing.T) {
	shells := []string{"pwsh"}
	if runtime.GOOS == "windows" {
		shells = []string{"powershell"}
	}
	for _, shell := range shells {
		t.Run(shell, func(t *testing.T) {
			path, err := exec.LookPath(shell)
			if err != nil {
				t.Fatal(err)
			}
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
			ctx := context.Background()
			profile, err := Run(ctx, path, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", "[Console]::Write($PROFILE.CurrentUserAllHosts)")
			if err != nil {
				t.Fatal(err)
			}
			profile = strings.TrimSpace(profile)
			if !filepath.IsAbs(profile) {
				t.Fatalf("unexpected profile: %q", profile)
			}
			toolDir := filepath.Join(home, "tool parent")
			appDir := filepath.Join(home, "app exact tools")
			for _, dir := range []string{toolDir, appDir} {
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("FESTIVAL_APP_TOOL_BIN", "")
			t.Setenv("FESTIVAL_INTEGRATION_PROBE", "")
			helper := filepath.Join(home, "helper.ps1")
			if err := os.WriteFile(helper, []byte(Header(shell, []string{filepath.Join(toolDir, "camp")})+"$env:FESTIVAL_TEST_HELPER_LOADED = 'yes'\nfunction cgo { }\nfunction fgo { }\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(profile), 0700); err != nil {
				t.Fatal(err)
			}
			// Save/restore in case another proof in this container uses the same user.
			old, readErr := os.ReadFile(profile)
			t.Cleanup(func() {
				if readErr == nil {
					_ = os.WriteFile(profile, old, 0600)
				} else {
					_ = os.Remove(profile)
				}
			})
			if err := os.WriteFile(profile, []byte(Block(Source(shell, helper))), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("FESTIVAL_TEST_HELPER_LOADED", "")
			probe := ProbeHelpers(shell, []string{"cgo", "fgo"}, false) + "exit\n"
			out, err := RunProbeInput(ctx, path, probe, "-NoLogo", "-NoExit")
			if err != nil || !strings.Contains(out, "FESTIVAL_INTEGRATION_READY") {
				diagnostic, diagnosticErr := RunProbeInput(ctx, path, "@{Host=$Host.Name; UserInteractive=[Environment]::UserInteractive; Args=[Environment]::GetCommandLineArgs(); Profile=$PROFILE.CurrentUserAllHosts; Policy=(Get-ExecutionPolicy); Errors=@($Error | ForEach-Object { $_.ToString() })} | ConvertTo-Json -Compress\nexit\n", "-NoLogo", "-NoExit")
				t.Fatalf("interactive: %q %v; startup facts: %s (%v)", out, err, diagnostic, diagnosticErr)
			}
			assertion := "if ($env:FESTIVAL_TEST_HELPER_LOADED -eq 'yes') { exit 31 }; if (Get-Command cgo -CommandType Function -ErrorAction SilentlyContinue) { exit 32 }; echo FESTIVAL_SCRIPT_UNCHANGED"
			script := filepath.Join(home, "script.ps1")
			if err := os.WriteFile(script, []byte(assertion), 0600); err != nil {
				t.Fatal(err)
			}
			var encoded []byte
			for _, c := range utf16.Encode([]rune(assertion)) {
				encoded = append(encoded, byte(c), byte(c>>8))
			}
			for _, args := range [][]string{
				{"-NoLogo", "-NonInteractive", "-Command", assertion},
				{"-NoLogo", "-Command", assertion},
				{"-NoLogo", "-NoExit", "-Command", assertion + "; exit"},
				{"-NoLogo", "-c", assertion},
				{"-NoLogo", "-File", script},
				{"-NoLogo", "-f", script},
				{"-NoLogo", script},
				{"-NoLogo", "-e", base64.StdEncoding.EncodeToString(encoded)},
			} {
				out, err := Run(ctx, path, args...)
				if err != nil || !strings.Contains(out, "FESTIVAL_SCRIPT_UNCHANGED") {
					t.Fatalf("script %v: %q %v", args, out, err)
				}
			}
			out, err = RunProbe(ctx, path, "-NoLogo", "-NoExit", "-Command", assertion+"; exit")
			if err != nil || !strings.Contains(out, "FESTIVAL_SCRIPT_UNCHANGED") {
				t.Fatalf("probe marker must not enable command sessions: %q %v", out, err)
			}
			for _, args := range [][]string{{"-NoLogo"}, {"-NoLogo", "-NoExit"}} {
				out, err = RunInput(ctx, path, assertion+"\nexit\n", args...)
				if err != nil || !strings.Contains(out, "FESTIVAL_SCRIPT_UNCHANGED") {
					t.Fatalf("pipeline %v: %q %v", args, out, err)
				}
			}
			for _, tc := range []struct{ override, want string }{
				{appDir, appDir}, {"", toolDir}, {"relative/path", toolDir}, {filepath.Join(home, "absent"), toolDir},
			} {
				t.Setenv("FESTIVAL_APP_TOOL_BIN", tc.override)
				assertPath := "if (($env:PATH -split [IO.Path]::PathSeparator)[0] -ne " + Quote(shell, tc.want) + ") { exit 35 }; echo FESTIVAL_PATH_READY\nexit\n"
				out, err := RunProbeInput(ctx, path, assertPath, "-NoLogo", "-NoExit")
				if err != nil || !strings.Contains(out, "FESTIVAL_PATH_READY") {
					t.Fatalf("app override %q: %q %v", tc.override, out, err)
				}
			}
		})
	}
}
