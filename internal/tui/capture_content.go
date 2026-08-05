package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// captureRender sanitizes raw child output for the hub pager: ANSI control
// sequences are stripped (the viewport is not a terminal, so embedded escapes
// would execute against the hub chrome), carriage-return overwrites collapse
// to their final content, and lines truncate to the viewport width so a wide
// child line can never wrap the frame taller than the alt screen.
func captureRender(raw []byte, width int) string {
	if width < 1 {
		width = 1
	}
	s := ansi.Strip(string(raw))
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		if j := strings.LastIndexByte(ln, '\r'); j >= 0 {
			ln = ln[j+1:]
		}
		lines[i] = ansi.Truncate(ln, width, "")
	}
	return strings.Join(lines, "\n")
}
