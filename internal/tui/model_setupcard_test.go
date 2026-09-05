package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/Obedience-Corp/festival-installer/internal/app"
)

func homeModelWithSetup(setup app.SetupState, action string) model {
	m := newModel(Options{Version: "test"})
	m.reduced = true
	m.screen = screenHome
	m.status.Action = action
	m.status.Setup = setup
	m.status.ManagedBin = setup.ManagedBin
	m.status.ManagedBinOnPath = setup.ManagedBinOnPath
	return m
}

func TestViewHome_SetupCardVisibility(t *testing.T) {
	tests := []struct {
		name     string
		setup    app.SetupState
		action   string
		wantCard bool
	}{
		{
			name:     "fresh home shows the card",
			setup:    app.SetupState{ManagedBin: "/home/u/.obey/installer/bin"},
			action:   "absent",
			wantCard: true,
		},
		{
			name:     "installed but PATH not wired still shows the card",
			setup:    app.SetupState{HasReceipts: true, HasMarketplaces: true, ManagedBin: "/home/u/.obey/installer/bin"},
			action:   "managed",
			wantCard: true,
		},
		{
			name: "fully set up hides the card",
			setup: app.SetupState{
				HasReceipts:      true,
				HasMarketplaces:  true,
				ManagedBinOnPath: true,
				ManagedBin:       "/home/u/.obey/installer/bin",
			},
			action:   "managed",
			wantCard: false,
		},
		{
			name:     "package install hides the card",
			setup:    app.SetupState{ManagedBin: "/home/u/.obey/installer/bin"},
			action:   "package",
			wantCard: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := homeModelWithSetup(tt.setup, tt.action).viewHome()
			if strings.Contains(got, "Setup") != tt.wantCard {
				t.Fatalf("card present = %v, want %v:\n%s", !tt.wantCard, tt.wantCard, got)
			}
		})
	}
}

func TestViewHome_SetupCardMarksFinishedSteps(t *testing.T) {
	setup := app.SetupState{HasReceipts: true, ManagedBin: "/home/u/.obey/installer/bin"}
	got := homeModelWithSetup(setup, "managed").viewHome()

	for _, want := range []string{"Install the suite", "Put the managed bin on PATH", "Browse the catalog"} {
		if !strings.Contains(got, want) {
			t.Fatalf("card missing step %q:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "✓ 1. Install the suite") {
		t.Fatalf("a finished step must be marked done:\n%s", got)
	}
	if !strings.Contains(got, "· 2. Put the managed bin on PATH") {
		t.Fatalf("an unfinished step must be marked pending:\n%s", got)
	}
}

// TestViewHome_SetupCardFitsEightyColumns keeps the card inside the width the
// VHS tapes record at. Every line of the rendered home is checked, not only the
// card, because a card that pushes the home past 80 columns is the same bug.
func TestViewHome_SetupCardFitsEightyColumns(t *testing.T) {
	setup := app.SetupState{ManagedBin: "/home/u/.obey/installer/bin"}
	for _, line := range strings.Split(homeModelWithSetup(setup, "absent").viewHome(), "\n") {
		if width := lipgloss.Width(line); width > 80 {
			t.Fatalf("home line is %d columns wide, over 80:\n%q", width, line)
		}
	}
}

// TestHome_UnknownSignalsShowADiagnosticNotTheChecklist is the regression test
// for a home screen that offered "install the suite" to a user whose installed
// home had simply become unreadable, turning an incident into onboarding.
func TestHome_UnknownSignalsShowADiagnosticNotTheChecklist(t *testing.T) {
	tests := []struct {
		name         string
		setup        app.SetupState
		action       string
		wantCard     bool
		wantNotice   bool
		wantMenuRows int
	}{
		{
			name:         "an empty home gets the checklist",
			setup:        app.SetupState{},
			action:       "absent",
			wantCard:     true,
			wantMenuRows: 11,
		},
		{
			name:         "unreadable signals get one diagnostic line",
			setup:        app.SetupState{SignalsIncomplete: true},
			action:       "absent",
			wantNotice:   true,
			wantMenuRows: 11,
		},
		{
			name:         "unreadable signals on a home that did read as installed",
			setup:        app.SetupState{SignalsIncomplete: true, HasReceipts: true},
			action:       "managed",
			wantNotice:   true,
			wantMenuRows: 11,
		},
		{
			name:         "a package install gets neither",
			setup:        app.SetupState{SignalsIncomplete: true},
			action:       "package",
			wantMenuRows: 11,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := homeModelWithSetup(tc.setup, tc.action)
			m.width, m.height = 80, 40
			out := m.View()

			hasCard := strings.Contains(out, "Install the suite")
			if hasCard != tc.wantCard {
				t.Fatalf("setup card present = %v, want %v:\n%s", hasCard, tc.wantCard, out)
			}
			hasNotice := strings.Contains(out, "could not read the installer home")
			if hasNotice != tc.wantNotice {
				t.Fatalf("diagnostic present = %v, want %v:\n%s", hasNotice, tc.wantNotice, out)
			}
			if tc.wantNotice {
				if strings.Contains(out, "Put the managed bin on PATH") {
					t.Fatal("the checklist must be gone entirely, not just its first row")
				}
				if !strings.Contains(out, "festival doctor") {
					t.Fatal("the diagnostic must name the command that explains the incident")
				}
			}

			// The menu rows sequence 03 records must not move.
			rows := 0
			for _, item := range m.homeItems() {
				if strings.Contains(out, item) {
					rows++
				}
			}
			if rows != tc.wantMenuRows {
				t.Fatalf("menu rows visible = %d, want %d", rows, tc.wantMenuRows)
			}
		})
	}
}

// TestHome_TheDiagnosticIsOneLine keeps the notice from growing into a second
// card and pushing the menu down.
func TestHome_TheDiagnosticIsOneLine(t *testing.T) {
	m := homeModelWithSetup(app.SetupState{SignalsIncomplete: true}, "absent")
	m.width, m.height = 80, 40

	notice := m.setupNotice()
	if notice == "" {
		t.Fatal("expected a diagnostic")
	}
	if strings.Contains(notice, "\n") {
		t.Fatalf("the diagnostic must be one line, got:\n%s", notice)
	}
	if got := lipgloss.Width(notice); got > 80 {
		t.Fatalf("the diagnostic is %d columns wide, over 80", got)
	}
	if m.setupCard() != "" {
		t.Fatal("the card and the diagnostic must never both render")
	}
}
