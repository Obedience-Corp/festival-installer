package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

// TestOpFailed_NeverPutsTheGitChainOnTheResultScreen is the regression guard for
// the surface a first-time install failure actually lands on. This screen is
// rendered by viewResult, which deliberately bypasses ErrorBox, so the friendly
// rendering has to happen where the message is built.
func TestOpFailed_NeverPutsTheGitChainOnTheResultScreen(t *testing.T) {
	raw := errpkg.New("E_GIT_CLONE",
		"git clone -- https://github.com/Obedience-Corp/marketplace.git: fatal: could not read Username for 'https://github.com'")
	seed := &app.MarketplaceSeedProblem{Err: errpkg.Wrap("E_MARKETPLACE_SEED", raw, "ensure official marketplace"), Fatal: true}

	tests := []struct {
		name      string
		err       error
		note      string
		wantIn    string
		wantNotIn []string
	}{
		{
			name:      "seed failure renders friendly",
			err:       seed,
			wantIn:    "couldn't reach the official marketplace",
			wantNotIn: []string{"E_GIT_CLONE", "could not read Username", "fatal:"},
		},
		{
			name:   "seed failure keeps a caller note",
			err:    seed,
			note:   "(selected obedience-corp/fest as fest)",
			wantIn: "(selected obedience-corp/fest as fest)",
		},
		{
			name:   "an ordinary coded error keeps its text",
			err:    errpkg.New("E_INSTALL_PACKAGE_CHANNEL", "refusing to plant over a package install"),
			wantIn: "refusing to plant over a package install",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := opFailed(nil, "Install failed", tt.err, tt.note)
			if msg.success {
				t.Fatal("a failure message must not be marked successful")
			}
			if msg.err != tt.err {
				t.Fatal("the original error must stay on the message for the caller")
			}
			if !strings.Contains(msg.body, tt.wantIn) {
				t.Fatalf("body missing %q:\n%s", tt.wantIn, msg.body)
			}
			for _, leak := range tt.wantNotIn {
				if strings.Contains(msg.body, leak) {
					t.Fatalf("body leaked %q:\n%s", leak, msg.body)
				}
			}
		})
	}
}

func TestViewResult_RendersTheFailureBody(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.reduced = true
	m.screen = screenResult
	m.resultTitle = "Install failed"
	m.resultBody = "couldn't reach the official marketplace; check your network"
	m.resultOK = false

	if got := m.viewResult(); !strings.Contains(got, "couldn't reach the official marketplace") {
		t.Fatalf("result screen dropped the body:\n%s", got)
	}
}

func TestViewResult_WrapsALongFailureBodyToTheTerminal(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.reduced = true
	m.screen = screenResult
	m.width = 60
	m.resultTitle = "Install failed"
	m.resultBody = "couldn't reach the official marketplace; check your network, " +
		"or add a marketplace with 'festival marketplace add <url>'"
	m.resultOK = false

	got := m.viewResult()
	for _, line := range strings.Split(got, "\n") {
		if lipgloss.Width(line) > m.width {
			t.Fatalf("line is %d columns wide, over %d:\n%q", lipgloss.Width(line), m.width, line)
		}
	}
	if !strings.Contains(strings.Join(strings.Fields(got), " "), "festival marketplace add <url>") {
		t.Fatalf("wrapping must not lose the end of the command:\n%s", got)
	}
}
