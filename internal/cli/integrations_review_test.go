package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	"github.com/Obedience-Corp/festival-installer/internal/cli"
)

func TestIntegrationCommandReadinessExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix fixture")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FESTIVAL_HOME", filepath.Join(home, "installer"))
	t.Setenv("OBEY_INSTALLER_HOME", "")
	t.Setenv("PATH", "/usr/bin:/bin")
	for _, tc := range []struct {
		name    string
		args    []string
		wantErr bool
		json    bool
	}{
		{"not ready", []string{"integrations", "--shell", "/missing/nu"}, true, false},
		{"json report", []string{"integrations", "--shell", "/missing/nu", "--json"}, false, true},
		{"disable", []string{"integrations", "--shell", "/missing/nu", "--disable"}, false, false},
		{"persisted opt out", []string{"integrations", "--shell", "/missing/nu"}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if os.Geteuid() == 0 && tc.name == "disable" {
				t.Skip("root refusal is not a successful disable")
			}
			cmd := cli.NewSetupCommand()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v output=%s", err, out.String())
			}
			if tc.json {
				var envelope struct {
					Data app.IntegrationReport `json:"data"`
				}
				if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
					t.Fatal(err)
				}
				if envelope.Data.Ready || envelope.Data.Schema != app.IntegrationSchema {
					t.Fatalf("report: %+v", envelope)
				}
			}
		})
	}
}

func TestUninstallObeyProviderHookCleanup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix shell fixture")
	}
	for _, mode := range []string{"success", "failed", "both failed", "no receipt", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			f := obeyServiceFixture(t, "0.2.0")
			if mode != "no receipt" {
				if _, stderr, err := runInstaller(t, "install", "obey", "--allow-unverified", "--json"); err != nil {
					t.Fatalf("install: %v %s", err, stderr)
				}
			}
			bin := filepath.Join(f.home, "bin", "obey")
			marker := filepath.Join(t.TempDir(), "hooks")
			writeFile(t, marker, "owned hooks")
			log := filepath.Join(t.TempDir(), "cleanup.log")
			body := "#!/bin/sh\ncase \"$*\" in\n'bridge uninstall all --scope user')\n[ -x \"$0\" ] || exit 23\nprintf 'hooks\\n' >> '" + log + "'\n"
			if mode == "failed" || mode == "both failed" {
				body += "echo 'hook configuration is unwritable' >&2\nexit 1\n"
			} else {
				body += "/bin/rm '" + marker + "'\n"
			}
			body += ";;\n'service uninstall') printf 'service\\n' >> '" + log + "'\n"
			if mode == "both failed" {
				body += "echo 'service cleanup refused' >&2\nexit 1\n"
			}
			body += ";;\nesac\n"
			writeFile(t, bin, body)
			if err := os.Chmod(bin, 0755); err != nil {
				t.Fatal(err)
			}
			if mode == "cancelled" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				if _, _, err := app.UninstallTarget(ctx, "obey"); err == nil {
					t.Fatal("cancelled uninstall succeeded")
				}
				if _, err := os.Stat(bin); err != nil {
					t.Fatalf("cancelled uninstall removed binary: %v", err)
				}
				if _, err := os.Stat(log); !os.IsNotExist(err) {
					t.Fatalf("cancelled uninstall invoked cleanup: %v", err)
				}
				return
			}
			out, stderr, err := runInstaller(t, "uninstall", "obey", "--json")
			if err != nil {
				t.Fatalf("uninstall: %v %s", err, stderr)
			}
			calls, _ := os.ReadFile(log)
			if mode == "no receipt" {
				if len(calls) != 0 {
					t.Fatalf("unowned install changed: %s", calls)
				}
				if _, err := os.Stat(bin); err != nil {
					t.Fatal(err)
				}
			} else {
				if string(calls) != "hooks\nservice\n" {
					t.Fatalf("cleanup order: %q", calls)
				}
				if _, err := os.Stat(bin); !os.IsNotExist(err) {
					t.Fatalf("binary remains: %v", err)
				}
			}
			_, markerErr := os.Stat(marker)
			if (mode == "success") != os.IsNotExist(markerErr) {
				t.Fatalf("hook cleanup: %v, calls=%q, output=%s, stderr=%s", markerErr, calls, out, stderr)
			}
			if mode == "both failed" && !strings.Contains(out, "service cleanup refused") {
				t.Fatalf("lost service warning: %s", out)
			}
			if (mode == "failed" || mode == "both failed") && (!strings.Contains(out, "hook configuration is unwritable") || !strings.Contains(stderr, "obey bridge uninstall all --scope user")) {
				t.Fatalf("missing warning/recovery: %s %s", out, stderr)
			}
		})
	}
}
