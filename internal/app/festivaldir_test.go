package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

// fakeFest puts a fest on PATH that answers list with body and answers next
// according to whether the directory it was run in is marked runnable. It logs
// every invocation so a test can count how many candidates were probed.
//
// The script uses only shell builtins. PATH here holds nothing but this fake,
// so a script calling cat or printf would find neither and print nothing at
// all, which would read as an empty listing rather than a broken fixture.
func fakeFest(t *testing.T, body string) (logFile string) {
	t.Helper()
	dir := t.TempDir()
	logFile = filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\n" +
		"echo \"$1 $2 [$(pwd)]\" >> " + logFile + "\n" +
		"case \"$1\" in\n" +
		"  list) echo '" + body + "' ;;\n" +
		"  next) if [ -f .runnable ]; then exit 0; fi; exit 1 ;;\n" +
		"esac\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "fest"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake fest: %v", err)
	}
	t.Setenv("PATH", dir)
	return logFile
}

// festEnv points the installer home at an empty dir so ResolveTool finds the
// fake on PATH rather than a managed binary or the developer's own install.
func festEnv(t *testing.T) {
	t.Helper()
	t.Setenv("FESTIVAL_HOME", t.TempDir())
	t.Setenv("OBEY_INSTALLER_HOME", "")
	t.Setenv("PATH", t.TempDir())
}

// festivalDir makes a real directory to stand in for a festival. The paths in
// fest's listing must exist, because probing one means running fest with that
// directory as its working directory.
func festivalDir(t *testing.T, camp, name string, runnable bool) string {
	t.Helper()
	dir := filepath.Join(camp, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if runnable {
		if err := os.WriteFile(filepath.Join(dir, ".runnable"), nil, 0o644); err != nil {
			t.Fatalf("mark runnable: %v", err)
		}
	}
	return dir
}

func entry(path string) string {
	return `{"name":"` + filepath.Base(path) + `","path":"` + path + `","status":"x"}`
}

func probeCount(t *testing.T, logFile string) int {
	t.Helper()
	raw, err := os.ReadFile(logFile)
	if err != nil {
		return 0
	}
	n := 0
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "next ") {
			n++
		}
	}
	return n
}

func TestResolveRunTarget_PrefersActiveThenReadyThenPlanning(t *testing.T) {
	tests := []struct {
		name    string
		buckets []string
		want    string
	}{
		{"active wins", []string{"active", "ready", "planning"}, "active"},
		{"ready when nothing is active", []string{"ready", "planning"}, "ready"},
		{"planning is the last resort", []string{"planning"}, "planning"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			festEnv(t)
			camp := t.TempDir()
			parts := make([]string, 0, len(tc.buckets))
			var want string
			for _, b := range tc.buckets {
				dir := festivalDir(t, camp, b, true)
				parts = append(parts, `"`+b+`":[`+entry(dir)+`]`)
				if b == tc.want {
					want = dir
				}
			}
			fakeFest(t, "{"+strings.Join(parts, ",")+`,"total":3}`)

			got, err := ResolveRunTarget(context.Background(), camp)
			if err != nil {
				t.Fatalf("ResolveRunTarget: %v", err)
			}
			if got.Dir != want {
				t.Fatalf("dir = %q, want %q", got.Dir, want)
			}
			if got.Kind != TargetFestival || !got.Ready {
				t.Fatalf("kind/ready = %v/%v, want festival/true", got.Kind, got.Ready)
			}
		})
	}
}

// TestResolveRunTarget_SkipsAFestivalFestWillNotRun is the reason the probe
// exists. A freshly scaffolded festival fails its own validation until an agent
// resolves its markers, and picking it would leave the step permanently stuck.
func TestResolveRunTarget_SkipsAFestivalFestWillNotRun(t *testing.T) {
	festEnv(t)
	camp := t.TempDir()
	blocked := festivalDir(t, camp, "blocked", false)
	good := festivalDir(t, camp, "good", true)
	fakeFest(t, `{"active":[`+entry(blocked)+","+entry(good)+`],"total":2}`)

	got, err := ResolveRunTarget(context.Background(), camp)
	if err != nil {
		t.Fatalf("ResolveRunTarget: %v", err)
	}
	if got.Dir != good {
		t.Fatalf("dir = %q, want the runnable festival %q", got.Dir, good)
	}
}

// TestResolveRunTarget_StopsProbingAtTheFirstAcceptance keeps one keypress from
// costing a subprocess per festival in a camp with a backlog.
func TestResolveRunTarget_StopsProbingAtTheFirstAcceptance(t *testing.T) {
	festEnv(t)
	camp := t.TempDir()
	first := festivalDir(t, camp, "first", true)
	second := festivalDir(t, camp, "second", true)
	logFile := fakeFest(t, `{"active":[`+entry(first)+","+entry(second)+`],"total":2}`)

	if _, err := ResolveRunTarget(context.Background(), camp); err != nil {
		t.Fatalf("ResolveRunTarget: %v", err)
	}
	if n := probeCount(t, logFile); n != 1 {
		t.Fatalf("probed %d candidates, want 1", n)
	}
}

// TestResolveRunTarget_GivingUpIsNotAnEmptyCamp is the guard against scaffolding
// a getting started workflow into a camp that already holds real work. Running
// out of probes means the answer is unknown, not that there is nothing here.
func TestResolveRunTarget_GivingUpIsNotAnEmptyCamp(t *testing.T) {
	festEnv(t)
	camp := t.TempDir()
	entries := make([]string, 0, maxRunTargetProbes+2)
	for i := 0; i < maxRunTargetProbes+2; i++ {
		entries = append(entries, entry(festivalDir(t, camp, "f"+string(rune('a'+i)), false)))
	}
	logFile := fakeFest(t, `{"active":[`+strings.Join(entries, ",")+`],"total":7}`)

	_, err := ResolveRunTarget(context.Background(), camp)
	if got := errpkg.Code(err); got != CodeRunTargetUnchecked {
		t.Fatalf("code = %q, want %q (err=%v)", got, CodeRunTargetUnchecked, err)
	}
	if n := probeCount(t, logFile); n != maxRunTargetProbes {
		t.Fatalf("probed %d candidates, want the cap of %d", n, maxRunTargetProbes)
	}
}

// TestResolveRunTarget_AllCandidatesRuledOutFallsThrough is the other half: when
// every candidate really was checked and none runs, scaffolding is correct.
func TestResolveRunTarget_AllCandidatesRuledOutFallsThrough(t *testing.T) {
	festEnv(t)
	camp := t.TempDir()
	blocked := festivalDir(t, camp, "blocked", false)
	fakeFest(t, `{"planning":[`+entry(blocked)+`],"total":1}`)

	_, err := ResolveRunTarget(context.Background(), camp)
	if got := errpkg.Code(err); got != CodeNoFestival {
		t.Fatalf("code = %q, want %q (err=%v)", got, CodeNoFestival, err)
	}
}

func TestResolveRunTarget_FindsAStandaloneWorkflow(t *testing.T) {
	festEnv(t)
	camp := t.TempDir()
	wf := workflowDir(t, camp, filepath.Join("workflow", "getting-started"), true)
	fakeFest(t, `{"total":0}`)

	got, err := ResolveRunTarget(context.Background(), camp)
	if err != nil {
		t.Fatalf("ResolveRunTarget: %v", err)
	}
	if got.Dir != wf || got.Kind != TargetWorkflow || !got.Ready {
		t.Fatalf("got %+v, want a ready workflow at %q", got, wf)
	}
}

// TestResolveRunTarget_AFinishedWorkflowNeedsANewRun covers the repeat visit. A
// workflow whose last run completed is still the right place to go; fest next
// refuses there until a run is started, so the target comes back not ready.
func TestResolveRunTarget_AFinishedWorkflowNeedsANewRun(t *testing.T) {
	festEnv(t)
	camp := t.TempDir()
	wf := workflowDir(t, camp, filepath.Join("workflow", "getting-started"), false)
	fakeFest(t, `{"total":0}`)

	got, err := ResolveRunTarget(context.Background(), camp)
	if err != nil {
		t.Fatalf("ResolveRunTarget: %v", err)
	}
	if got.Dir != wf || got.Kind != TargetWorkflow {
		t.Fatalf("got %+v, want the workflow at %q", got, wf)
	}
	if got.Ready {
		t.Fatal("a workflow with no active run must not be reported ready")
	}
}

// TestResolveRunTarget_ADocumentAloneIsNotAWorkflow keeps a WORKFLOW.md copied
// into a repo for reference from being mistaken for a run to continue.
func TestResolveRunTarget_ADocumentAloneIsNotAWorkflow(t *testing.T) {
	festEnv(t)
	camp := t.TempDir()
	dir := filepath.Join(camp, "docs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "WORKFLOW.md"), []byte("# copied"), 0o644); err != nil {
		t.Fatalf("write doc: %v", err)
	}
	fakeFest(t, `{"total":0}`)

	_, err := ResolveRunTarget(context.Background(), camp)
	if got := errpkg.Code(err); got != CodeNoFestival {
		t.Fatalf("code = %q, want %q (err=%v)", got, CodeNoFestival, err)
	}
}

func TestResolveRunTarget_IgnoresParkedAndRitual(t *testing.T) {
	festEnv(t)
	camp := t.TempDir()
	parked := festivalDir(t, camp, "parked", true)
	ritual := festivalDir(t, camp, "ritual", true)
	fakeFest(t, `{"parked":[`+entry(parked)+`],"ritual":[`+entry(ritual)+`],"total":2}`)

	_, err := ResolveRunTarget(context.Background(), camp)
	if got := errpkg.Code(err); got != CodeNoFestival {
		t.Fatalf("code = %q, want %q (err=%v)", got, CodeNoFestival, err)
	}
}

// TestResolveRunTarget_TotalDoesNotBreakTheParse is the regression test for
// decoding fest's listing as a map of arrays. fest puts an integer total beside
// the status buckets, which makes that parse fail on every real camp.
func TestResolveRunTarget_TotalDoesNotBreakTheParse(t *testing.T) {
	festEnv(t)
	camp := t.TempDir()
	good := festivalDir(t, camp, "good", true)
	fakeFest(t, `{"active":[`+entry(good)+`],"residents":{"active":[{"name":"someone"}]},"total":13}`)

	got, err := ResolveRunTarget(context.Background(), camp)
	if err != nil {
		t.Fatalf("ResolveRunTarget: %v", err)
	}
	if got.Dir != good {
		t.Fatalf("dir = %q, want %q", got.Dir, good)
	}
}

func TestResolveRunTarget_EmptyCampHasNothingToRun(t *testing.T) {
	bodies := []struct{ name, body string }{
		{"an empty document", `{"total":0}`},
		{"no output at all", ``},
		{"a json null", `null`},
		{"buckets present but empty", `{"active":[],"ready":[],"planning":[],"total":0}`},
	}
	for _, tc := range bodies {
		t.Run(tc.name, func(t *testing.T) {
			festEnv(t)
			fakeFest(t, tc.body)
			_, err := ResolveRunTarget(context.Background(), t.TempDir())
			if got := errpkg.Code(err); got != CodeNoFestival {
				t.Fatalf("code = %q, want %q (err=%v)", got, CodeNoFestival, err)
			}
		})
	}
}

func TestResolveRunTarget_AsksInsideTheCamp(t *testing.T) {
	festEnv(t)
	camp := t.TempDir()
	good := festivalDir(t, camp, "good", true)
	logFile := fakeFest(t, `{"active":[`+entry(good)+`],"total":1}`)

	if _, err := ResolveRunTarget(context.Background(), camp); err != nil {
		t.Fatalf("ResolveRunTarget: %v", err)
	}
	raw, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read call log: %v", err)
	}
	// The fake reports the logical working directory, which is the path the
	// test handed over, not its symlink-resolved form.
	if !strings.Contains(string(raw), "list --json ["+camp+"]") {
		t.Fatalf("the listing must run in the camp root %q, got:\n%s", camp, raw)
	}
	if !strings.Contains(string(raw), "next --json ["+good+"]") {
		t.Fatalf("the probe must run in the festival %q, got:\n%s", good, raw)
	}
}

func TestResolveRunTarget_NoCampRootIsItsOwnAnswer(t *testing.T) {
	festEnv(t)
	fakeFest(t, `{"total":0}`)

	_, err := ResolveRunTarget(context.Background(), "  ")
	if got := errpkg.Code(err); got != CodeNoCampRoot {
		t.Fatalf("code = %q, want %q (err=%v)", got, CodeNoCampRoot, err)
	}
}

func TestResolveRunTarget_UnreadableListingIsNotAnEmptyCamp(t *testing.T) {
	festEnv(t)
	fakeFest(t, `this is not json`)

	_, err := ResolveRunTarget(context.Background(), t.TempDir())
	if got := errpkg.Code(err); got != CodeFestivalList {
		t.Fatalf("code = %q, want %q (err=%v)", got, CodeFestivalList, err)
	}
}

func TestResolveRunTarget_MissingFestIsANotFound(t *testing.T) {
	festEnv(t)

	_, err := ResolveRunTarget(context.Background(), t.TempDir())
	if got := errpkg.Code(err); got != "E_LAUNCH_NOT_FOUND" {
		t.Fatalf("code = %q, want E_LAUNCH_NOT_FOUND (err=%v)", got, err)
	}
}

func TestResolveRunTarget_CancelledContext(t *testing.T) {
	festEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := ResolveRunTarget(ctx, t.TempDir()); err == nil {
		t.Fatal("a cancelled context must not run fest")
	}
}

// workflowDir builds a standalone workflow: the document plus the runtime dir
// fest writes beside it. runnable marks it as having an active run.
func workflowDir(t *testing.T, camp, rel string, runnable bool) string {
	t.Helper()
	dir := filepath.Join(camp, rel)
	if err := os.MkdirAll(filepath.Join(dir, ".workflow"), 0o755); err != nil {
		t.Fatalf("mkdir workflow: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "WORKFLOW.md"), []byte("# wf"), 0o644); err != nil {
		t.Fatalf("write WORKFLOW.md: %v", err)
	}
	if runnable {
		if err := os.WriteFile(filepath.Join(dir, ".runnable"), nil, 0o644); err != nil {
			t.Fatalf("mark runnable: %v", err)
		}
	}
	return dir
}

// festWorkspaceRefusal is what fest prints and exits with in a camp that has no
// festivals directory, which is what `camp init` leaves behind when fest was
// not on PATH. Captured from fest v0.6.6.
const festWorkspaceRefusal = "Error: not in a fest workspace\n\n  Could not find a festivals/ directory.\n"

// refusingFest puts a fest on PATH that writes fest's real workspace refusal to
// stderr and exits 1 for every subcommand.
func refusingFest(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n>&2 echo '" + festWorkspaceRefusal + "'\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "fest"), []byte(script), 0o755); err != nil {
		t.Fatalf("write refusing fest: %v", err)
	}
	t.Setenv("PATH", dir)
}

// TestResolveRunTarget_CampWithoutAFestivalsDirectory is the regression test for
// a raw child exit status reaching the tour screen. A camp with no fest
// workspace is not a failure to report, it is the answer "no festivals", and the
// step goes on to scaffold a workflow.
func TestResolveRunTarget_CampWithoutAFestivalsDirectory(t *testing.T) {
	festEnv(t)
	refusingFest(t)

	_, err := ResolveRunTarget(context.Background(), t.TempDir())
	if got := errpkg.Code(err); got != CodeNoFestival {
		t.Fatalf("code = %q, want %q so the scaffold branch runs (err=%v)", got, CodeNoFestival, err)
	}
}

// TestResolveRunTarget_TheThreeCampShapes pins the distinction the tour depends
// on: a camp fest refuses to list, a fest that is not installed, and a working
// fest with an empty camp.
func TestResolveRunTarget_TheThreeCampShapes(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T)
		wantCode string
	}{
		{
			name:     "no festivals directory: fest runs and refuses",
			setup:    func(t *testing.T) { refusingFest(t) },
			wantCode: CodeNoFestival,
		},
		{
			name:     "fest is not installed at all",
			setup:    func(t *testing.T) {},
			wantCode: "E_LAUNCH_NOT_FOUND",
		},
		{
			name:     "fest works and the camp is empty",
			setup:    func(t *testing.T) { fakeFest(t, `{"total":0}`) },
			wantCode: CodeNoFestival,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			festEnv(t)
			tc.setup(t)
			_, err := ResolveRunTarget(context.Background(), t.TempDir())
			if got := errpkg.Code(err); got != tc.wantCode {
				t.Fatalf("code = %q, want %q (err=%v)", got, tc.wantCode, err)
			}
		})
	}
}

// TestRunTargetProblem_KeepsDiagnosticsOffTheScreen is the guard on what a user
// reads. FriendlyMessage is what every terminal surface renders, so an error
// code or a child's exit status must not survive it.
func TestRunTargetProblem_KeepsDiagnosticsOffTheScreen(t *testing.T) {
	festEnv(t)
	fakeFest(t, `this is not json`)

	_, err := ResolveRunTarget(context.Background(), t.TempDir())
	if got := errpkg.Code(err); got != CodeFestivalList {
		t.Fatalf("code = %q, want %q", got, CodeFestivalList)
	}
	friendly := FriendlyMessage(err)
	for _, leak := range []string{"E_FESTIVAL_", "exit status", "E_LAUNCH_"} {
		if strings.Contains(friendly, leak) {
			t.Fatalf("friendly line leaks %q: %q", leak, friendly)
		}
	}
	if !strings.Contains(friendly, "fest list") {
		t.Fatalf("the friendly line must name the command that shows the reason, got %q", friendly)
	}
	// The detail is still available to a log, just not to the screen.
	if !strings.Contains(err.Error(), "E_FESTIVAL_LIST") {
		t.Fatalf("Error() must keep the code for diagnosis, got %q", err.Error())
	}
}

// TestUncheckedProblem_KeepsDiagnosticsOffTheScreen does the same for the probe
// bound, which also renders on the tour screen.
func TestUncheckedProblem_KeepsDiagnosticsOffTheScreen(t *testing.T) {
	festEnv(t)
	camp := t.TempDir()
	entries := make([]string, 0, maxRunTargetProbes+2)
	for i := 0; i < maxRunTargetProbes+2; i++ {
		entries = append(entries, entry(festivalDir(t, camp, "f"+string(rune('a'+i)), false)))
	}
	fakeFest(t, `{"active":[`+strings.Join(entries, ",")+`],"total":7}`)

	_, err := ResolveRunTarget(context.Background(), camp)
	friendly := FriendlyMessage(err)
	for _, leak := range []string{"E_FESTIVAL_", "exit status"} {
		if strings.Contains(friendly, leak) {
			t.Fatalf("friendly line leaks %q: %q", leak, friendly)
		}
	}
}
