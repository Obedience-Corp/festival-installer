package tui

import (
	"context"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/launch"
)

// selectedTourStep is the step under the cursor on the tour screen.
func (m model) selectedTourStep() (app.TourStep, bool) {
	if m.cursor < 0 || m.cursor >= len(m.tour.Steps) {
		return app.TourStep{}, false
	}
	return m.tour.Steps[m.cursor], true
}

// runTourStep does the selected step. Each step hands off to machinery that
// already exists: the install screen, the shell rc consent flow, and the
// launchpad's suspend, exec and resume loop. Nothing here spawns a child by
// itself.
func (m model) runTourStep() (tea.Model, tea.Cmd) {
	step, ok := m.selectedTourStep()
	if !ok {
		return m, nil
	}
	switch step.Key {
	case app.TourStepInstall:
		return m.openInstallScreen()
	case app.TourStepPath:
		return m.confirmTourPathAppend()
	case app.TourStepCampInit:
		return m.confirmTourCampInit()
	case app.TourStepFestNext:
		return m.runTourFestNext()
	}
	return m, nil
}

// tourFestivalCreatedBanner is what the hub says on the way back from creating a
// festival. The step is not finished by that child, and saying so here is the
// only place the user finds out why the box is still empty.
const tourFestivalCreatedBanner = "festival created, run this step again to start fest next in it"

// runTourFestNext resolves the directory fest next has to run inside.
//
// fest next fails outside a festival directory, so launching it at the camp root
// could never finish this step. When the camp holds no festival yet the step
// hands the user fest's own create flow instead and stays unfinished, because
// creating a festival is not the same as running fest next in one. Both branches
// go through the launchpad's suspend, exec and resume loop; neither spawns a
// child here.
func (m model) runTourFestNext() (tea.Model, tea.Cmd) {
	root := launch.DetectCampaignRoot("")
	dir, err := app.ResolveFestivalDir(m.ctx, root)
	switch {
	case err == nil:
		return m.launchTourStep(app.TourStepFestNext, launch.Spec{
			Tool:  "fest",
			Args:  []string{"next"},
			Dir:   dir,
			Title: "fest next",
		})
	case errpkg.Code(err) == app.CodeNoFestival:
		// Bare `fest create festival` opens fest's own interactive form, so the
		// user names it themselves rather than having the hub invent a name.
		next, cmd := m.launchTourStep("", launch.Spec{
			Tool:  "fest",
			Args:  []string{"create", "festival"},
			Dir:   root,
			Title: "fest create festival",
		})
		nm, ok := next.(model)
		if !ok || nm.pendingLaunch == nil {
			return next, cmd
		}
		nm.launchBanner = tourFestivalCreatedBanner
		return nm, cmd
	default:
		m.err = err
		m.screen = screenTour
		return m, nil
	}
}

// skipTourStep records that the user passed over the selected step. A skipped
// step renders as skipped, never as done, and the record survives a restart.
func (m model) skipTourStep() (tea.Model, tea.Cmd) {
	step, ok := m.selectedTourStep()
	if !ok {
		return m, nil
	}
	if step.State == app.TourStepDone {
		return m, nil
	}
	ctx := m.ctx
	return m, func() tea.Msg {
		if err := app.SkipTourStep(ctx, step.Key); err != nil {
			return tourMsg{err: err}
		}
		tour, err := app.LoadTour(ctx)
		return tourMsg{tour: tour, err: err}
	}
}

// dismissTour records that the user closed the tour deliberately. It stays in
// the home menu afterwards; what changes is that the hub stops treating it as
// the thing the user still has to get through. Leaving the screen with esc is
// not this: only pressing d is.
func (m model) dismissTour() (tea.Model, tea.Cmd) {
	ctx := m.ctx
	return m, func() tea.Msg {
		if err := app.DismissTour(ctx); err != nil {
			return tourMsg{err: err}
		}
		tour, err := app.LoadTour(ctx)
		return tourMsg{tour: tour, err: err}
	}
}

// confirmTourPathAppend shows the exact rc file and block, then asks. The rc
// file belongs to the user, so the tour never writes to it without a yes on
// this screen, matching what `festival shell-init --append` does at the CLI.
func (m model) confirmTourPathAppend() (tea.Model, tea.Cmd) {
	plan, err := app.PlanShellRCAppend(m.ctx, app.ShellFromEnv())
	if err != nil {
		m.err = err
		return m, nil
	}
	switch {
	case plan.Present:
		m.err = nil
		m.banner = plan.File + " already has the block. Restart your shell to pick it up."
		return m, nil
	case !plan.Appendable:
		m.err = errpkg.New("E_SHELL_RC_NO_EFFECT",
			"there is nothing here for a shell to run yet. Install the suite first.")
		return m, nil
	}
	m.confirmMsg = "Append the festival PATH block to " + plan.File + "?\n\n" + plan.Block
	m.confirmYes = false
	m.confirmAct = "tour-path"
	m.confirmArg = plan.Shell
	m.confirmReturn = screenTour
	m.screen = screenConfirm
	m.err = nil
	return m, nil
}

// applyTourPathAppend performs the append the user just approved. The step is
// not recorded as done: only an observed PATH promotes it, because a user who
// never restarts their shell does not have a working PATH.
func (m model) applyTourPathAppend() (tea.Model, tea.Cmd) {
	shell := m.confirmArg
	m.confirmAct = ""
	m.confirmArg = ""
	m.confirmReturn = screenBoot
	m.screen = screenTour
	ctx := m.ctx
	return m, func() tea.Msg {
		plan, err := app.PlanShellRCAppend(ctx, shell)
		if err != nil {
			return tourMsg{err: err}
		}
		if err := app.ApplyShellRCAppend(ctx, plan); err != nil {
			return tourMsg{err: err}
		}
		tour, lerr := app.LoadTour(ctx)
		return tourMsg{tour: tour, err: lerr}
	}
}

// confirmTourCampInit names the directory before creating a camp in it. The hub
// has no better default than its own working directory, and turning whatever
// directory the user happened to launch from into a camp without saying so
// would be a surprise they cannot undo from here.
func (m model) confirmTourCampInit() (tea.Model, tea.Cmd) {
	if _, err := launch.Resolve(m.ctx, "camp"); err != nil {
		m.err = err
		return m, nil
	}
	dir, err := os.Getwd()
	if err != nil {
		m.err = errpkg.Wrap("E_TOUR_CWD", err, "resolve the current directory")
		return m, nil
	}
	m.confirmMsg = "Run camp init in " + dir + "?\n\nThis creates a camp workspace in that directory."
	m.confirmYes = false
	m.confirmAct = "tour-camp-init"
	m.confirmArg = dir
	m.confirmReturn = screenTour
	m.screen = screenConfirm
	m.err = nil
	return m, nil
}

// launchTourStep hands the terminal to a child through the launchpad's existing
// suspend, exec and resume loop.
//
// record names a step whose completion should be taken from the child's exit
// status. It is empty for a step whose result the hub can observe afterwards,
// because an exit code is the weaker evidence and should not be used when a
// real signal exists.
func (m model) launchTourStep(record app.TourStepKey, spec launch.Spec) (tea.Model, tea.Cmd) {
	if _, err := launch.Resolve(m.ctx, spec.Tool); err != nil {
		m.err = err
		m.screen = screenTour
		return m, nil
	}
	m.confirmAct = ""
	m.confirmArg = ""
	m.confirmReturn = screenBoot
	m.screen = screenTour
	m.recordTourStep = record
	m.launchBanner = ""
	cp := spec
	m.pendingLaunch = &cp
	return m, tea.Quit
}

// recordTourStepAfterChild is what the hub loop calls once a tour step's child
// has exited. Only a clean exit counts, so a tool the user quit out of or that
// failed does not mark the step done.
func recordTourStepAfterChild(ctx context.Context, key app.TourStepKey, res launch.Result) error {
	if key == "" {
		return nil
	}
	if !res.Started || res.ExitCode != 0 {
		return nil
	}
	return app.RecordTourStepDone(ctx, key)
}

func (m model) loadTour() tea.Cmd {
	ctx := m.ctx
	return func() tea.Msg {
		tour, err := app.LoadTour(ctx)
		return tourMsg{tour: tour, err: err}
	}
}

// tourCursor is where the tour screen parks the cursor: on the first step still
// to do, or on the last step once the tour is finished, so the screen opens on
// the thing the user has to act on.
func (m model) tourCursor() int {
	n := len(m.tour.Steps)
	if n == 0 {
		return 0
	}
	next := m.tour.NextStep()
	if next >= n {
		return n - 1
	}
	return next
}
