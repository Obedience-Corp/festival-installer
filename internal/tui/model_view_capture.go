package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/Obedience-Corp/obey-installer/internal/launch"
)

// viewChildOutput renders a capture-mode child: a status line and the
// scrollable output viewport.
func (m model) viewChildOutput() string {
	s := m.styles
	status := "running"
	if m.captureMode == launch.ModeStream && m.capture != nil {
		status = "streaming"
	}
	if m.captureRes != nil {
		status = childOutputStatus(*m.captureRes)
	}
	head := s.FireTip.Render("▸ " + m.captureTitle + " — " + status)
	return lipgloss.JoinVertical(lipgloss.Left, head, m.captureVP.View())
}

// childOutputStatus mirrors formatChildBanner's exit classification, shortened
// for the capture title line.
func childOutputStatus(res launch.Result) string {
	switch {
	case !res.Started:
		return "failed to start"
	case res.Signal != "":
		return "stopped (" + res.Signal + ")"
	case res.ExitCode != 0:
		return fmt.Sprintf("exit %d", res.ExitCode)
	default:
		return "done"
	}
}

func (m model) childOutputFooter() string {
	if m.capture != nil {
		return "↑↓ scroll  q/esc stop"
	}
	return "↑↓ scroll  q/esc back to launchpad"
}
