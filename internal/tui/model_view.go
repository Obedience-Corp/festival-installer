package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Obedience-Corp/obey-installer/internal/tui/anim"
	"github.com/Obedience-Corp/obey-installer/internal/tui/components"
)

func (m model) View() string {
	w := m.width
	if w <= 0 {
		w = 80
	}
	// Keep chrome inside the frame; reserve a little margin for alt-screen edges.
	if w > 4 {
		w -= 0 // full width; lipgloss handles display width
	}
	s := m.styles
	var body string
	title := "home"
	footer := "↑↓ navigate  enter select  ? help  q quit"

	switch m.screen {
	case screenBoot:
		return anim.BootView(m.frame, w, s, m.reduced) + "\n" + s.Muted.Render("  enter to skip")
	case screenHome:
		title = "home"
		body = m.viewHome()
	case screenInstall:
		title = "install"
		body = m.viewInstall()
		footer = "←→ channel  enter install  esc back"
	case screenUpdate, screenProgress:
		title = "working"
		body = anim.ProgressFlame(m.progress.Percent, m.progress.Stage, m.progress.Message, m.animationFrame(), s)
		footer = "ctrl+c cancel"
	case screenList:
		title = "installed"
		body = m.viewList()
		footer = "r refresh  esc back"
	case screenBrowse:
		title = "browse"
		body = m.viewBrowse()
		footer = "f product  c class  enter install  r refresh  esc back"
	case screenUninstall:
		title = "uninstall"
		body = m.viewList()
		footer = "enter uninstall  esc back"
	case screenMarketplace:
		title = "marketplaces"
		body = m.viewMarketplace()
		footer = "a add  enter refresh  r reload  esc back"
	case screenDoctor:
		title = "doctor"
		body = m.viewDoctor()
		footer = "r re-run  esc back"
	case screenShell:
		title = "shell"
		body = m.viewShell()
		footer = "esc back"
	case screenConfirm:
		title = "confirm"
		body = components.ConfirmBox(m.confirmMsg, m.confirmYes, s)
		footer = "←→  y/n  enter"
	case screenResult:
		title = "result"
		body = m.viewResult()
		footer = "enter continue  esc home"
	}

	if m.help {
		body = components.HelpOverlay(s)
	}

	header := components.Header(title, m.opts.Version, w, s)
	foot := components.Footer(footer, w, s)
	parts := []string{header, body}
	if m.err != nil && m.screen != screenResult {
		parts = append(parts, components.ErrorBox(m.err, s))
	}
	parts = append(parts, foot)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m model) viewHome() string {
	s := m.styles
	var status string
	switch m.status.Action {
	case "managed":
		status = components.StatusLine(fmt.Sprintf("festival %s (%s) · managed", m.status.Version, m.status.Channel), "ok", s)
	case "unmanaged":
		status = components.StatusLine("camp/fest found but not managed by festival", "warn", s)
	case "absent":
		status = components.StatusLine("festival suite not installed", "fail", s)
	default:
		status = components.StatusLine("checking status…", "", s)
	}
	pathKind := "fail"
	pathText := "managed bin not on PATH"
	binShort := m.status.ManagedBin
	if len(binShort) > 48 {
		binShort = "…" + binShort[len(binShort)-47:]
	}
	if m.status.ManagedBinOnPath {
		pathKind = "ok"
		pathText = "PATH ok · " + binShort
	} else if m.status.ManagedBin != "" {
		pathText = "PATH missing · " + binShort
	}

	var flame string
	if m.reduced {
		flame = anim.StaticFlame(s)
	} else {
		flame = anim.Flame(m.frame, 1, s)
	}
	booths := anim.RenderBooths(anim.DefaultHomeBooths(homeBoothIndex(m.cursor)), m.animationFrame(), s)
	center := lipgloss.JoinVertical(lipgloss.Center, flame, "", booths)
	menu := components.Menu(homeItems, m.cursor, s)
	tag := s.Tagline.Render(anim.Tagline)

	return status + "\n" + components.StatusLine(pathText, pathKind, s) + "\n\n" +
		center + "\n\n" + menu + "\n" + tag
}

func (m model) animationFrame() int {
	if m.reduced {
		return 0
	}
	return m.frame
}

func (m model) viewInstall() string {
	s := m.styles
	var ch strings.Builder
	for i, c := range m.channels {
		if i == m.channelIdx {
			ch.WriteString(s.Selected.Render("▸ " + c + " "))
		} else {
			ch.WriteString(s.Muted.Render("  " + c + " "))
		}
	}
	return s.Title.Render("Install Festival suite (camp + fest)") + "\n\n" +
		s.Muted.Render("channel") + "\n" + ch.String() + "\n\n" +
		s.Fire.Render("enter") + s.Muted.Render(" to install")
}

func (m model) viewList() string {
	s := m.styles
	if len(m.list.Packages) == 0 {
		return s.Muted.Render("no packages installed yet")
	}
	items := make([]string, len(m.list.Packages))
	for i, p := range m.list.Packages {
		items[i] = fmt.Sprintf("%s  %s  (%s)", p.PackageID, p.Version, p.Channel)
	}
	return components.Menu(items, m.cursor, s)
}

func (m model) viewBrowse() string {
	s := m.styles
	filt := s.Muted.Render(fmt.Sprintf("filter product=%q class=%q", m.productF, m.kindF))
	if len(m.browseFlat) == 0 {
		return filt + "\n\n" + s.Muted.Render("no packages match (add a marketplace or refresh)")
	}
	items := make([]string, len(m.browseFlat))
	for i, p := range m.browseFlat {
		items[i] = fmt.Sprintf("%s  [%s]  %s", p.ID, p.Class, p.Source)
	}
	return filt + "\n\n" + components.Menu(items, m.cursor, s)
}

func (m model) viewMarketplace() string {
	s := m.styles
	if m.marketMode == "add" {
		return s.Title.Render("Add marketplace") + "\n\n" + m.addInput.View() + "\n\n" +
			s.Muted.Render("enter to add · esc cancel")
	}
	items := make([]string, 0, len(m.markets)+1)
	for _, v := range m.markets {
		line := fmt.Sprintf("%s  %s  pkgs=%d", v.Name, short(v.Commit, 12), v.Packages)
		if v.Err != "" {
			line += "  ERR:" + v.Err
		}
		items = append(items, line)
	}
	items = append(items, "↻ refresh all")
	return components.Menu(items, m.cursor, s) + "\n" + s.Muted.Render("a to add a git marketplace")
}

func (m model) viewDoctor() string {
	s := m.styles
	if len(m.checks) == 0 {
		return s.Muted.Render("running checks…")
	}
	var b strings.Builder
	// multi-booth style checks; wrap long messages so failures are fully readable.
	msgWidth := m.width - 18
	if msgWidth < 24 {
		msgWidth = 24
	}
	for i, c := range m.checks {
		spin := "·"
		if !m.reduced {
			spin = []string{"·", "°", "*", "✦"}[(m.frame+i)%4]
		}
		var badge string
		switch c.Status {
		case "ok":
			badge = s.OK.Render(fmt.Sprintf("[%s ok] ", spin))
		case "warn":
			badge = s.Warn.Render(fmt.Sprintf("[%s warn] ", spin))
		default:
			badge = s.Err.Render(fmt.Sprintf("[%s fail] ", spin))
		}
		msg := c.ID + " — " + c.Message
		wrapped := wrapWords(msg, msgWidth)
		for j, part := range wrapped {
			if j == 0 {
				b.WriteString(badge + s.Normal.Render(part))
			} else {
				b.WriteString(s.Muted.Render("         ") + s.Normal.Render(part))
			}
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func wrapWords(text string, width int) []string {
	if width < 8 || len(text) <= width {
		return []string{text}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{text}
	}
	var lines []string
	var cur strings.Builder
	for _, w := range words {
		if cur.Len() == 0 {
			cur.WriteString(w)
			continue
		}
		if cur.Len()+1+len(w) > width {
			lines = append(lines, cur.String())
			cur.Reset()
			cur.WriteString(w)
			continue
		}
		cur.WriteByte(' ')
		cur.WriteString(w)
	}
	if cur.Len() > 0 {
		lines = append(lines, cur.String())
	}
	return lines
}

func (m model) viewShell() string {
	s := m.styles
	status := "NOT on PATH"
	kind := "fail"
	if m.shellOnPath {
		status = "on PATH"
		kind = "ok"
	}
	return components.StatusLine(m.shellBin+" · "+status, kind, s) + "\n\n" +
		s.Muted.Render("add to your shell rc:") + "\n\n" +
		s.FireTip.Render(m.shellSnippet)
}

func (m model) viewResult() string {
	s := m.styles
	title := s.Title.Render(m.resultTitle)
	if m.resultOK {
		if !m.reduced {
			return title + "\n\n" + anim.Celebrate(m.frame, s) + "\n\n" + s.Normal.Render(m.resultBody)
		}
		return title + "\n\n" + s.OK.Render("done") + "\n\n" + s.Normal.Render(m.resultBody)
	}
	return title + "\n\n" + s.Err.Render(m.resultBody)
}

func short(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// homeBoothIndex maps the home menu cursor onto ambient booths so the
// multi-activity strip tracks what the user is looking at.
func homeBoothIndex(cursor int) int {
	switch cursor {
	case 0, 1: // install / update
		return 0
	case 2, 3, 4: // list / browse / uninstall
		return 1
	case 5: // marketplaces
		return 2
	case 6: // doctor
		return 3
	case 7: // shell / path
		return 4
	default:
		return 0
	}
}
