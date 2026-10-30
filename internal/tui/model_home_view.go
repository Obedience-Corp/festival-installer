package tui

import (
	"fmt"
	"strings"

	"github.com/Obedience-Corp/festival-installer/internal/textsafe"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (m model) viewHome() string {
	s := m.styles
	w := max(20, m.width-4)
	var summary string
	switch {
	case m.statusErr != nil || m.status.Setup.Unknown():
		summary = "Suite status unavailable · Check setup to diagnose"
	case m.status.Action == "managed" || m.status.Action == "package":
		summary = "Festival suite " + textsafe.Line(m.status.Version) + " · installed"
		if m.status.Action == "package" {
			summary += " via " + textsafe.Line(packageChannelLabel(m.status))
		}
	case m.status.Action == "unmanaged":
		summary = "Camp / Fest found on PATH · not managed by Festival"
	case m.status.Action == "absent":
		summary = "Festival suite not installed · choose Install Festival suite"
	default:
		summary = "Checking your Festival suite..."
	}
	if m.updateAvailable() {
		summary += " · " + textsafe.Line(m.status.Latest) + " available"
	}
	notice := "Camp, Fest and Festival ship together. Add Obey and packages here."
	if m.status.Action == "managed" && !m.status.ManagedBinOnPath {
		notice = "Terminal setup needed · add the managed commands to PATH"
	}
	if m.status.Dual || len(m.status.Shadows) > 0 {
		notice = "Multiple installations found · Check setup before making changes"
	}
	top := s.Title.Render("Your tools") + "\n" + s.Normal.UnsetPadding().Render(ansi.Truncate(summary, w, "…")) + "\n" + s.Muted.Render(ansi.Truncate(notice, w, "…"))
	menu := m.homeMenu()
	// Reserve chrome, summary and the context line. Scroll the menu around the
	// selection rather than letting low terminal heights hide reachable actions.
	rows := max(3, m.height-11)
	if rows > len(menu) {
		rows = len(menu)
	}
	first := 0
	if m.cursor >= rows {
		first = m.cursor - rows + 1
	}
	last := min(len(menu), first+rows)
	wide := m.width >= 100
	menuWidth := w
	if wide {
		menuWidth = 36
	}
	lines := make([]string, 0, rows)
	for i := first; i < last; i++ {
		key := " "
		if i < 9 {
			key = fmt.Sprint(i + 1)
		}
		if menu[i].id == homeQuit {
			key = "q"
		}
		marker := " "
		style := s.Normal.UnsetPadding()
		if i == m.cursor {
			marker = "›"
			style = s.Selected.UnsetPadding().Background(lipgloss.Color("#252128"))
		}
		text := fmt.Sprintf(" %s %s  %s", marker, key, menu[i].label)
		lines = append(lines, style.Width(menuWidth).Render(ansi.Truncate(text, menuWidth, "…")))
	}
	body := strings.Join(lines, "\n")
	selected, _ := m.homeItemAt(m.cursor)
	detail := homeDescription(selected.id)
	if wide {
		rightWidth := w - menuWidth - 4
		right := s.Title.Render(selected.label) + "\n\n" + detail
		right = lipgloss.NewStyle().Width(rightWidth).Render(right)
		body = lipgloss.JoinHorizontal(lipgloss.Top, body, "    ", right)
	} else {
		body += "\n" + s.Muted.Render(ansi.Truncate(detail, w, "…"))
	}
	rangeNote := ""
	if first > 0 || last < len(menu) {
		rangeNote = fmt.Sprintf("  %d–%d of %d · ↑↓ more", first+1, last, len(menu))
	}
	return lipgloss.NewStyle().Padding(0, 2).Render(top + "\n\n" + body + "\n" + s.Muted.Render(rangeNote))
}

func homeDescription(id homeItemID) string {
	switch id {
	case homeLaunchpad:
		return "Work in your camps and festivals. Open a tool here, then return when you quit it."
	case homeObey:
		return "Run and chat with local agents. Install the Obey daemon and ob client, or check for updates."
	case homeBrowse:
		return "Add tools and plugins from your marketplaces. The Festival suite and Obey are separate products."
	case homeUpdate:
		return "Check installed tools and plugins for updates. Update one package or all eligible packages; keep existing sources, channels and running Obey sessions."
	case homeInstall:
		return "Check how your suite is installed. A missing suite can be installed here; an existing suite stays in place."
	case homeList:
		return "See installed packages, their versions and where they came from."
	case homeDoctor:
		return "Diagnose missing tools, PATH conflicts and incomplete setup."
	case homeShell:
		return "Find the terminal setup for your installed tools."
	case homeTour:
		return "Learn how camps and festivals fit together, and take your first steps."
	case homeMarketplace:
		return "Manage the catalogs used to discover and install packages."
	case homeUninstall:
		return "Remove a package installed by Festival. Review the selection before removing its files."
	case homeDiscord:
		return "Meet other users and get help in the Obedience Corp Discord."
	default:
		return "Leave Festival and return to your terminal."
	}
}
