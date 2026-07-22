package textsafe

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Line returns s with every control character removed, yielding a single line
// of printable text that is safe to render to a terminal. It strips the C0
// controls (including ESC 0x1b, BEL 0x07, tab, carriage return, and newline),
// DEL, and the C1 controls 0x80-0x9f (including the single-byte CSI, OSC, and
// DCS introducers). With those bytes gone an OSC, CSI, or DCS escape sequence
// in hostile input cannot survive; the remaining bytes render as literal text.
// Invalid UTF-8 bytes are dropped as well, so a raw C1 byte in a source label
// or git error cannot slip through as a decode artifact.
//
// Tab and newline are removed, not preserved: every caller renders s as a table
// cell or single-line menu item, where either would break column layout or
// inject additional rows.
func Line(s string) string {
	return strings.Map(func(r rune) rune {
		if r == utf8.RuneError || unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// Block behaves like Line but preserves newlines, so multi-line text keeps its
// layout. Every other control character is removed, including ESC, BEL,
// carriage return, tab, DEL, and the C1 controls, so no escape sequence can
// survive across the remaining line breaks. Use it for multi-line render
// boundaries such as error bodies, where the surrounding code owns the line
// structure but the content is untrusted.
func Block(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' {
			return r
		}
		if r == utf8.RuneError || unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}
