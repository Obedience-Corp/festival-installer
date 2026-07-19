package theme

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Festival fire brand palette (fest.build flame logo).
const (
	Fire      = "#F2721C"
	FireCore  = "#EA5513"
	FireTip   = "#FFB347"
	FireEmber = "#C2410C"
	Void      = "#0d0d11"
	Raised    = "#16161a"
	FG        = "#f8f8f2"
	Muted     = "#a1a1aa"
	OK        = "#50fa7b"
	Warn      = "#febc2e"
	Err       = "#ff5f57"
)

// Styles holds lipgloss styles for the manager TUI.
type Styles struct {
	Title      lipgloss.Style
	Subtitle   lipgloss.Style
	Muted      lipgloss.Style
	Fire       lipgloss.Style
	FireCore   lipgloss.Style
	FireTip    lipgloss.Style
	Selected   lipgloss.Style
	Normal     lipgloss.Style
	OK         lipgloss.Style
	Warn       lipgloss.Style
	Err        lipgloss.Style
	Border     lipgloss.Style
	Header     lipgloss.Style
	Footer     lipgloss.Style
	StatusOK   lipgloss.Style
	StatusWarn lipgloss.Style
	StatusFail lipgloss.Style
	Tagline    lipgloss.Style
}

// New returns default festival styles.
func New() Styles {
	return Styles{
		Title:      lipgloss.NewStyle().Foreground(lipgloss.Color(FG)).Bold(true),
		Subtitle:   lipgloss.NewStyle().Foreground(lipgloss.Color(Muted)),
		Muted:      lipgloss.NewStyle().Foreground(lipgloss.Color(Muted)),
		Fire:       lipgloss.NewStyle().Foreground(lipgloss.Color(Fire)).Bold(true),
		FireCore:   lipgloss.NewStyle().Foreground(lipgloss.Color(FireCore)),
		FireTip:    lipgloss.NewStyle().Foreground(lipgloss.Color(FireTip)),
		Selected:   lipgloss.NewStyle().Foreground(lipgloss.Color(Fire)).Bold(true).PaddingLeft(1),
		Normal:     lipgloss.NewStyle().Foreground(lipgloss.Color(FG)).PaddingLeft(1),
		OK:         lipgloss.NewStyle().Foreground(lipgloss.Color(OK)),
		Warn:       lipgloss.NewStyle().Foreground(lipgloss.Color(Warn)),
		Err:        lipgloss.NewStyle().Foreground(lipgloss.Color(Err)),
		Border:     lipgloss.NewStyle().Foreground(lipgloss.Color(FireEmber)),
		Header:     lipgloss.NewStyle().Foreground(lipgloss.Color(FG)),
		Footer:     lipgloss.NewStyle().Foreground(lipgloss.Color(Muted)),
		StatusOK:   lipgloss.NewStyle().Foreground(lipgloss.Color(OK)),
		StatusWarn: lipgloss.NewStyle().Foreground(lipgloss.Color(Warn)),
		StatusFail: lipgloss.NewStyle().Foreground(lipgloss.Color(Err)),
		Tagline:    lipgloss.NewStyle().Foreground(lipgloss.Color(Muted)).Italic(true),
	}
}

// ReducedMotion reports whether ambient animation should be disabled.
func ReducedMotion() bool {
	v := strings.TrimSpace(os.Getenv("FESTIVAL_REDUCED_MOTION"))
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// Rule draws a horizontal rule of the given width.
func Rule(width int, s Styles) string {
	if width < 1 {
		width = 1
	}
	// Cap extreme widths so a bad terminal size cannot flood the frame.
	if width > 300 {
		width = 300
	}
	return s.Border.MaxWidth(width).Render(strings.Repeat("─", width))
}
