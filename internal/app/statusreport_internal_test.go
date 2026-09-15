package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

func TestVersionProbes_CoverEveryReportedTool(t *testing.T) {
	for _, tool := range statusTools {
		if _, ok := versionProbes[tool]; !ok {
			t.Errorf("no versionProbes entry for reported tool %q", tool)
		}
		if _, ok := toolPackage[tool]; !ok {
			t.Errorf("no toolPackage entry for reported tool %q", tool)
		}
	}
	if len(versionProbes) != len(statusTools) {
		t.Errorf("versionProbes has %d entries for %d reported tools", len(versionProbes), len(statusTools))
	}
	if len(toolPackage) != len(statusTools) {
		t.Errorf("toolPackage has %d entries for %d reported tools", len(toolPackage), len(statusTools))
	}
	want := []string{"camp", "fest", "festival", "obey", "ob"}
	if len(statusTools) != len(want) {
		t.Fatalf("statusTools = %v, want %v", statusTools, want)
	}
	for i := range want {
		if statusTools[i] != want[i] {
			t.Fatalf("statusTools = %v, want %v", statusTools, want)
		}
	}
}

func writeProbeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil { //nolint:gosec // an executable fixture is the point
		t.Fatalf("write probe script %s: %v", path, err)
	}
	return path
}

func TestProbeToolVersion_TrimsPrefixAndRejectsGarbage(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name     string
		tool     string
		script   string
		want     string
		wantErr  bool
		contains string
	}{
		{
			name:   "bare version from the subcommand",
			tool:   "camp",
			script: "#!/bin/sh\necho 0.6.0\n",
			want:   "0.6.0",
		},
		{
			name:   "obey falls back to the root flag",
			tool:   obeyBinary,
			script: "#!/bin/sh\ncase \"$1\" in\n  --version) echo \"obey version 0.2.0\" ;;\n  *) exit 1 ;;\nesac\n",
			want:   "0.2.0",
		},
		{
			name:   "obey prefers the subcommand when it answers",
			tool:   obeyBinary,
			script: "#!/bin/sh\ncase \"$1\" in\n  version) echo 0.3.0 ;;\n  *) echo \"obey version 0.2.0\" ;;\nesac\n",
			want:   "0.3.0",
		},
		{
			name:   "ob strips its own prefix",
			tool:   obeyDevCLIBinary,
			script: "#!/bin/sh\necho \"ob version 0.2.0\"\n",
			want:   "0.2.0",
		},
		{
			name:     "a help banner is not a version",
			tool:     obeyDevCLIBinary,
			script:   "#!/bin/sh\necho 'Usage: ob [command]'\necho 'more help'\n",
			wantErr:  true,
			contains: "unparseable",
		},
		{
			name:    "a probe that exits nonzero reports the failure",
			tool:    "fest",
			script:  "#!/bin/sh\nexit 3\n",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeProbeScript(t, dir, "probe-"+strings.ReplaceAll(tc.name, " ", "-"), tc.script)
			got, err := probeToolVersion(context.Background(), tc.tool, path)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got version %q", got)
				}
				if code := errpkg.Code(err); code != "E_VERSION_PROBE" {
					t.Fatalf("code = %q, want E_VERSION_PROBE", code)
				}
				if tc.contains != "" && !strings.Contains(err.Error(), tc.contains) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.contains)
				}
				if got != "" {
					t.Fatalf("expected no version on failure, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("probeToolVersion: %v", err)
			}
			if got != tc.want {
				t.Fatalf("version = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestProbeToolVersion_UnknownToolHasNoProbe(t *testing.T) {
	_, err := probeToolVersion(context.Background(), "nosuchtool", "/bin/echo")
	if err == nil {
		t.Fatal("expected an error for a tool with no probe table entry")
	}
	if code := errpkg.Code(err); code != "E_VERSION_PROBE" {
		t.Fatalf("code = %q, want E_VERSION_PROBE", code)
	}
	if !strings.Contains(err.Error(), "no version probe defined") {
		t.Fatalf("unexpected message: %v", err)
	}
}
