package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	// Seed dark background before any lipgloss/termenv OSC query.
	_ "github.com/Obedience-Corp/obey-installer/internal/bginit"
)

// Run starts the full-screen Festival manager TUI.
func Run(ctx context.Context, opts Options) error {
	// Force a dark, truecolor profile so Init never blocks on OSC 11 / DSR.
	lipgloss.SetHasDarkBackground(true)
	lipgloss.SetColorProfile(termenv.TrueColor)

	m := newModel(opts)
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return fmt.Errorf("festival tui: %w", err)
	}
	if fm, ok := final.(model); ok && fm.quitErr != nil {
		return fm.quitErr
	}
	return nil
}
