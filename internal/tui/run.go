package tui

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	// Seed dark background before any lipgloss/termenv OSC query.
	_ "github.com/Obedience-Corp/obey-installer/internal/bginit"
	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
	"github.com/Obedience-Corp/obey-installer/internal/launch"
)

// resumeState carries launchpad position across Pattern-1 program restarts.
type resumeState struct {
	active bool
	screen screen
	cursor int
}

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
	var resume resumeState
	for {
		if err := ctx.Err(); err != nil {
			return SessionResult{Quit: true}, err
		}
		sess, err := runOnce(ctx, opts, banner, resume)
		if err != nil {
			return sess, err
		}
		if sess.Quit || sess.Launch == nil {
			return sess, sess.Err
		}

		// Remember launchpad cursor for post-child resume.
		resume = resumeState{
			active: true,
			screen: screenLaunchpad,
			cursor: sess.ResumeCursor,
		}

		// Child owns the terminal; hub alt-screen is already gone.
		if _, err := fmt.Fprintf(opts.stderr(), "\n▸ launching %s … (quit the tool to return to festival)\n\n", launchLabel(*sess.Launch)); err != nil {
			return SessionResult{Quit: true}, errpkg.Wrap("E_TUI_LAUNCH_BANNER", err, "write launch banner")
		}
		res := launch.Run(ctx, *sess.Launch)
		resetTerminalAfterChild()
		banner = formatChildBanner(*sess.Launch, res)
		// Loop: re-enter hub TUI on launchpad with status refresh via Init.
	}
}

func runOnce(ctx context.Context, opts Options, banner string, resume resumeState) (SessionResult, error) {
	m := newModel(opts)
	m.ctx = ctx
	m.banner = banner
	switch {
	case resume.active:
		m.screen = resume.screen
		m.cursor = resume.cursor
		m.bootLeft = 0
	case banner != "" || opts.SkipBoot:
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
		return SessionResult{
			Launch:       &spec,
			ResumeCursor: fm.cursor,
		}, nil
	}
	return SessionResult{Quit: true}, nil
}

func launchLabel(s launch.Spec) string {
	if s.Title != "" {
		return s.Title
	}
	return s.Tool
}

// formatChildBanner distinguishes resolve/start failures from signalled or
// non-zero child exits so Ctrl+C in a tool does not look like a launch failure.
func formatChildBanner(spec launch.Spec, res launch.Result) string {
	label := launchLabel(spec)
	if !res.Started {
		if res.Err != nil {
			return fmt.Sprintf("could not launch %s: %v", label, res.Err)
		}
		return fmt.Sprintf("could not launch %s", label)
	}
	if res.Signal != "" {
		return fmt.Sprintf("returned from %s (%s) — back in festival hub", label, res.Signal)
	}
	if res.ExitCode != 0 {
		return fmt.Sprintf("returned from %s (exit %d)", label, res.ExitCode)
	}
	return fmt.Sprintf("returned from %s — back in festival hub", label)
}

// resetTerminalAfterChild best-effort restores cursor and SGR if a child left
// the TTY dirty (raw mode / hidden cursor / partial alt-screen).
func resetTerminalAfterChild() {
	// show cursor; reset SGR; disable mouse tracking common leftovers
	_, _ = fmt.Fprint(os.Stdout, "\x1b[?25h\x1b[0m\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l")
}
