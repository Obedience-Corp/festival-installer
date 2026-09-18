package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestProbeToolVersion_ReadsRealOutputAndRejectsGarbage(t *testing.T) {
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
			name:   "camp answers version --short with a git describe stamp",
			tool:   "camp",
			script: "#!/bin/sh\necho v0.10.1-4-gd1fb37c7\n",
			want:   "0.10.1-4-gd1fb37c7",
		},
		{
			name:   "camp falls back to the prefixed version block",
			tool:   "camp",
			script: "#!/bin/sh\nif [ \"$2\" = --short ]; then exit 1; fi\necho 'camp v0.10.1-4-gd1fb37c7'\necho 'commit: d1fb37c7'\necho 'profile: dev'\n",
			want:   "0.10.1-4-gd1fb37c7",
		},
		{
			name:   "fest answers version --short",
			tool:   "fest",
			script: "#!/bin/sh\necho v0.8.0-4-gdfb49547\n",
			want:   "0.8.0-4-gdfb49547",
		},
		{
			name:   "festival answers the release stamp",
			tool:   selfBinaryName,
			script: "#!/bin/sh\necho v0.2.2\n",
			want:   "0.2.2",
		},
		{
			name:   "a released festival rejects --short and answers plain version",
			tool:   selfBinaryName,
			script: "#!/bin/sh\nif [ \"$2\" = --short ]; then echo 'unknown flag: --short' >&2; exit 1; fi\necho 0.0.0-dev\n",
			want:   "0.0.0-dev",
		},
		{
			name:   "obey falls back to the root flag",
			tool:   obeyBinary,
			script: "#!/bin/sh\ncase \"$1\" in\n  --version) echo \"obey version 0.1.0\" ;;\n  *) exit 1 ;;\nesac\n",
			want:   "0.1.0",
		},
		{
			name:   "obey prefers the subcommand when it answers",
			tool:   obeyBinary,
			script: "#!/bin/sh\ncase \"$1\" in\n  version) echo 0.2.0 ;;\n  *) echo \"obey version 0.1.0\" ;;\nesac\n",
			want:   "0.2.0",
		},
		{
			name:   "ob answers the cobra version template",
			tool:   obeyDevCLIBinary,
			script: "#!/bin/sh\necho \"ob version 0.1.0\"\n",
			want:   "0.1.0",
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

func TestStatusReport_ToolOriginComesFromItsOwnPath(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	t.Setenv("HOMEBREW_PREFIX", "")

	pkgBin, _ := writePackagePrefix(t, t.TempDir())
	writeProbeScript(t, pkgBin, "camp", "#!/bin/sh\necho 'camp v0.10.1'\n")

	leftoverBin := t.TempDir()
	writeProbeScript(t, leftoverBin, obeyBinary, "#!/bin/sh\necho \"obey version 0.1.0\"\n")

	t.Setenv("PATH", pkgBin+string(os.PathListSeparator)+leftoverBin)

	suite, err := DetectSuite(ctx)
	if err != nil {
		t.Fatalf("DetectSuite: %v", err)
	}
	if suite.Kind != OriginPackage {
		t.Fatalf("fixture suite origin = %q, want package", suite.Kind)
	}

	rep, err := StatusReportFor(ctx)
	if err != nil {
		t.Fatalf("StatusReportFor: %v", err)
	}
	if rep.Origin != OriginPackage {
		t.Fatalf("report Origin = %q, want the suite origin package", rep.Origin)
	}

	entries := map[string]StatusToolEntry{}
	for _, e := range rep.Tools {
		entries[e.Tool] = e
	}

	camp := entries["camp"]
	if camp.Path != filepath.Join(pkgBin, "camp") {
		t.Fatalf("camp Path = %q, want %q", camp.Path, filepath.Join(pkgBin, "camp"))
	}
	if camp.Origin != OriginPackage {
		t.Fatalf("camp Origin = %q from %q, want package", camp.Origin, camp.Path)
	}

	obey := entries[obeyBinary]
	if obey.Path != filepath.Join(leftoverBin, obeyBinary) {
		t.Fatalf("obey Path = %q, want %q", obey.Path, filepath.Join(leftoverBin, obeyBinary))
	}
	if obey.Origin != OriginLeftover {
		t.Fatalf("obey Origin = %q from %q, want leftover: a package-manager suite must not brand an unrelated binary", obey.Origin, obey.Path)
	}
}

func TestStatusReport_LeftoverPathBeatsStaleManaged(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	t.Setenv("HOMEBREW_PREFIX", "")

	managed := filepath.Join(home, "bin")
	if err := os.MkdirAll(managed, 0o700); err != nil {
		t.Fatal(err)
	}
	writeProbeScript(t, managed, "camp", "#!/bin/sh\necho 0.2.12\n")
	writeProbeScript(t, managed, "fest", "#!/bin/sh\necho 0.2.12\n")

	live := t.TempDir()
	writeProbeScript(t, live, "camp", "#!/bin/sh\necho v0.10.1-4-gd1fb37c7\n")
	writeProbeScript(t, live, "fest", "#!/bin/sh\necho v0.8.1-2-g5a34a64a\n")
	t.Setenv("PATH", live)

	rep, err := StatusReportFor(ctx)
	if err != nil {
		t.Fatalf("StatusReportFor: %v", err)
	}
	entries := map[string]StatusToolEntry{}
	for _, e := range rep.Tools {
		entries[e.Tool] = e
	}
	camp := entries["camp"]
	if camp.Path != filepath.Join(live, "camp") {
		t.Fatalf("camp Path = %q, want the leftover PATH copy %q, not the stale managed 0.2.12", camp.Path, filepath.Join(live, "camp"))
	}
	if camp.Version != "0.10.1-4-gd1fb37c7" {
		t.Fatalf("camp Version = %q, want the PATH copy", camp.Version)
	}
	fest := entries["fest"]
	if fest.Path != filepath.Join(live, "fest") {
		t.Fatalf("fest Path = %q, want the leftover PATH copy", fest.Path)
	}
	if fest.Version != "0.8.1-2-g5a34a64a" {
		t.Fatalf("fest Version = %q, want the PATH copy", fest.Version)
	}
}

// TestStatusReport_FreshManagedBeatsStaleLeftoverPATH is the other half of the
// leftover rule: the hub just installed camp and fest into its own bin and the
// shell has not picked that directory up yet, so PATH still answers with the
// copy the install replaced. Preferring PATH unconditionally would report the
// version the user just upgraded away from.
func TestStatusReport_FreshManagedBeatsStaleLeftoverPATH(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	t.Setenv("HOMEBREW_PREFIX", "")

	managed := filepath.Join(home, "bin")
	if err := os.MkdirAll(managed, 0o700); err != nil {
		t.Fatal(err)
	}
	writeProbeScript(t, managed, "camp", "#!/bin/sh\necho v0.10.1-4-gd1fb37c7\n")
	writeProbeScript(t, managed, "fest", "#!/bin/sh\necho v0.8.1-2-g5a34a64a\n")

	stale := t.TempDir()
	writeProbeScript(t, stale, "camp", "#!/bin/sh\necho 0.2.12\n")
	writeProbeScript(t, stale, "fest", "#!/bin/sh\necho 0.2.12\n")
	t.Setenv("PATH", stale)

	rep, err := StatusReportFor(ctx)
	if err != nil {
		t.Fatalf("StatusReportFor: %v", err)
	}
	entries := map[string]StatusToolEntry{}
	for _, e := range rep.Tools {
		entries[e.Tool] = e
	}
	for tool, want := range map[string]string{"camp": "0.10.1-4-gd1fb37c7", "fest": "0.8.1-2-g5a34a64a"} {
		got := entries[tool]
		if got.Path != filepath.Join(managed, tool) {
			t.Errorf("%s Path = %q, want the just-installed managed copy %q", tool, got.Path, filepath.Join(managed, tool))
		}
		if got.Version != want {
			t.Errorf("%s Version = %q, want %q", tool, got.Version, want)
		}
	}
}

// backgroundingProbeScript exits immediately but leaves a child holding the
// stdout it inherited, which is the shape that defeats a bare context timeout.
// The fixture needs /bin/sh to background a child, which is the whole point; it
// writes nothing outside the test's own temp dir.
func backgroundingProbeScript(t *testing.T) string {
	t.Helper()
	return writeProbeScript(t, t.TempDir(), "camp", "#!/bin/sh\nsleep 30 &\necho v0.9.9\n")
}

// probeBound is comfortably above probeTimeout plus probeWaitDelay and far
// below the 30 seconds the grandchild holds the pipe.
const probeBound = probeTimeout + 3*time.Second

// TestRunProbe_FoldsWaitDelayAndKeepsTheOutput is the deterministic half of the
// WaitDelay claim: given room to exit normally, the probe still returns the
// version the tool printed even though a grandchild holds stdout open.
func TestRunProbe_FoldsWaitDelayAndKeepsTheOutput(t *testing.T) {
	path := backgroundingProbeScript(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	start := time.Now()
	out, err := runProbe(ctx, path, "version", "--short")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("runProbe after %s: %v", elapsed, err)
	}
	if got := ParseToolVersion("camp", string(out)); got != "0.9.9" {
		t.Fatalf("version = %q after %s, want 0.9.9: a leftover grandchild must not cost the version", got, elapsed)
	}
	if elapsed > probeBound {
		t.Fatalf("runProbe took %s, want under %s: WaitDelay did not close the pipes", elapsed, probeBound)
	}
}

// TestProbes_BoundedWhenAChildHoldsStdout is the reason both probe sites set
// cmd.WaitDelay. exec kills the direct child when the context expires, but
// Output waits for stdout to reach EOF, and the backgrounded sleep holds the
// inherited pipe open long past the probe's budget. Measured without WaitDelay:
// a 2s context returned after 30s, the full lifetime of the grandchild.
//
// Only the bound is asserted here. Whether the probe comes back with the
// version or with a deadline error depends on whether the fixture shell got to
// exit inside the budget, which on a loaded machine it may not; both outcomes
// are correct and neither takes 30 seconds. The version itself is pinned by
// TestRunProbe_FoldsWaitDelayAndKeepsTheOutput, which has no deadline pressure.
func TestProbes_BoundedWhenAChildHoldsStdout(t *testing.T) {
	path := backgroundingProbeScript(t)

	t.Run("probeToolVersion", func(t *testing.T) {
		start := time.Now()
		got, err := probeToolVersion(context.Background(), "camp", path)
		elapsed := time.Since(start)
		if elapsed > probeBound {
			t.Fatalf("probeToolVersion took %s, want under %s: the timeout does not bound the read", elapsed, probeBound)
		}
		if err == nil && got != "0.9.9" {
			t.Fatalf("version = %q after %s, want 0.9.9", got, elapsed)
		}
	})

	t.Run("probeBinary", func(t *testing.T) {
		start := time.Now()
		got, _, _ := probeBinary(context.Background(), path, "camp")
		elapsed := time.Since(start)
		if elapsed > probeBound {
			t.Fatalf("probeBinary took %s, want under %s: the timeout does not bound the read", elapsed, probeBound)
		}
		if got != "" && got != "0.9.9" {
			t.Fatalf("version = %q after %s, want 0.9.9 or none", got, elapsed)
		}
	})
}

// TestProbeToolVersion_FallbackGetsItsOwnBudget is why the budget is per
// argument set. The fixture's `version --short` hangs past its deadline, the
// shape a released festival's refusal or a cold first exec produces, and plain
// `version` answers at once. Under one budget shared across the table the first
// attempt spends all of it and the fallback is dead on arrival, so the tool
// reports no version at all.
func TestProbeToolVersion_FallbackGetsItsOwnBudget(t *testing.T) {
	path := writeProbeScript(t, t.TempDir(), "festival",
		"#!/bin/sh\nif [ \"$2\" = --short ]; then sleep 30; fi\necho v0.9.9\n")

	start := time.Now()
	got, err := probeToolVersion(context.Background(), selfBinaryName, path)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("probeToolVersion after %s: %v", elapsed, err)
	}
	if got != "0.9.9" {
		t.Fatalf("version = %q after %s, want 0.9.9 from the fallback", got, elapsed)
	}
	if elapsed < probeTimeout {
		t.Fatalf("took %s, under the %s the first attempt should have spent: the fixture did not hang as intended", elapsed, probeTimeout)
	}
	if ceiling := 2*probeTimeout + 3*time.Second; elapsed > ceiling {
		t.Fatalf("took %s, want under %s: the fallback did not start promptly after the first attempt timed out", elapsed, ceiling)
	}
}

// loggingProbeScript answers every version probe in the table and appends the
// argv of each run to log, so a test can count what one call actually spawned.
func loggingProbeScript(log string) string {
	return "#!/bin/sh\n" +
		"echo \"$0 $*\" >> " + log + "\n" +
		"case \"$1 $2\" in\n" +
		"  \"version --short\") echo v0.10.1-4-gd1fb37c7; exit 0;;\n" +
		"  \"version --json\") echo '{\"version\":\"v0.10.1-4-gd1fb37c7\"}'; exit 0;;\n" +
		"  \"version \") echo \"$(basename \"$0\") v0.10.1-4-gd1fb37c7\"; exit 0;;\n" +
		"  \"--version \") echo \"$(basename \"$0\") version 0.1.0\"; exit 0;;\n" +
		"esac\n" +
		"exit 1\n"
}

func probeLogLines(t *testing.T, log string) []string {
	t.Helper()
	raw, err := os.ReadFile(log) //nolint:gosec // the path is this test's own temp dir
	if err != nil {
		t.Fatalf("read probe log: %v", err)
	}
	var out []string
	for _, line := range strings.Split(string(raw), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// TestStatusReport_ProbesEachBinaryOncePerCall pins the exec count of the path
// festival-app runs on every launch. Detection used to happen inside
// ResolveTool, so one report ran DetectSuite six times, once for the report and
// once per reported tool, and DetectSuite probes every camp, fest, and festival
// it finds on PATH. Against this fixture that was 23 execs for the 8 distinct
// answers the report reads: the three PATH copies DetectSuite classifies, and
// the five resolved binaries the report asks for a version.
func TestStatusReport_ProbesEachBinaryOncePerCall(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	t.Setenv("HOMEBREW_PREFIX", "")

	log := filepath.Join(t.TempDir(), "calls.log")
	binDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir managed bin: %v", err)
	}
	for _, tool := range statusTools {
		writeProbeScript(t, binDir, tool, loggingProbeScript(log))
	}
	pathDir := t.TempDir()
	for _, tool := range suiteTools {
		writeProbeScript(t, pathDir, tool, loggingProbeScript(log))
	}
	t.Setenv("PATH", pathDir)

	rep, err := StatusReportFor(ctx)
	if err != nil {
		t.Fatalf("StatusReportFor: %v", err)
	}
	for _, e := range rep.Tools {
		if e.Version == "" {
			t.Fatalf("%s reported no version (%s): the fixture did not answer its probe", e.Tool, e.Error)
		}
	}

	runs := map[string]int{}
	for _, line := range probeLogLines(t, log) {
		runs[line]++
	}
	for line, n := range runs {
		if n != 1 {
			t.Errorf("%q ran %d times, want 1: a repeated suite detection is back", line, n)
		}
	}
	// Every managed copy is probed once. For camp, fest, and festival that
	// one probe is the leftover comparison, and the report reuses its answer
	// rather than running the winner again.
	for _, tool := range statusTools {
		want := filepath.Join(binDir, tool) + " "
		if n := countWithPrefix(runs, want); n != 1 {
			t.Errorf("managed %s ran %d times, want 1", tool, n)
		}
	}
	// Each PATH copy is probed once, by DetectSuite, and the comparison reads
	// that version instead of spawning its own.
	for _, tool := range suiteTools {
		want := filepath.Join(pathDir, tool) + " "
		if n := countWithPrefix(runs, want); n != 1 {
			t.Errorf("PATH %s ran %d times, want 1", tool, n)
		}
	}
	if total := len(probeLogLines(t, log)); total != len(statusTools)+len(suiteTools) {
		t.Errorf("one status call spawned %d processes, want %d", total, len(statusTools)+len(suiteTools))
	}
}

func countWithPrefix(runs map[string]int, prefix string) int {
	total := 0
	for line, n := range runs {
		if strings.HasPrefix(line, prefix) {
			total += n
		}
	}
	return total
}
