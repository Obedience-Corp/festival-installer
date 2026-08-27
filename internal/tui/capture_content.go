package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// captureRender sanitizes raw child output for the hub pager. Cursor and
// screen-control sequences are dropped so they cannot move the hub chrome.
// SGR color is kept so fest list (and other styled CLI) still looks like a
// terminal. Carriage-return overwrites collapse to their final content, and
// lines truncate to the viewport's display width.
func captureRender(raw []byte, width int) string {
	if width < 1 {
		width = 1
	}
	s := keepSGR(string(raw))
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		if j := strings.LastIndexByte(ln, '\r'); j >= 0 {
			ln = ln[j+1:]
		}
		lines[i] = ansi.Truncate(ln, width, "")
	}
	return strings.Join(lines, "\n")
}

// keepSGR copies text and SGR color sequences, dropping other ANSI (cursor
// moves, erases, alt-screen, OSC) that would execute against the hub frame.
func keepSGR(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] != 0x1b {
			b.WriteByte(s[i])
			i++
			continue
		}
		if i+1 >= len(s) {
			break
		}
		switch s[i+1] {
		case '[': // CSI
			j := i + 2
			for j < len(s) {
				c := s[j]
				j++
				if c >= 0x40 && c <= 0x7E {
					if c == 'm' {
						b.WriteString(s[i:j])
					}
					break
				}
			}
			i = j
		case ']': // OSC: skip through BEL or ST
			j := i + 2
			for j < len(s) {
				if s[j] == 0x07 {
					j++
					break
				}
				if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
					j += 2
					break
				}
				j++
			}
			i = j
		default:
			i += 2
		}
	}
	return b.String()
}
