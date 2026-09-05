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
