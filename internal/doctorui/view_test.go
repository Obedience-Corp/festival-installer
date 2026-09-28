package doctorui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	"github.com/Obedience-Corp/festival-installer/internal/tui/theme"
)

func TestRender_PlainBoard(t *testing.T) {
	got := Render([]app.DoctorCheck{
		{ID: "managed_bin_on_path", Status: app.DoctorFail, Message: "managed bin dir is not on PATH"},
		{ID: "sources_reachable", Status: app.DoctorWarn, Message: "no marketplaces registered"},
		{ID: "receipts_integrity", Status: app.DoctorOK, Message: "no receipts"},
		{ID: "path_shadowing", Status: app.DoctorPending, Message: "setup not finished"},
	}, Options{Width: 48, Heading: true})

	if strings.Contains(got, "\x1b") {
		t.Fatalf("plain render emitted an escape:\n%s", got)
	}
	for _, want := range []string{
		"▲ festival  ·  doctor",
		"● PATH",
		"fail",
		"  managed bin dir is not on PATH",
		"● sources",
		"warn",
		"○ shadowing",
		"pending",
		"1 fail   1 warn   1 pending   1 ok    one check failed",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q\n%s", want, got)
		}
	}
	if strings.Contains(got, "managed_bin_on_path") {
		t.Fatalf("the board still leads with the machine id:\n%s", got)
	}
	if strings.Contains(got, "CHECK") {
		t.Fatalf("the spreadsheet header is back:\n%s", got)
	}
}

func TestRender_PendingIsNotFail(t *testing.T) {
	got := Render([]app.DoctorCheck{
		{ID: "managed_bin_on_path", Status: app.DoctorPending, Message: "setup not finished"},
	}, Options{Width: 48})

	if !strings.Contains(got, "pending") {
		t.Fatalf("never says pending:\n%s", got)
	}
	if strings.Contains(got, "fail") {
		t.Fatalf("grades a pending check as fail:\n%s", got)
	}
	if !strings.Contains(got, "setup still pending") {
		t.Fatalf("missing the pending verdict:\n%s", got)
	}
}

func TestRender_StatusWords(t *testing.T) {
	tests := []struct {
		status string
		word   string
	}{
		{app.DoctorOK, "ok"},
		{app.DoctorWarn, "warn"},
		{app.DoctorFail, "fail"},
		{"something the hub has never heard of", "fail"},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			got := Render([]app.DoctorCheck{{ID: "check", Status: tt.status, Message: "message"}}, Options{Width: 48})
			if !strings.Contains(got, tt.word) {
				t.Fatalf("status %q rendered without %q:\n%s", tt.status, tt.word, got)
			}
		})
	}
}

func TestRender_WrapsUnderTheDetail(t *testing.T) {
	got := Render([]app.DoctorCheck{{
		ID:      "managed_bin_on_path",
		Status:  app.DoctorPending,
		Message: strings.Repeat("setup not finished yet ", 8),
	}}, Options{Width: 40})

	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	var detail []string
	for _, line := range lines {
		if strings.HasPrefix(line, "  ") {
			detail = append(detail, line)
		}
	}
	if len(detail) < 2 {
		t.Fatalf("expected the long message to wrap:\n%s", got)
	}
	for i, line := range detail {
		if !strings.HasPrefix(line, "  ") {
			t.Fatalf("detail line %d is not under the title: %q", i, line)
		}
	}
}

func TestRender_BreaksALongPathOnSlashes(t *testing.T) {
	path := "/Users/someone/.obey/installer/bin/festival-managed-directory"
	got := Render([]app.DoctorCheck{{
		ID:      "managed_bin_on_path",
		Status:  app.DoctorFail,
		Message: "not on PATH: " + path,
	}}, Options{Width: 40})

	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "  ") && lipgloss.Width(line) > 40 {
			t.Fatalf("detail wider than the board (%d): %q", lipgloss.Width(line), line)
		}
	}
	if strings.Contains(got, "festival-managed-direc") && !strings.Contains(got, "festival-managed-directory") {
		t.Fatalf("path broke inside a directory name:\n%s", got)
	}
	if !strings.Contains(got, "installer/") {
		t.Fatalf("path lost a directory boundary:\n%s", got)
	}
}

func TestRender_ColorHasNoPlainDisagreement(t *testing.T) {
	checks := []app.DoctorCheck{
		{ID: "sources_reachable", Status: app.DoctorOK, Message: "1 source reachable"},
		{ID: "marketplace_trust", Status: app.DoctorWarn, Message: "unsigned"},
	}
	plain := Render(checks, Options{Width: 60, Heading: true})
	colored := Render(checks, Options{Width: 60, Color: true, Heading: true})
	if !strings.Contains(colored, "\x1b") {
		t.Fatal("color render produced no escapes")
	}
	if stripANSI(colored) != plain {
		t.Fatalf("color and plain disagree\nplain:\n%s\ncolored:\n%s", plain, stripANSI(colored))
	}
}

func TestStatusColor_MatchesTheWord(t *testing.T) {
	s := theme.New()
	colour := func(status string) string {
		return fmt.Sprintf("%v", StatusColor(status, s))
	}
	want := func(c lipgloss.TerminalColor) string { return fmt.Sprintf("%v", c) }

	if colour(app.DoctorPending) == colour(app.DoctorFail) {
		t.Fatal("a pending check is painted in the failure colour")
	}
	tests := []struct {
		status string
		want   string
	}{
		{app.DoctorOK, want(s.OK.GetForeground())},
		{app.DoctorWarn, want(s.Warn.GetForeground())},
		{app.DoctorPending, want(s.Muted.GetForeground())},
		{app.DoctorFail, want(s.Err.GetForeground())},
		{"a status the hub has never heard of", want(s.Err.GetForeground())},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			if got := colour(tt.status); got != tt.want {
				t.Fatalf("colour = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestRender_Verdicts(t *testing.T) {
	tests := []struct {
		name   string
		checks []app.DoctorCheck
		want   string
	}{
		{"clean", []app.DoctorCheck{{ID: "receipts_integrity", Status: app.DoctorOK, Message: "no receipts"}}, "all checks passed"},
		{"warn", []app.DoctorCheck{{ID: "sources_reachable", Status: app.DoctorWarn, Message: "none"}}, "passed with warnings"},
		{"two fails", []app.DoctorCheck{
			{ID: "sources_reachable", Status: app.DoctorFail, Message: "down"},
			{ID: "marketplace_trust", Status: app.DoctorFail, Message: "bad"},
		}, "2 checks failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Render(tt.checks, Options{Width: 48})
			if !strings.Contains(got, tt.want) {
				t.Fatalf("missing %q\n%s", tt.want, got)
			}
		})
	}
}

func TestRender_Empty(t *testing.T) {
	got := Render(nil, Options{Width: 48})
	if !strings.Contains(got, "running checks…") {
		t.Fatalf("empty board: %q", got)
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != 0x1b {
			b.WriteByte(s[i])
			continue
		}
		// CSI: ESC [ ... final byte 0x40-0x7E
		if i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && (s[i] < 0x40 || s[i] > 0x7e) {
				i++
			}
			continue
		}
	}
	return b.String()
}
