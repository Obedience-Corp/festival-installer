package components

import (
	"errors"
	"strings"
	"testing"

	"github.com/Obedience-Corp/obey-installer/internal/tui/theme"
)

type stubFriendlyError struct {
	detail   string
	friendly string
}

func (e *stubFriendlyError) Error() string    { return e.detail }
func (e *stubFriendlyError) Friendly() string { return e.friendly }

func TestErrorBox_PrefersFriendlyOverError(t *testing.T) {
	s := theme.New()
	err := &stubFriendlyError{
		detail:   "E_GIT_CLONE: git clone -- https://example.com: fatal: could not read Username",
		friendly: "couldn't reach the official marketplace; showing local sources only",
	}
	out := ErrorBox(err, s)
	if !strings.Contains(out, err.friendly) {
		t.Fatalf("expected friendly message rendered, got: %s", out)
	}
	if strings.Contains(out, "git clone") || strings.Contains(out, "fatal:") {
		t.Fatalf("expected raw error detail to be suppressed, got: %s", out)
	}
}

func TestErrorBox_FallsBackToErrorForPlainErrors(t *testing.T) {
	s := theme.New()
	err := errors.New("no launchpad entries")
	out := ErrorBox(err, s)
	if !strings.Contains(out, "no launchpad entries") {
		t.Fatalf("expected plain error text rendered, got: %s", out)
	}
}

func TestErrorBox_NilIsEmpty(t *testing.T) {
	if got := ErrorBox(nil, theme.New()); got != "" {
		t.Fatalf("expected empty string for nil error, got %q", got)
	}
}
