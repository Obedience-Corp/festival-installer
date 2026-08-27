package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	"github.com/Obedience-Corp/festival-installer/internal/textsafe"
	"github.com/Obedience-Corp/festival-installer/internal/tui/anim"
	"github.com/Obedience-Corp/festival-installer/internal/tui/components"
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
	footer := "↑↓ navigate  enter select  1-9 menus  0/q quit  ? help"

	switch m.screen {
	case screenBoot:
		return anim.BootView(m.frame, w, s, m.reduced) + "\n" + s.Muted.Render("  enter to skip")
	case screenHome:
		title = "home"
		body = m.viewHome()
	case screenInstall:
		title = "install"
		body = m.viewInstall()
		switch m.installKind {
		case "package":
			footer = "enter back  f second copy  esc home"
		case "leftover":
			footer = "enter continue  esc back"
		default:
			footer = "←→ channel  enter install  esc back"
		}
	case screenUpdate, screenProgress:
		title = "working"
		body = m.viewWorking()
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
		footer = "a add  s seed official  enter refresh  r reload  esc back"
	case screenDoctor:
		title = "doctor"
		body = m.viewDoctor()
		footer = "r re-run  esc back"
	case screenShell:
		title = "shell"
		body = m.viewShell()
		footer = "esc back"
	case screenLaunchpad:
		title = "launchpad"
		body = m.viewLaunchpad()
		footer = "enter open tool · quit tool returns here · esc back"
	case screenConfirm:
		title = "confirm"
		body = components.ConfirmBox(m.confirmMsg, m.confirmYes, s)
		footer = "←→  y/n  enter"
	case screenChildOutput:
		title = "launchpad"
		body = m.viewChildOutput()
		footer = m.childOutputFooter()
	case screenResult:
		title = "result"
		body = m.viewResult()
		footer = "enter continue  esc home"
		if m.resultRestart {
			footer = "r restart  enter continue  esc home"
		}
	}

	if m.help {
		body = components.HelpOverlay(s)
	}

	header := components.Header(title, m.opts.Version, w, s)
	foot := components.Footer(footer, w, s)
	parts := []string{header}
	if m.banner != "" && (m.screen == screenHome || m.screen == screenLaunchpad) {
		parts = append(parts, s.FireTip.Render("◆ "+m.banner))
	}
	parts = append(parts, body)
	if m.err != nil && m.screen != screenResult {
		parts = append(parts, components.ErrorBox(m.err, s))
	}
	parts = append(parts, foot)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m model) viewHome() string {
	s := m.styles
	status, pathLine, extras := m.homeChannelCard()

	var flame string
	if m.reduced {
		flame = anim.StaticFlame(s)
	} else {
		flame = anim.Flame(m.frame, 1, s)
	}
	booths := anim.RenderBooths(anim.DefaultHomeBooths(homeBoothIndex(m.cursor)), m.animationFrame(), s)
	center := lipgloss.JoinVertical(lipgloss.Center, flame, "", booths)
	menu := components.Menu(m.homeItems(), m.cursor, s)
	tag := s.Tagline.Render(anim.Tagline)

	var b strings.Builder
	b.WriteString(status)
	b.WriteByte('\n')
	b.WriteString(pathLine)
	if extras != "" {
		b.WriteByte('\n')
		b.WriteString(extras)
	}
	b.WriteString("\n\n")
	b.WriteString(center)
	b.WriteString("\n\n")
	b.WriteString(menu)
	b.WriteByte('\n')
	b.WriteString(tag)
	return b.String()
}

func (m model) homeChannelCard() (status, pathLine, extras string) {
	s := m.styles
	switch m.status.Action {
	case "managed":
		ch := m.status.Channel
		if ch == "" {
			ch = "stable"
		}
		status = components.StatusLine(fmt.Sprintf("festival %s (%s) · managed", m.status.Version, ch), "ok", s)
		bin := shortenPath(m.status.ManagedBin)
		if m.status.ManagedBinOnPath {
			pathLine = components.StatusLine("PATH ok · "+bin, "ok", s)
		} else if m.status.ManagedBin != "" {
			pathLine = components.StatusLine("PATH missing · "+bin, "fail", s)
		} else {
			pathLine = components.StatusLine("managed bin not on PATH", "fail", s)
		}
		if m.updateAvailable() {
			return status, pathLine, s.FireTip.Render("   update available: " + m.status.Version + " → " + m.status.Latest)
		}
		return status, pathLine, ""
	case "package":
		return m.packageChannelCard()
	case "unmanaged":
		return m.leftoverChannelCard()
	case "absent":
		status = components.StatusLine("festival suite not installed", "fail", s)
		pathLine = components.StatusLine("install first", "", s)
		return status, pathLine, ""
	default:
		status = components.StatusLine("checking status…", "", s)
		pathLine = components.StatusLine("managed bin not on PATH", "", s)
		return status, pathLine, ""
	}
}

func (m model) packageChannelCard() (status, pathLine, extras string) {
	s := m.styles
	ver := m.status.Version
	if ver == "" {
		ver = m.opts.Version
	}
	status = components.StatusLine(fmt.Sprintf("festival %s · %s", ver, packageChannelLabel(m.status)), "ok", s)
	prefix := shortenPath(m.status.Prefix)
	if prefix == "" {
		prefix = shortenPath(m.status.ManagedBin)
	}

	var extra []string
	if len(m.status.Shadows) > 0 {
		pathLine = components.StatusLine("leftover binaries ahead of "+prefix, "warn", s)
		for _, loc := range m.status.Shadows {
			extra = append(extra, s.Muted.Render("   "+leftoverToolLine(loc)))
		}
		extra = append(extra, s.Muted.Render("   "+leftoverCleanupHint(m.status.Shadows)))
	} else {
		pathLine = components.StatusLine("suite on PATH · "+prefix, "ok", s)
		if h := helperCardLine(m.status.Helper); h != "" {
			extra = append(extra, s.Muted.Render("   "+h))
		}
		if m.updateAvailable() {
			extra = append(extra, s.FireTip.Render("   update available: "+m.status.Version+" → "+m.status.Latest))
		}
		if m.status.Upgrade != "" {
			extra = append(extra, s.Muted.Render("   upgrade: "+m.status.Upgrade))
		}
		extra = append(extra, s.FireTip.Render("   do not run festival install (that plants a second copy under ~/.obey/installer)"))
	}
	return status, pathLine, strings.Join(extra, "\n")
}

func (m model) leftoverChannelCard() (status, pathLine, extras string) {
	s := m.styles
	status = components.StatusLine("camp/fest found but not managed by festival", "warn", s)
	prefix := shortenPath(m.status.Prefix)
	if prefix == "" {
		prefix = "PATH"
	}
	pathLine = components.StatusLine("leftover binaries on PATH · "+prefix, "warn", s)
	var extra []string
	for _, loc := range m.status.Shadows {
		extra = append(extra, s.Muted.Render("   "+leftoverToolLine(loc)))
	}
	if len(m.status.Shadows) > 0 {
		extra = append(extra, s.Muted.Render("   festival uninstall cannot remove these files"))
	}
	return status, pathLine, strings.Join(extra, "\n")
}

func packageChannelLabel(sum app.StatusSummary) string {
	title := flavorTitle(sum.Flavor)
	pkg := sum.Package
	if pkg == "" {
		if _, rest, ok := strings.Cut(sum.Source, ":"); ok {
			pkg = rest
		}
	}
	if pkg != "" {
		return title + " (" + pkg + ")"
	}
	return title
}

func flavorTitle(f app.PackageFlavor) string {
	switch f {
	case app.FlavorAUR:
		return "AUR"
	case app.FlavorHomebrew:
		return "Homebrew"
	case app.FlavorNpm:
		return "npm"
	case app.FlavorDeb:
		return "deb"
	case app.FlavorRpm:
		return "rpm"
	case app.FlavorApk:
		return "apk"
	case app.FlavorInstallSh:
		return "install.sh"
	case app.FlavorUnknown:
		return "package"
	default:
		if f != "" {
			return string(f)
		}
		return "package"
	}
}

func helperCardLine(helper string) string {
	helper = strings.TrimSpace(helper)
	if helper == "" {
		return ""
	}
	if strings.HasPrefix(helper, "source ") {
		return "helper: " + helper
	}
	return "helper: source " + helper
}

func leftoverToolLine(loc app.ToolLocation) string {
	ver := strings.TrimSpace(loc.Version)
	if ver == "" {
		ver = "unknown"
	}
	bundle := "no bundle"
	if loc.Bundle != "" {
		bundle = loc.Bundle
	}
	path := loc.Path
	if path == "" {
		path = loc.Tool
	}
	return path + "  (" + ver + ", " + bundle + ")"
}

func leftoverCleanupHint(shadows []app.ToolLocation) string {
	dirs := map[string]struct{}{}
	for _, loc := range shadows {
		if loc.Path != "" {
			dirs[filepath.Dir(loc.Path)] = struct{}{}
		}
	}
	n := len(shadows)
	files := "those files"
	switch n {
	case 1:
		files = "that file"
	case 2:
		files = "those two files"
	}
	if len(dirs) == 1 {
		for d := range dirs {
			return "remove " + files + "; keep " + shortenPath(d) + " on PATH"
		}
	}
	return "remove " + files + "; keep their directory on PATH"
}

func shortenPath(p string) string {
	if len(p) > 48 {
		return "…" + p[len(p)-47:]
	}
	return p
}

func (m model) animationFrame() int {
	if m.reduced {
		return 0
	}
	return m.frame
}

func (m model) viewLaunchpad() string {
	s := m.styles
	if len(m.launchEntries) == 0 {
		return s.Muted.Render("no launchpad entries")
	}
	items := make([]string, len(m.launchEntries))
	for i, e := range m.launchEntries {
		items[i] = e.Label
		if e.Detail != "" {
			items[i] = e.Label + "  " + e.Detail
		}
	}
	intro := s.Title.Render("Open a camp / fest tool") + "\n" +
		s.Muted.Render("Runs the real binary. Quit that tool to return here. No need to relaunch festival.") + "\n\n"
	return intro + components.Menu(items, m.cursor, s)
}

func (m model) viewWorking() string {
	s := m.styles
	body := anim.ProgressFlame(m.progress.Percent, m.progress.Stage, m.progress.Message, m.animationFrame(), s)
	if m.warnText == "" {
		return body
	}
	return body + "\n\n" + s.Warn.Render(textsafe.Block(m.warnText))
}

func (m model) viewInstall() string {
	s := m.styles
	switch m.installKind {
	case "package":
		return m.viewInstallPackage()
	case "leftover":
		return m.viewInstallLeftover()
	default:
		var ch strings.Builder
		for i, c := range m.channels {
			if i == m.channelIdx {
				ch.WriteString(s.Selected.Render("▸ " + c + " "))
			} else {
				ch.WriteString(s.Muted.Render("  " + c + " "))
			}
		}
		title := "Install Festival suite (camp + fest)"
		if m.installForce {
			title = "Install a hub-managed copy anyway"
		}
		return s.Title.Render(title) + "\n\n" +
			s.Muted.Render("channel") + "\n" + ch.String() + "\n\n" +
			s.Fire.Render("enter") + s.Muted.Render(" to install")
	}
}

func (m model) viewInstallPackage() string {
	s := m.styles
	label := packageChannelLabel(m.status)
	prefix := m.status.Prefix
	if prefix == "" {
		prefix = "/usr/bin"
	}
	var b strings.Builder
	b.WriteString(s.Title.Render("Festival is already installed via " + label + " at " + prefix + "."))
	b.WriteString("\n\n")
	b.WriteString(s.Normal.Render("Installing again would put a second copy in ~/.obey/installer/bin"))
	b.WriteByte('\n')
	b.WriteString(s.Normal.Render("and can shadow or be shadowed."))
	b.WriteString("\n\n")
	if m.status.Upgrade != "" {
		b.WriteString(s.Muted.Render("upgrade: " + m.status.Upgrade))
		b.WriteString("\n\n")
	}
	b.WriteString(s.Muted.Render("esc / enter: back"))
	b.WriteByte('\n')
	b.WriteString(s.FireTip.Render("f: install a hub copy anyway (--force), then the channel picker"))
	return b.String()
}

func (m model) viewInstallLeftover() string {
	s := m.styles
	var b strings.Builder
	b.WriteString(s.Title.Render("leftover binaries on PATH (not a package or hub install)"))
	b.WriteString("\n\n")
	for _, loc := range m.status.Shadows {
		b.WriteString(s.Normal.Render("  " + leftoverToolLine(loc)))
		b.WriteByte('\n')
	}
	if len(m.status.Shadows) == 0 && m.status.Prefix != "" {
		b.WriteString(s.Normal.Render("  " + m.status.Prefix))
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	b.WriteString(s.Muted.Render("festival uninstall cannot remove these files."))
	b.WriteByte('\n')
	b.WriteString(s.Muted.Render("see https://docs.fest.build/getting-started/installation/"))
	b.WriteString("\n\n")
	b.WriteString(s.Fire.Render("enter") + s.Muted.Render(" to continue to the channel picker"))
	return b.String()
}

func (m model) viewList() string {
	s := m.styles
	if len(m.list.Packages) == 0 {
		return s.Muted.Render("no packages installed yet")
	}
	items := make([]string, len(m.list.Packages))
	for i, p := range m.list.Packages {
		paren := p.Channel
		if p.Origin == "package" || p.Origin == "leftover" {
			paren = p.Source
		}
		items[i] = fmt.Sprintf("%s  %s  (%s)", textsafe.Line(p.PackageID), textsafe.Line(p.Version), textsafe.Line(paren))
	}
	return components.Menu(items, m.cursor, s)
}

func (m model) viewBrowse() string {
	s := m.styles
	filt := s.Muted.Render(fmt.Sprintf("filter product=%q class=%q", m.productF, m.kindF))
	if len(m.browseFlat) == 0 {
		return filt + "\n\n" + s.Muted.Render("no packages match (add a marketplace or refresh)") + "\n" +
			s.Muted.Render("browse seeds the official marketplace (creates ~/.obey/installer)")
	}
	items := make([]string, len(m.browseFlat))
	for i, p := range m.browseFlat {
		items[i] = fmt.Sprintf("%s  [%s]  %s", textsafe.Line(p.ID), textsafe.Line(p.Class), textsafe.Line(p.Source))
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
		line := fmt.Sprintf("%s  %s  pkgs=%d", textsafe.Line(v.Name), short(textsafe.Line(v.Commit), 12), v.Packages)
		if v.Err != "" {
			line += "  ERR:" + textsafe.Line(v.Err)
		}
		items = append(items, line)
	}
	items = append(items, "↻ refresh all")
	return components.Menu(items, m.cursor, s) + "\n" +
		s.Muted.Render("a to add a git marketplace · s to seed official (creates installer home)")
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
		msg := c.ID + ": " + textsafe.Line(c.Message)
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
	body := textsafe.Block(m.resultBody)
	if m.resultOK {
		if !m.reduced {
			return title + "\n\n" + anim.Celebrate(m.frame, s) + "\n\n" + s.Normal.Render(body)
		}
		return title + "\n\n" + s.OK.Render("done") + "\n\n" + s.Normal.Render(body)
	}
	return title + "\n\n" + s.Err.Render(body)
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
	case 8: // launchpad: multi-activity energy
		return 0
	default:
		return 0
	}
}
