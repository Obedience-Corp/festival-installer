package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	// Seed dark background before any lipgloss/termenv OSC query.
	_ "github.com/Obedience-Corp/obey-installer/internal/bginit"
	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
	"github.com/Obedience-Corp/obey-installer/internal/launch"
)

// Run starts the full-screen Festival hub TUI once and returns when the user
// quits or requests a child launch. Prefer RunLoop for the suspend→resume UX.
func Run(ctx context.Context, opts Options) error {
	_, err := RunLoop(ctx, opts)
	return err
}

// RunLoop runs the hub TUI, launching camp/fest children as requested, until
// the user quits. The festival process stays alive across child exits so the
// user never retypes `festival` to return home.
//
// Design: workflow/design/festival-hub-control-plane/03-launch-and-return-model.md
func RunLoop(ctx context.Context, opts Options) (SessionResult, error) {
	// Force a dark, truecolor profile so Init never blocks on OSC 11 / DSR.
	lipgloss.SetHasDarkBackground(true)
	lipgloss.SetColorProfile(termenv.TrueColor)

	var banner string
	for {
		if err := ctx.Err(); err != nil {
			return SessionResult{Quit: true}, err
		}
		sess, err := runOnce(ctx, opts, banner)
		if err != nil {
			return sess, err
		}
		if sess.Quit || sess.Launch == nil {
			return sess, sess.Err
		}

		// Child owns the terminal; hub alt-screen is already gone.
		if _, err := fmt.Fprintf(opts.stderr(), "\n▸ launching %s … (quit the tool to return to festival)\n\n", launchLabel(*sess.Launch)); err != nil {
			return SessionResult{Quit: true}, errpkg.Wrap("E_TUI_LAUNCH_BANNER", err, "write launch banner")
		}
		res := launch.Run(ctx, *sess.Launch)
		banner = formatChildBanner(*sess.Launch, res)
		// Loop: re-enter hub TUI.
	}
}

func runOnce(ctx context.Context, opts Options, banner string) (SessionResult, error) {
	m := newModel(opts)
	m.banner = banner
	// Skip boot splash when returning from a child tool.
	if banner != "" || opts.SkipBoot {
		m.screen = screenHome
		m.bootLeft = 0
	}
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return SessionResult{}, fmt.Errorf("festival tui: %w", err)
	}
	fm, ok := final.(model)
	if !ok {
		return SessionResult{Quit: true}, nil
	}
	if fm.quitErr != nil {
		return SessionResult{Quit: true, Err: fm.quitErr}, fm.quitErr
	}
	if fm.pendingLaunch != nil {
		spec := *fm.pendingLaunch
		return SessionResult{Launch: &spec}, nil
	}
	return SessionResult{Quit: true}, nil
}

func launchLabel(s launch.Spec) string {
	if s.Title != "" {
		return s.Title
	}
	return s.Tool
}

func formatChildBanner(spec launch.Spec, res launch.Result) string {
	label := launchLabel(spec)
	if res.Err != nil && res.ExitCode < 0 {
		return fmt.Sprintf("could not launch %s: %v", label, res.Err)
	}
	if res.ExitCode != 0 {
		return fmt.Sprintf("returned from %s (exit %d)", label, res.ExitCode)
	}
	return fmt.Sprintf("returned from %s — back in festival hub", label)
}
