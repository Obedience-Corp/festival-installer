package textsafe

import (
	"strings"
	"testing"
	"unicode"
)

func TestLine(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "plain ascii unchanged",
			in:   "obedience-corp/festival",
			want: "obedience-corp/festival",
		},
		{
			name: "unicode text passes through unchanged",
			in:   "café — naïve Ω 日本語 🔥",
			want: "café — naïve Ω 日本語 🔥",
		},
		{
			name: "empty stays empty",
			in:   "",
			want: "",
		},
		{
			name: "osc title set with BEL terminator stripped",
			in:   "pkg\x1b]0;pwned\x07name",
			want: "pkg]0;pwnedname",
		},
		{
			name: "osc title set with ST terminator: ESC gone, inert backslash kept",
			in:   "pkg\x1b]2;pwned\x1b\\name",
			want: "pkg]2;pwned\\name",
		},
		{
			name: "csi color sequence stripped",
			in:   "\x1b[31mred\x1b[0m",
			want: "[31mred[0m",
		},
		{
			name: "csi cursor move stripped",
			in:   "line\x1b[2A\x1b[1Gclobber",
			want: "line[2A[1Gclobber",
		},
		{
			name: "dcs sequence: ESC gone, inert backslash kept",
			in:   "a\x1bPq#0;2;0;0;0\x1b\\b",
			want: "aPq#0;2;0;0;0\\b",
		},
		{
			name: "bare ESC stripped",
			in:   "before\x1bafter",
			want: "beforeafter",
		},
		{
			name: "lone BEL stripped",
			in:   "ding\x07dong",
			want: "dingdong",
		},
		{
			name: "raw C1 CSI byte (invalid utf-8) dropped",
			in:   "a\x9b31mb",
			want: "a31mb",
		},
		{
			name: "raw C1 OSC and DCS bytes dropped",
			in:   "a\x9d0;x\x9bmb\x90q",
			want: "a0;xmbq",
		},
		{
			name: "full raw C1 byte range dropped",
			in:   "x\x80\x85\x8f\x90\x9b\x9dy",
			want: "xy",
		},
		{
			name: "tab and newline removed to keep single line",
			in:   "col1\tcol2\ninjected\rrow",
			want: "col1col2injectedrow",
		},
		{
			name: "DEL stripped",
			in:   "a\x7fb",
			want: "ab",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Line(tt.in)
			if got != tt.want {
				t.Fatalf("Line(%q) = %q, want %q", tt.in, got, tt.want)
			}
			if i := strings.IndexFunc(got, unicode.IsControl); i >= 0 {
				t.Fatalf("Line(%q) = %q still contains a control character at %d", tt.in, got, i)
			}
		})
	}
}

func TestLineStripsProperlyEncodedC1Runes(t *testing.T) {
	// A marketplace manifest is JSON, so a C1 control arrives as a properly
	// UTF-8-encoded rune (for example from a backslash-u escape), not a raw byte.
	// Every such rune in 0x80-0x9f must be stripped by the IsControl path.
	for r := rune(0x80); r <= 0x9f; r++ {
		in := "a" + string(r) + "b"
		if got := Line(in); got != "ab" {
			t.Fatalf("Line(%q) = %q, want %q (C1 rune %U not stripped)", in, got, "ab", r)
		}
	}
}

func TestBlock(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "newlines preserved, indentation kept",
			in:   "installed acme/pkg 1.0\n  bin/acme\n  bin/acme-helper\n",
			want: "installed acme/pkg 1.0\n  bin/acme\n  bin/acme-helper\n",
		},
		{
			name: "escape inside a multi-line body stripped, lines intact",
			in:   "line1\n\x1b]0;pwn\x07line2\nline3",
			want: "line1\n]0;pwnline2\nline3",
		},
		{
			name: "carriage return and tab removed, newline stays",
			in:   "a\r\n\tb",
			want: "a\nb",
		},
		{
			name: "csi and C1 controls stripped across lines",
			in:   "\x1b[31merr\x9b0m\none\ntwo",
			want: "[31merr0m\none\ntwo",
		},
		{
			name: "unicode preserved",
			in:   "naïve 日本語\n🔥",
			want: "naïve 日本語\n🔥",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Block(tt.in)
			if got != tt.want {
				t.Fatalf("Block(%q) = %q, want %q", tt.in, got, tt.want)
			}
			for _, r := range got {
				if r != '\n' && unicode.IsControl(r) {
					t.Fatalf("Block(%q) = %q retained control %U", tt.in, got, r)
				}
			}
		})
	}
}

func TestLineCleanInputUnchangedReference(t *testing.T) {
	// A string with no control characters should be returned as-is without
	// reallocation, so callers pay nothing on the common clean path.
	const s = "obedience-corp/festival@v1.2.3"
	if got := Line(s); got != s {
		t.Fatalf("Line(%q) = %q, want identical", s, got)
	}
}
