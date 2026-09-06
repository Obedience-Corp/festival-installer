package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/source"
)

func TestInstallFestivalBootstrapsOfficialSourceOnFreshHome(t *testing.T) {
	t.Setenv("FESTIVAL_HOME", t.TempDir())
	t.Setenv("OBEY_INSTALLER_HOME", "")
	t.Setenv("PATH", t.TempDir())

	want := errors.New("seed unavailable")
	called := false
	previous := ensureOfficialSeed
	ensureOfficialSeed = func(context.Context, source.VerifyOptions) error {
		called = true
		return want
	}
	t.Cleanup(func() { ensureOfficialSeed = previous })

	_, err := InstallFestival(context.Background(), InstallOptions{})
	if !called {
		t.Fatal("InstallFestival did not bootstrap the official source")
	}
	if !errors.Is(err, want) {
		t.Fatalf("expected seed error, got %v", err)
	}
}

// TestInstallFestivalHidesTheGitTraceOnSeedFailure pins the user-facing half of
// a failed seed: install still fails, still carries E_MARKETPLACE_SEED for
// machines, and the text a person reads never contains the git chain.
func TestInstallFestivalHidesTheGitTraceOnSeedFailure(t *testing.T) {
	t.Setenv("FESTIVAL_HOME", t.TempDir())
	t.Setenv("OBEY_INSTALLER_HOME", "")
	t.Setenv("PATH", t.TempDir())

	raw := errors.New("E_GIT_CLONE: clone https://github.com/Obedience-Corp/marketplace.git: " +
		"E_GIT_EXEC: git clone -- : fatal: could not read Username for 'https://github.com'")
	previous := ensureOfficialSeed
	ensureOfficialSeed = func(context.Context, source.VerifyOptions) error { return raw }
	t.Cleanup(func() { ensureOfficialSeed = previous })

	_, err := InstallFestival(context.Background(), InstallOptions{})

	var problem *MarketplaceSeedProblem
	if !errors.As(err, &problem) {
		t.Fatalf("expected a MarketplaceSeedProblem, got %v", err)
	}
	if !problem.Fatal {
		t.Fatal("a failed seed on the install path is fatal, not a warning")
	}
	if got := problem.Friendly(); got != marketplaceSeedFatalMessage {
		t.Fatalf("Friendly()=%q, want the fatal seed message", got)
	}
	for _, leak := range []string{"E_GIT_CLONE", "E_GIT_EXEC", "could not read Username", "fatal:"} {
		if strings.Contains(problem.Friendly(), leak) {
			t.Fatalf("Friendly() leaked %q: %s", leak, problem.Friendly())
		}
	}
	if code := errpkg.Code(err); code != "E_MARKETPLACE_SEED" {
		t.Fatalf("machine code = %q, want E_MARKETPLACE_SEED", code)
	}
	if !errors.Is(err, raw) {
		t.Fatal("the underlying seed error must stay in the chain for debugging")
	}
}

func TestMarketplaceSeedProblem_NonFatalKeepsTheReadPathWording(t *testing.T) {
	problem := &MarketplaceSeedProblem{Err: errors.New("offline")}
	if got := problem.Friendly(); got != marketplaceSeedFriendlyMessage {
		t.Fatalf("Friendly()=%q, want the read-path message", got)
	}
}

func TestMarketplaceListSurfacesSeedWarning(t *testing.T) {
	t.Setenv("FESTIVAL_HOME", t.TempDir())
	t.Setenv("OBEY_INSTALLER_HOME", "")

	want := errors.New("offline")
	previous := ensureOfficialSeed
	ensureOfficialSeed = func(context.Context, source.VerifyOptions) error { return want }
	t.Cleanup(func() { ensureOfficialSeed = previous })

	views, err := MarketplaceList(context.Background(), source.DefaultVerifyOptions(nil, false))
	if len(views) != 0 {
		t.Fatalf("expected no sources, got %+v", views)
	}
	var warning *MarketplaceSeedProblem
	if !errors.As(err, &warning) {
		t.Fatalf("expected MarketplaceSeedProblem, got %v", err)
	}
	if !errors.Is(err, want) {
		t.Fatalf("warning lost seed error: %v", err)
	}
}

func TestFriendlyMessage(t *testing.T) {
	raw := errors.New("E_GIT_CLONE: fatal: could not read Username for 'https://github.com'")
	tests := []struct {
		name      string
		err       error
		want      string
		wantEmpty bool
	}{
		{name: "nil", err: nil, wantEmpty: true},
		{name: "plain error keeps its text", err: errors.New("disk full"), want: "disk full"},
		{
			name: "fatal seed problem renders the directive line",
			err:  &MarketplaceSeedProblem{Err: raw, Fatal: true},
			want: marketplaceSeedFatalMessage,
		},
		{
			name: "read-path seed problem renders the softer line",
			err:  &MarketplaceSeedProblem{Err: raw},
			want: marketplaceSeedFriendlyMessage,
		},
		{
			name: "a wrapped seed problem is still found",
			err:  errpkg.Wrap("E_INSTALL", &MarketplaceSeedProblem{Err: raw, Fatal: true}, "install festival"),
			want: marketplaceSeedFatalMessage,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FriendlyMessage(tt.err)
			if tt.wantEmpty {
				if got != "" {
					t.Fatalf("want empty, got %q", got)
				}
				return
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
			if strings.Contains(got, "could not read Username") {
				t.Fatalf("leaked the git chain: %q", got)
			}
		})
	}
}
