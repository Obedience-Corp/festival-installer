// Package doctorui renders festival doctor for a person at a terminal.
// The same layout is the command output and the doctor screen inside the
// hub, so the two surfaces cannot disagree about a check's status word.
package doctorui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	"github.com/Obedience-Corp/festival-installer/internal/textsafe"
	"github.com/Obedience-Corp/festival-installer/internal/tui/theme"
)

const (
	defaultWidth = 76
	minWidth     = 40
	maxWidth     = 120
)

// Options controls one render. Width 0 selects the default. Color paints
// status words with the festival palette; plain output stays free of escapes
// so a pipe or NO_COLOR stays readable and stable.
type Options struct {
	Width   int
	Color   bool
	Heading bool
}

// Render formats checks as a status board: a named row, the status word at
// the right edge, and the detail wrapped underneath.
func Render(checks []app.DoctorCheck, opt Options) string {
	width := opt.Width
	if width <= 0 {
		width = defaultWidth
	}
	width = min(max(width, minWidth), maxWidth)

	r := newRenderer(opt.Color)
	var b strings.Builder
	if opt.Heading {
		b.WriteString(r.text("fire", "▲ festival"))
		b.WriteString(r.text("muted", "  ·  "))
		b.WriteString(r.text("title", "doctor"))
		b.WriteByte('\n')
		b.WriteString(r.text("rule", strings.Repeat("─", width)))
		b.WriteString("\n\n")
	}
	if len(checks) == 0 {
		b.WriteString(r.text("muted", "running checks…"))
		b.WriteByte('\n')
		return b.String()
	}

	counts := map[string]int{}
	for i, c := range checks {
		if i > 0 {
			b.WriteByte('\n')
		}
		status := badgeWord(c.Status)
		counts[status]++
		writeCheck(&b, r, width, humanTitle(c.ID), status, textsafe.Line(c.Message))
	}
	b.WriteByte('\n')
	b.WriteString(r.text("rule", strings.Repeat("─", width)))
	b.WriteByte('\n')
	b.WriteString(r.summary(counts))
	b.WriteByte('\n')
	return b.String()
}

// leader fills the gap between a check's name and its status word. One column
// is a single space; anything wider is a space, a muted dot run, and a space,
// so the status words share a right edge without a blank field between them.
func leader(r renderer, gap int) string {
	if gap < 2 {
		return " "
	}
	return " " + r.text("muted", strings.Repeat("·", gap-2)) + " "
}

func badgeWord(status string) string {
	switch status {
	case app.DoctorOK, app.DoctorWarn, app.DoctorPending:
		return status
	default:
		return app.DoctorFail
	}
}

// StatusColor is the palette colour for a status word. Pending is muted, not
// the failure red: the colour is what a reader sees first, and an unfinished
// setup is not a broken machine.
func StatusColor(status string, s theme.Styles) lipgloss.TerminalColor {
	switch badgeWord(status) {
	case app.DoctorOK:
		return s.OK.GetForeground()
	case app.DoctorWarn:
		return s.Warn.GetForeground()
	case app.DoctorPending:
		return s.Muted.GetForeground()
	default:
		return s.Err.GetForeground()
	}
}

func humanTitle(id string) string {
	switch id {
	case "managed_bin_on_path":
		return "PATH"
	case "sources_reachable":
		return "sources"
	case "marketplace_trust":
		return "trust"
	case "receipts_integrity":
		return "receipts"
	case "path_shadowing":
		return "shadowing"
	default:
		return strings.ReplaceAll(id, "_", " ")
	}
}

func writeCheck(b *strings.Builder, r renderer, width int, title, status, message string) {
	mark := "●"
	if status == app.DoctorPending {
		mark = "○"
	}
	left := mark + " " + title
	gap := width - lipgloss.Width(left) - lipgloss.Width(status)
	if gap < 1 {
		gap = 1
	}
	b.WriteString(r.text(status, mark+" "))
	b.WriteString(r.text("name", title))
	b.WriteString(leader(r, gap))
	b.WriteString(r.text(status, status))
	b.WriteByte('\n')
	for _, line := range wrapWords(message, width-2) {
		b.WriteString("  ")
		b.WriteString(r.text("muted", line))
		b.WriteByte('\n')
	}
}

func (r renderer) summary(counts map[string]int) string {
	order := []string{app.DoctorFail, app.DoctorWarn, app.DoctorPending, app.DoctorOK}
	var parts []string
	for _, status := range order {
		n := counts[status]
		if n == 0 {
			continue
		}
		parts = append(parts, r.text(status, fmt.Sprintf("%d %s", n, status)))
	}
	verdict, kind := verdict(counts)
	return strings.Join(parts, r.text("muted", "   ")) + r.text("muted", "    ") + r.text(kind, verdict)
}

func verdict(counts map[string]int) (string, string) {
	switch {
	case counts[app.DoctorFail] == 1:
		return "one check failed", app.DoctorFail
	case counts[app.DoctorFail] > 1:
		return fmt.Sprintf("%d checks failed", counts[app.DoctorFail]), app.DoctorFail
	case counts[app.DoctorPending] > 0:
		return "setup still pending", app.DoctorPending
	case counts[app.DoctorWarn] > 0:
		return "passed with warnings", app.DoctorWarn
	default:
		return "all checks passed", app.DoctorOK
	}
}

type renderer struct {
	color bool
	lip   *lipgloss.Renderer
}

// newRenderer paints with an explicit truecolor profile when color is on.
// The default lipgloss renderer follows the process stdout, which is Ascii in
// tests and in a redirected pipe, so a Color flag would otherwise be a no-op.
func newRenderer(color bool) renderer {
	if !color {
		return renderer{}
	}
	lip := lipgloss.NewRenderer(io.Discard)
	lip.SetColorProfile(termenv.TrueColor)
	lip.SetHasDarkBackground(true)
	return renderer{color: true, lip: lip}
}

func (r renderer) text(kind, s string) string {
	if !r.color || s == "" {
		return s
	}
	return r.style(kind).Render(s)
}

func (r renderer) style(kind string) lipgloss.Style {
	s := r.lip.NewStyle()
	switch kind {
	case "fire":
		return s.Foreground(lipgloss.Color(theme.Fire)).Bold(true)
	case "title", "name":
		return s.Foreground(lipgloss.Color(theme.FG)).Bold(true)
	case "muted", app.DoctorPending:
		return s.Foreground(lipgloss.Color(theme.Muted))
	case "rule":
		return s.Foreground(lipgloss.Color(theme.FireEmber))
	case app.DoctorOK:
		return s.Foreground(lipgloss.Color(theme.OK))
	case app.DoctorWarn:
		return s.Foreground(lipgloss.Color(theme.Warn))
	case app.DoctorFail:
		return s.Foreground(lipgloss.Color(theme.Err))
	default:
		return s.Foreground(lipgloss.Color(theme.FG))
	}
}

func wrapWords(text string, width int) []string {
	if text == "" {
		return nil
	}
	if width < 8 {
		width = 8
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	var cur []rune
	flush := func() {
		if len(cur) == 0 {
			return
		}
		lines = append(lines, string(cur))
		cur = cur[:0]
	}
	appendWord := func(word []rune) {
		if len(cur) == 0 {
			cur = append(cur, word...)
			return
		}
		if len(cur)+1+len(word) > width {
			flush()
			cur = append(cur, word...)
			return
		}
		cur = append(cur, ' ')
		cur = append(cur, word...)
	}
	for _, word := range words {
		for _, piece := range splitToken(word, width) {
			appendWord([]rune(piece))
		}
	}
	flush()
	return lines
}

// splitToken breaks a token that is wider than the line. Paths break after a
// slash so a directory stays intact; anything else breaks on the column.
func splitToken(word string, width int) []string {
	if len([]rune(word)) <= width {
		return []string{word}
	}
	if strings.Contains(word, "/") {
		var segs []string
		var cur strings.Builder
		parts := strings.Split(word, "/")
		for i, part := range parts {
			piece := part
			if i < len(parts)-1 {
				piece += "/"
			}
			if cur.Len() > 0 && cur.Len()+len(piece) > width {
				segs = append(segs, cur.String())
				cur.Reset()
			}
			cur.WriteString(piece)
			if cur.Len() > width {
				segs = append(segs, hardSplit(cur.String(), width)...)
				cur.Reset()
			}
		}
		if cur.Len() > 0 {
			segs = append(segs, cur.String())
		}
		if len(segs) > 0 {
			return segs
		}
	}
	return hardSplit(word, width)
}

func hardSplit(word string, width int) []string {
	runes := []rune(word)
	var out []string
	for len(runes) > width {
		out = append(out, string(runes[:width]))
		runes = runes[width:]
	}
	if len(runes) > 0 {
		out = append(out, string(runes))
	}
	return out
}
