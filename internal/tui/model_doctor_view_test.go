package tui

import (
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/app"
)

// TestViewDoctor_GradesPendingAsPendingNotFail is the regression guard for the
// screen a first-run user opens from the home menu. A fresh home is graded
// pending and the command exits 0; painting that check as a red "fail" told
// the user the opposite of what "festival doctor" says.
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
	if strings.Contains(got, "managed_bin_on_path") {
		t.Fatalf("doctor screen still leads with the machine id:\n%s", got)
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

			if got := m.viewDoctor(); !strings.Contains(got, tt.want) {
				t.Fatalf("status %q rendered without %q:\n%s", tt.status, tt.want, got)
			}
		})
	}
}

func TestViewDoctor_PutsTheDetailOnItsOwnLine(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.reduced = true
	m.width = 60
	m.screen = screenDoctor
	m.checks = []app.DoctorCheck{{
		ID:      "managed_bin_on_path",
		Status:  app.DoctorPending,
		Message: "setup not finished yet and the rest of the sentence keeps going",
	}}

	plain := stripSGR(m.viewDoctor())
	if !strings.Contains(plain, "\n  setup not finished") {
		t.Fatalf("detail is not on its own indented line:\n%s", plain)
	}
}

func stripSGR(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != 0x1b {
			b.WriteByte(s[i])
			continue
		}
		if i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && (s[i] < 0x40 || s[i] > 0x7e) {
				i++
			}
		}
	}
	return b.String()
}
