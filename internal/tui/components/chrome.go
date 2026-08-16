package components

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Obedience-Corp/festival-installer/internal/textsafe"
	"github.com/Obedience-Corp/festival-installer/internal/tui/theme"
)

// brandMark is a single-cell fire chip (avoid emoji width quirks in VHS/pty).
const brandMark = "▲"

// Header renders brand + title + version.
func Header(title, version string, width int, s theme.Styles) string {
	if width < 20 {
		width = 20
	}
	left := s.Fire.Render(brandMark + " festival")
	if title != "" && title != "home" {
		left += s.Muted.Render("  ·  ") + s.Title.Render(title)
	}
	right := s.Muted.Render(version)
	// Place left and right on one row without over-wide spacer math.
	row := lipgloss.NewStyle().Width(width).Render(
		lipgloss.JoinHorizontal(lipgloss.Top,
			left,
			lipgloss.NewStyle().Width(max(1, width-lipgloss.Width(left)-lipgloss.Width(right))).Render(""),
			right,
		),
	)
	return row + "\n" + theme.Rule(width, s)
}

// Footer renders key hints.
func Footer(hints string, width int, s theme.Styles) string {
	if width < 20 {
		width = 20
	}
	return theme.Rule(width, s) + "\n" + s.Footer.Width(width).MaxWidth(width).Render(hints)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Menu renders a selectable list.
func Menu(items []string, cursor int, s theme.Styles) string {
	var b strings.Builder
	for i, it := range items {
		prefix := "  "
		style := s.Normal
		if i == cursor {
			prefix = "▸ "
			style = s.Selected
		}
		b.WriteString(style.Render(prefix + it))
		b.WriteByte('\n')
	}
	return b.String()
}

// StatusLine formats a one-line status strip.
func StatusLine(text string, kind string, s theme.Styles) string {
	switch kind {
	case "ok":
		return s.StatusOK.Render("● ") + s.Normal.Render(text)
	case "warn":
		return s.StatusWarn.Render("● ") + s.Normal.Render(text)
	case "fail":
		return s.StatusFail.Render("● ") + s.Normal.Render(text)
	default:
		return s.Muted.Render("○ ") + s.Normal.Render(text)
	}
}

// ConfirmBox prompts yes/no.
func ConfirmBox(msg string, yesSelected bool, s theme.Styles) string {
	yes := s.Normal.Render("[ yes ]")
	no := s.Normal.Render("[ no ]")
	if yesSelected {
		yes = s.Selected.Render("[ yes ]")
	} else {
		no = s.Selected.Render("[ no ]")
	}
	return s.Title.Render(msg) + "\n\n" + yes + "  " + no
}

// friendlyError is implemented by warnings (e.g. app.MarketplaceSeedWarning)
// whose Error() carries diagnostic detail, such as raw git command output,
// that must never reach the terminal.
type friendlyError interface {
	Friendly() string
}

// ErrorBox shows an error without using brand orange. Errors that carry a
// Friendly() rendering are non-fatal warnings and are presented as such.
func ErrorBox(err error, s theme.Styles) string {
	if err == nil {
		return ""
	}
	var f friendlyError
	if errors.As(err, &f) {
		return s.Warn.Render("warning: ") + s.Normal.Render(textsafe.Block(f.Friendly()))
	}
	return s.Err.Render("error: ") + s.Normal.Render(textsafe.Block(err.Error()))
}

// HelpOverlay lists global keys.
func HelpOverlay(s theme.Styles) string {
	lines := []string{
		s.Fire.Render("keys"),
		s.Muted.Render("↑↓/jk  move"),
		s.Muted.Render("enter  select"),
		s.Muted.Render("1-9    home menus (first nine)"),
		s.Muted.Render("0      quit (home)"),
		s.Muted.Render("esc    back"),
		s.Muted.Render("q      quit festival (home) / back"),
		s.Muted.Render("?      toggle help"),
		"",
		s.Tagline.Render(fmt.Sprintf("%q", "a lot of different things going on")),
	}
	return strings.Join(lines, "\n")
}
