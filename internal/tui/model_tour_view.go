package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	"github.com/Obedience-Corp/festival-installer/internal/textsafe"
	"github.com/Obedience-Corp/festival-installer/internal/tui/theme"
)

// viewTour renders the four getting-started steps, which are done, and which
// one is next. Step state arrives through tourMsg; this function only draws it.
//
// Styles here are the unpadded ones (Title, Muted, StatusOK) rather than
// Normal and Selected, which carry a left pad that would shift the selected
// row out of line with the rest of the list.
func (m model) viewTour() string {
	s := m.styles
	steps := m.tour.Steps
	if len(steps) == 0 {
		return s.Muted.Render("  reading where you got to ...")
	}

	var b strings.Builder
	b.WriteString("  " + s.Title.Render("Getting started") + "\n")
	b.WriteString("  " + s.Muted.Render("four steps from an empty machine to a running fest next") + "\n")
	if m.tour.SignalsUnknown {
		// The first two steps read from the installer home. When that could not
		// be read, an unticked box means the hub cannot tell, not that the work
		// is outstanding, and saying which one it is belongs on screen.
		b.WriteString("  " + s.StatusWarn.Render("could not read the installer home, so steps 1 and 2 may be out of date") + "\n")
	}
	b.WriteString("\n")

	next := m.tour.NextStep()
	for i, st := range steps {
		b.WriteString(tourStepRow(st, i, i == m.cursor, i == next, s) + "\n")
		b.WriteString(tourStepNote(st.Detail, m.width, s.Muted))
		if st.Note != "" {
			b.WriteString(tourStepNote(st.Note, m.width, s.StatusWarn))
		}
	}

	switch {
	case m.tour.Complete():
		b.WriteString("\n  " + s.StatusOK.Render("all four steps are done, the suite is ready to use"))
	case m.tour.Dismissed:
		b.WriteString("\n  " + s.Muted.Render("tour dismissed, it stays in the menu if you want it back"))
	}
	return b.String()
}

// tourStepNote renders one indented line under a step, wrapped to the terminal
// so a long detail or a long rc path is readable instead of clipped at the
// right edge. The step list is the only text on this screen, so losing the end
// of a line loses the instruction.
func tourStepNote(text string, width int, style lipgloss.Style) string {
	const indent = "        "
	inner := width - len(indent)
	if inner < 20 {
		inner = 20
	}
	var b strings.Builder
	for _, line := range strings.Split(style.Width(inner).Render(textsafe.Line(text)), "\n") {
		b.WriteString(indent + strings.TrimRight(line, " ") + "\n")
	}
	return b.String()
}

func tourStepRow(st app.TourStep, i int, selected, isNext bool, s theme.Styles) string {
	cursor := "    "
	if selected {
		cursor = "  " + s.Fire.Render("\u25b8") + " "
	}
	mark, label := tourStepMark(st, s)
	row := cursor + mark + fmt.Sprintf(" %d. ", i+1) + label
	if isNext {
		row += s.FireTip.Render("   <- next")
	}
	return row
}

// tourStepMark renders a step's state. Pending and skipped get their own marks
// rather than collapsing into done or not-done: a PATH line written but not yet
// active, and a step the user chose to pass over, are different facts and a
// user acts differently on each.
func tourStepMark(st app.TourStep, s theme.Styles) (mark, label string) {
	title := textsafe.Line(st.Title)
	switch st.State {
	case app.TourStepDone:
		return s.StatusOK.Render("[x]"), s.Muted.Render(title)
	case app.TourStepPending:
		return s.StatusWarn.Render("[~]"), s.Title.Render(title)
	case app.TourStepSkipped:
		return s.Muted.Render("[-]"), s.Muted.Render(title)
	default:
		return s.Muted.Render("[ ]"), s.Title.Render(title)
	}
}
