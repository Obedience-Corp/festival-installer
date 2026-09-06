package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/Obedience-Corp/festival-installer/internal/app"
)

// TestViewDoctor_GradesPendingAsPendingNotFail is the regression guard for the
// screen a first-run user opens from the home menu. The CLI table prints
// "pending" for a fresh home and exits 0; the TUI rendered the same check as a
// red "fail" because its badge switch had no pending case, so the two surfaces
// disagreed about whether the machine was broken.
func TestViewDoctor_GradesPendingAsPendingNotFail(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.reduced = true
	m.width = 100
	m.screen = screenDoctor
	m.checks = []app.DoctorCheck{
		{ID: "managed_bin_on_path", Status: app.DoctorPending, Message: "setup not finished"},
	}

	got := m.viewDoctor()
	if !strings.Contains(got, "pending") {
		t.Fatalf("doctor screen never says pending:\n%s", got)
	}
	if strings.Contains(got, "fail") {
		t.Fatalf("doctor screen grades a pending check as fail:\n%s", got)
	}
}

// TestDoctorBadgeStyle_ColoursMatchTheWords guards the half of the fix a word
// check cannot see. The label and the colour are two separate decisions, and
// "pending" painted in the error red still tells the reader their machine is
// broken.
func TestDoctorBadgeStyle_ColoursMatchTheWords(t *testing.T) {
	s := newModel(Options{Version: "test"}).styles
	colour := func(status string) string {
		return fmt.Sprintf("%v", doctorBadgeStyle(status, s).GetForeground())
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
				t.Fatalf("badge colour = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestViewDoctor_KeepsTheOtherStatusWords(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{app.DoctorOK, "ok"},
		{app.DoctorWarn, "warn"},
		{app.DoctorFail, "fail"},
		{"something the hub has never heard of", "fail"},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			m := newModel(Options{Version: "test"})
			m.reduced = true
			m.width = 100
			m.screen = screenDoctor
			m.checks = []app.DoctorCheck{{ID: "check", Status: tt.status, Message: "message"}}

			if got := m.viewDoctor(); !strings.Contains(got, "[· "+tt.want+"] ") {
				t.Fatalf("status %q rendered without a %q badge:\n%s", tt.status, tt.want, got)
			}
		})
	}
}

// TestViewDoctor_WrapContinuationLinesUpUnderTheMessage keeps the hanging indent
// tied to the badge width. A "pending" badge is wider than "warn", and a fixed
// nine-space indent left its wrapped lines a column short.
func TestViewDoctor_WrapContinuationLinesUpUnderTheMessage(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.reduced = true
	m.width = 60
	m.screen = screenDoctor
	m.checks = []app.DoctorCheck{{
		ID:      "managed_bin_on_path",
		Status:  app.DoctorPending,
		Message: strings.Repeat("setup not finished yet ", 6),
	}}

	lines := strings.Split(strings.TrimRight(m.viewDoctor(), "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected the long message to wrap:\n%s", strings.Join(lines, "\n"))
	}
	badge := "[· pending] "
	for i, line := range lines[1:] {
		if !strings.HasPrefix(line, strings.Repeat(" ", len(badge))) {
			t.Fatalf("wrapped line %d is not indented under the message: %q", i+1, line)
		}
	}
}
