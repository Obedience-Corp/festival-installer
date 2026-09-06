package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	"github.com/Obedience-Corp/festival-installer/internal/launch"
)

// tourEnv isolates the home, the shell rc file and PATH so a tour action test
// never reads or writes the developer's own machine.
func tourEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	t.Setenv("OBEY_INSTALLER_HOME", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("PATH", t.TempDir())
	return home
}

func fakeToolOnPath(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake %s: %v", name, err)
	}
	t.Setenv("PATH", dir)
	return path
}

func tourAt(t *testing.T, key app.TourStepKey) model {
	t.Helper()
	m := newModel(Options{Version: "test"})
	m.reduced = true
	m.screen = screenTour
	m.tour = app.Tour{Steps: app.TourSteps()}
	for i, st := range m.tour.Steps {
		if st.Key == key {
			m.cursor = i
			return m
		}
	}
	t.Fatalf("tour has no step %q", key)
	return m
}

func TestRunTourStep_InstallOpensTheChannelPicker(t *testing.T) {
	tourEnv(t)
	m := tourAt(t, app.TourStepInstall)
	next, _ := m.handleEnter()
	nm := next.(model)
	if nm.screen != screenInstall {
		t.Fatalf("screen = %v, want screenInstall", nm.screen)
	}
	if nm.installKind != "" {
		t.Fatalf("installKind = %q, want empty on an absent origin", nm.installKind)
	}
}

// TestRunTourStep_InstallKeepsThePackageGuard proves the tour goes through the
// same install entry point as the home menu, guards included.
func TestRunTourStep_InstallKeepsThePackageGuard(t *testing.T) {
	tourEnv(t)
	m := tourAt(t, app.TourStepInstall)
	m.status.Action = "package"
	next, _ := m.handleEnter()
	if got := next.(model).installKind; got != "package" {
		t.Fatalf("installKind = %q, want package", got)
	}
}

func TestRunTourStep_PathAsksBeforeTouchingTheRCFile(t *testing.T) {
	home := tourEnv(t)
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}

	m := tourAt(t, app.TourStepPath)
	next, _ := m.handleEnter()
	nm := next.(model)

	if nm.screen != screenConfirm {
		t.Fatalf("screen = %v, want screenConfirm", nm.screen)
	}
	if nm.confirmAct != "tour-path" {
		t.Fatalf("confirmAct = %q, want tour-path", nm.confirmAct)
	}
	if nm.confirmYes {
		t.Fatal("the confirmation must start on no")
	}
	rc, err := app.ShellRCFile("zsh")
	if err != nil {
		t.Fatalf("ShellRCFile: %v", err)
	}
	if !strings.Contains(nm.confirmMsg, rc) {
		t.Fatalf("confirm message %q must name the rc file %q", nm.confirmMsg, rc)
	}
	if _, err := os.Stat(rc); !os.IsNotExist(err) {
		t.Fatal("showing the confirmation must not write the rc file")
	}
}

func TestRunTourStep_DecliningTheRCAppendReturnsToTheTour(t *testing.T) {
	home := tourEnv(t)
	if err := os.MkdirAll(filepath.Join(home, "bin"), 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	m := tourAt(t, app.TourStepPath)
	next, _ := m.handleEnter()

	nm := next.(model)
	nm.confirmYes = false
	back, _ := nm.handleEnter()
	if got := back.(model).screen; got != screenTour {
		t.Fatalf("screen after declining = %v, want screenTour", got)
	}
	rc, _ := app.ShellRCFile("zsh")
	if _, err := os.Stat(rc); !os.IsNotExist(err) {
		t.Fatal("declining must not write the rc file")
	}
}

func TestRunTourStep_ApprovingTheRCAppendWritesItOnce(t *testing.T) {
	home := tourEnv(t)
	if err := os.MkdirAll(filepath.Join(home, "bin"), 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	m := tourAt(t, app.TourStepPath)
	next, _ := m.handleEnter()

	nm := next.(model)
	nm.confirmYes = true
	after, cmd := nm.handleEnter()
	if got := after.(model).screen; got != screenTour {
		t.Fatalf("screen after approving = %v, want screenTour", got)
	}
	if cmd == nil {
		t.Fatal("approving must return a command that does the append")
	}
	msg, ok := cmd().(tourMsg)
	if !ok {
		t.Fatalf("command returned %T, want tourMsg", cmd())
	}
	if msg.err != nil {
		t.Fatalf("append reported: %v", msg.err)
	}

	rc, _ := app.ShellRCFile("zsh")
	data, err := os.ReadFile(rc)
	if err != nil {
		t.Fatalf("read rc file: %v", err)
	}
	if !strings.Contains(string(data), "festival shell integration") {
		t.Fatalf("rc file does not carry the guarded block:\n%s", data)
	}
	step, _ := msg.tour.Step(app.TourStepPath)
	if step.State != app.TourStepPending {
		t.Fatalf("path step = %q, want pending until PATH actually carries the dir", step.State)
	}
}

func TestRunTourStep_CampInitNamesTheDirectoryFirst(t *testing.T) {
	tourEnv(t)
	fakeToolOnPath(t, "camp")

	m := tourAt(t, app.TourStepCampInit)
	next, _ := m.handleEnter()
	nm := next.(model)

	if nm.screen != screenConfirm {
		t.Fatalf("screen = %v, want screenConfirm", nm.screen)
	}
	if nm.confirmAct != "tour-camp-init" {
		t.Fatalf("confirmAct = %q, want tour-camp-init", nm.confirmAct)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if !strings.Contains(nm.confirmMsg, wd) {
		t.Fatalf("confirm message %q must name the directory %q", nm.confirmMsg, wd)
	}
	if nm.pendingLaunch != nil {
		t.Fatal("showing the confirmation must not schedule a launch")
	}
}

func TestRunTourStep_CampInitWithoutCampReportsInsteadOfLaunching(t *testing.T) {
	tourEnv(t)
	m := tourAt(t, app.TourStepCampInit)
	next, _ := m.handleEnter()
	nm := next.(model)

	if nm.err == nil {
		t.Fatal("a missing camp must surface an error, not a black screen")
	}
	if nm.pendingLaunch != nil {
		t.Fatal("a missing camp must not schedule a launch")
	}
	if nm.screen == screenConfirm {
		t.Fatal("a missing camp must not ask to run camp init")
	}
}

func TestRunTourStep_ApprovingCampInitLaunchesInTheNamedDirectory(t *testing.T) {
	tourEnv(t)
	fakeToolOnPath(t, "camp")

	m := tourAt(t, app.TourStepCampInit)
	next, _ := m.handleEnter()
	nm := next.(model)
	nm.confirmYes = true
	after, cmd := nm.handleEnter()
	am := after.(model)

	if am.pendingLaunch == nil {
		t.Fatal("approving must schedule the launch")
	}
	if cmd == nil {
		t.Fatal("approving must return the quit command that frees the terminal")
	}
	if got := am.pendingLaunch.Tool; got != "camp" {
		t.Fatalf("tool = %q, want camp", got)
	}
	if got := strings.Join(am.pendingLaunch.Args, " "); got != "init" {
		t.Fatalf("args = %q, want init", got)
	}
	wd, _ := os.Getwd()
	if am.pendingLaunch.Dir != wd {
		t.Fatalf("dir = %q, want %q", am.pendingLaunch.Dir, wd)
	}
	if am.screen != screenTour {
		t.Fatalf("screen = %v, want screenTour so the hub resumes here", am.screen)
	}
	if am.recordTourStep != "" {
		t.Fatalf("recordTourStep = %q: camp init is observed from camp, not from an exit code", am.recordTourStep)
	}
}

// fakeFestListing puts a fest on PATH that answers list with body and answers
// next according to whether the directory holds a .runnable marker. The script
// uses only shell builtins because PATH here holds nothing but this fake.
func fakeFestListing(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"case \"$1\" in\n" +
		"  list) echo '" + body + "' ;;\n" +
		"  next) if [ -f .runnable ]; then exit 0; fi; exit 1 ;;\n" +
		"esac\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "fest"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake fest: %v", err)
	}
	t.Setenv("PATH", dir)
}

// campRootAt makes a directory the hub detects as a camp and moves into it, so
// DetectCampaignRoot answers with a temp dir rather than whichever camp the
// developer happens to be running the tests from. It returns the path as the
// process sees it, which is what DetectCampaignRoot will compute.
func campRootAt(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".campaign"), 0o755); err != nil {
		t.Fatalf("mkdir .campaign: %v", err)
	}
	t.Chdir(root)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	return wd
}

// runnableFestival makes a real directory fest's fake will accept.
func runnableFestival(t *testing.T, camp string) string {
	t.Helper()
	dir := filepath.Join(camp, "festivals", "active", "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir festival: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".runnable"), nil, 0o644); err != nil {
		t.Fatalf("mark runnable: %v", err)
	}
	return dir
}

// TestRunTourStep_FestNextRunsInsideTheResolvedFestival is the regression test
// for a step that could never finish. fest next fails outside a festival
// directory, so launching it at the camp root left the box permanently empty.
func TestRunTourStep_FestNextRunsInsideTheResolvedFestival(t *testing.T) {
	tourEnv(t)
	camp := campRootAt(t)
	fest := runnableFestival(t, camp)
	fakeFestListing(t, `{"active":[{"name":"demo","path":"`+fest+`","status":"active"}],"total":1}`)

	m := tourAt(t, app.TourStepFestNext)
	next, cmd := m.handleEnter()
	nm := next.(model)

	if nm.pendingLaunch == nil {
		t.Fatal("the step must schedule a launch")
	}
	if cmd == nil {
		t.Fatal("the step must quit so the child gets the terminal")
	}
	if got := strings.Join(nm.pendingLaunch.Args, " "); got != "next" {
		t.Fatalf("args = %q, want next", got)
	}
	if nm.pendingLaunch.Dir != fest {
		t.Fatalf("dir = %q, want the resolved festival %q", nm.pendingLaunch.Dir, fest)
	}
	if nm.pendingThen != nil {
		t.Fatal("a festival that already runs needs no second child")
	}
	if nm.recordTourStep != app.TourStepFestNext {
		t.Fatalf("recordTourStep = %q, want %q", nm.recordTourStep, app.TourStepFestNext)
	}
	if nm.launchBanner != "" {
		t.Fatalf("banner = %q: running fest next needs no explanation", nm.launchBanner)
	}
}

// TestRunTourStep_FestNextScaffoldsAWorkflowWhenTheCampHasNothing covers the
// empty camp. A brand new festival would fail its own validation, so the step
// scaffolds a standalone workflow, which fest starts a run for as it writes it,
// and runs fest next there in the same pass.
func TestRunTourStep_FestNextScaffoldsAWorkflowWhenTheCampHasNothing(t *testing.T) {
	tourEnv(t)
	camp := campRootAt(t)
	fakeFestListing(t, `{"total":0}`)

	m := tourAt(t, app.TourStepFestNext)
	next, cmd := m.handleEnter()
	nm := next.(model)

	if nm.pendingLaunch == nil || cmd == nil {
		t.Fatal("an empty camp must still offer a way forward, not a dead step")
	}
	args := nm.pendingLaunch.Args
	if len(args) < 3 || args[0] != "create" || args[1] != "workflow" {
		t.Fatalf("args = %v, want a create workflow launch", args)
	}
	if args[len(args)-2] != "--steps" || args[len(args)-1] == "" {
		t.Fatalf("the workflow must be scaffolded from compiled-in steps, got %v", args)
	}
	wantDir := filepath.Join(camp, "workflow", app.GettingStartedWorkflowName)
	if nm.pendingLaunch.Dir != wantDir {
		t.Fatalf("dir = %q, want %q", nm.pendingLaunch.Dir, wantDir)
	}
	if nm.pendingThen == nil {
		t.Fatal("scaffolding must be followed by fest next in the same pass")
	}
	if got := strings.Join(nm.pendingThen.Args, " "); got != "next" {
		t.Fatalf("second child args = %q, want next", got)
	}
	if nm.pendingThen.Dir != wantDir {
		t.Fatalf("second child dir = %q, want %q", nm.pendingThen.Dir, wantDir)
	}
	if nm.recordTourStep != app.TourStepFestNext {
		t.Fatalf("recordTourStep = %q, want the step to tick on a clean pass", nm.recordTourStep)
	}
	if nm.launchBanner == "" {
		t.Fatal("writing a directory into the user's camp must be reported")
	}
	if nm.err != nil {
		t.Fatalf("an empty camp is not an error: %v", nm.err)
	}
}

// TestRunTourStep_FestNextStartsAFinishedWorkflowAgain covers the repeat visit.
// fest next refuses in a workflow whose last run completed, and scaffolding over
// it fails because fest will not overwrite an existing WORKFLOW.md.
func TestRunTourStep_FestNextStartsAFinishedWorkflowAgain(t *testing.T) {
	tourEnv(t)
	camp := campRootAt(t)
	wf := filepath.Join(camp, "workflow", app.GettingStartedWorkflowName)
	if err := os.MkdirAll(filepath.Join(wf, ".workflow"), 0o755); err != nil {
		t.Fatalf("mkdir workflow: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wf, "WORKFLOW.md"), []byte("# wf"), 0o644); err != nil {
		t.Fatalf("write WORKFLOW.md: %v", err)
	}
	fakeFestListing(t, `{"total":0}`)

	m := tourAt(t, app.TourStepFestNext)
	next, _ := m.handleEnter()
	nm := next.(model)

	if nm.pendingLaunch == nil {
		t.Fatal("a finished workflow must be restartable, not a dead end")
	}
	if got := strings.Join(nm.pendingLaunch.Args, " "); got != "workflow start" {
		t.Fatalf("args = %q, want workflow start", got)
	}
	if nm.pendingLaunch.Dir != wf {
		t.Fatalf("dir = %q, want %q", nm.pendingLaunch.Dir, wf)
	}
	if nm.pendingThen == nil || strings.Join(nm.pendingThen.Args, " ") != "next" {
		t.Fatal("starting a run must be followed by fest next in the same pass")
	}
}

// TestRunTourStep_FestNextWillNotScaffoldOverACampItCouldNotCheck is the guard
// on the probe bound. Giving up early means the answer is unknown, and dropping
// a getting started workflow into a camp full of real festivals is not it.
func TestRunTourStep_FestNextWillNotScaffoldOverACampItCouldNotCheck(t *testing.T) {
	tourEnv(t)
	camp := campRootAt(t)
	var entries []string
	for i := 0; i < 7; i++ {
		dir := filepath.Join(camp, "festivals", "active", "f"+string(rune('a'+i)))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		entries = append(entries, `{"name":"f","path":"`+dir+`","status":"active"}`)
	}
	fakeFestListing(t, `{"active":[`+strings.Join(entries, ",")+`],"total":7}`)

	m := tourAt(t, app.TourStepFestNext)
	next, _ := m.handleEnter()
	nm := next.(model)

	if nm.pendingLaunch != nil {
		t.Fatalf("must not launch anything, got %v", nm.pendingLaunch.Args)
	}
	if nm.err == nil {
		t.Fatal("the user must be told the camp could not be checked")
	}
}

// TestRunTourStep_FestNextReportsAnUnreadableListing keeps a broken fest from
// looking like an empty camp, which would offer to create a festival that may
// already exist.
func TestRunTourStep_FestNextReportsAnUnreadableListing(t *testing.T) {
	tourEnv(t)
	campRootAt(t)
	fakeFestListing(t, `not json at all`)

	m := tourAt(t, app.TourStepFestNext)
	next, _ := m.handleEnter()
	nm := next.(model)

	if nm.err == nil {
		t.Fatal("a listing the hub cannot read must surface an error")
	}
	if nm.pendingLaunch != nil {
		t.Fatal("a listing the hub cannot read must not schedule a launch")
	}
}

func TestRunTourStep_FestNextWithoutFestReportsInsteadOfLaunching(t *testing.T) {
	tourEnv(t)
	m := tourAt(t, app.TourStepFestNext)
	next, _ := m.handleEnter()
	nm := next.(model)
	if nm.err == nil {
		t.Fatal("a missing fest must surface an error")
	}
	if nm.pendingLaunch != nil {
		t.Fatal("a missing fest must not schedule a launch")
	}
}

func TestRecordTourStepAfterChild(t *testing.T) {
	tests := []struct {
		name     string
		key      app.TourStepKey
		res      launch.Result
		wantDone bool
	}{
		{
			name:     "clean exit records the step",
			key:      app.TourStepFestNext,
			res:      launch.Result{Started: true, ExitCode: 0},
			wantDone: true,
		},
		{
			name: "a non-zero exit records nothing",
			key:  app.TourStepFestNext,
			res:  launch.Result{Started: true, ExitCode: 2},
		},
		{
			name: "a child that never started records nothing",
			key:  app.TourStepFestNext,
			res:  launch.Result{Started: false, ExitCode: -1},
		},
		{
			name: "an ordinary launchpad launch records nothing",
			res:  launch.Result{Started: true, ExitCode: 0},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tourEnv(t)
			ctx := context.Background()
			if err := recordTourStepAfterChild(ctx, tc.key, tc.res); err != nil {
				t.Fatalf("recordTourStepAfterChild: %v", err)
			}
			tour, err := app.LoadTour(ctx)
			if err != nil {
				t.Fatalf("LoadTour: %v", err)
			}
			step, _ := tour.Step(app.TourStepFestNext)
			if got := step.Done(); got != tc.wantDone {
				t.Fatalf("fest step done = %v, want %v", got, tc.wantDone)
			}
		})
	}
}

func TestSkipTourStep_RecordsSkippedAndReloads(t *testing.T) {
	tourEnv(t)
	m := tourAt(t, app.TourStepFestNext)
	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if cmd == nil {
		t.Fatal("skip must return a command that records and reloads")
	}
	_ = next

	msg, ok := cmd().(tourMsg)
	if !ok {
		t.Fatalf("command returned %T, want tourMsg", cmd())
	}
	if msg.err != nil {
		t.Fatalf("skip reported: %v", msg.err)
	}
	step, _ := msg.tour.Step(app.TourStepFestNext)
	if step.State != app.TourStepSkipped {
		t.Fatalf("step state = %q, want skipped", step.State)
	}
	if step.Done() {
		t.Fatal("a skipped step must never read as done")
	}
}

func TestSkipTourStep_WillNotUndoAFinishedStep(t *testing.T) {
	tourEnv(t)
	m := tourAt(t, app.TourStepFestNext)
	m.tour.Steps[m.cursor].State = app.TourStepDone
	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if cmd != nil {
		t.Fatal("skipping a finished step must do nothing")
	}
}

// TestTourEscapeDoesNotDismiss pins that leaving the screen is not the same as
// closing the tour: only an explicit action sets the dismissed flag.
func TestTourEscapeDoesNotDismiss(t *testing.T) {
	tourEnv(t)
	ctx := context.Background()
	m := tourAt(t, app.TourStepInstall)
	m.ctx = ctx
	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if got := next.(model).screen; got != screenHome {
		t.Fatalf("screen = %v, want screenHome", got)
	}
	tour, err := app.LoadTour(ctx)
	if err != nil {
		t.Fatalf("LoadTour: %v", err)
	}
	if tour.Dismissed {
		t.Fatal("escaping out of the tour must not mark it dismissed")
	}
}

// TestConfirmReturn_DoesNotLeakBetweenDialogs is the regression test for a
// confirmation raised by the tour, backed out of with esc, leaving its return
// screen behind so that declining a later unrelated confirmation landed on the
// tour instead of home.
func TestConfirmReturn_DoesNotLeakBetweenDialogs(t *testing.T) {
	exits := []struct {
		name string
		key  tea.KeyMsg
	}{
		{name: "esc", key: tea.KeyMsg{Type: tea.KeyEsc}},
		{name: "q", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}},
	}
	for _, exit := range exits {
		t.Run(exit.name, func(t *testing.T) {
			home := tourEnv(t)
			if err := os.MkdirAll(filepath.Join(home, "bin"), 0o755); err != nil {
				t.Fatalf("mkdir bin: %v", err)
			}

			m := tourAt(t, app.TourStepPath)
			next, _ := m.handleEnter()
			nm := next.(model)
			if nm.screen != screenConfirm {
				t.Fatalf("screen = %v, want screenConfirm", nm.screen)
			}

			backed, _ := nm.handleKey(exit.key)
			bm := backed.(model)
			if bm.screen != screenHome {
				t.Fatalf("screen after %s = %v, want screenHome", exit.name, bm.screen)
			}
			if bm.confirmReturn != screenBoot {
				t.Fatalf("confirmReturn survived %s: %v", exit.name, bm.confirmReturn)
			}

			// An unrelated confirmation, declined, must land on home.
			bm.screen = screenConfirm
			bm.confirmAct = "uninstall"
			bm.confirmYes = false
			done, _ := bm.handleEnter()
			if got := done.(model).screen; got != screenHome {
				t.Fatalf("declining an unrelated confirm went to %v, want screenHome", got)
			}
		})
	}
}

func TestConfirmReturn_UninstallDeclineGoesHome(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.screen = screenConfirm
	m.confirmAct = "uninstall"
	m.confirmReturn = screenTour // a stale value from an earlier dialog
	m.confirmYes = false

	// The uninstall dialog sets its own return screen, so opening it clears
	// whatever was there before.
	m.confirmReturn = screenHome
	next, _ := m.handleEnter()
	if got := next.(model).screen; got != screenHome {
		t.Fatalf("screen = %v, want screenHome", got)
	}
}

func TestDismissTour_RecordsOnlyOnAnExplicitKey(t *testing.T) {
	ctx := context.Background()
	tourEnv(t)

	m := tourAt(t, app.TourStepInstall)
	m.ctx = ctx
	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd == nil {
		t.Fatal("d must return a command that records the dismissal")
	}
	msg, ok := cmd().(tourMsg)
	if !ok {
		t.Fatalf("command returned %T, want tourMsg", cmd())
	}
	if msg.err != nil {
		t.Fatalf("dismiss reported: %v", msg.err)
	}
	if !msg.tour.Dismissed {
		t.Fatal("the reloaded tour must report itself dismissed")
	}

	tour, err := app.LoadTour(ctx)
	if err != nil {
		t.Fatalf("LoadTour: %v", err)
	}
	if !tour.Dismissed {
		t.Fatal("the dismissal must survive a reload")
	}
	if tour.Complete() {
		t.Fatal("dismissing must not mark the steps done")
	}
}
