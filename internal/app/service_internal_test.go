package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

// obeyDrainBudget is obey's own worst case for a restart: a bounded 10s SIGTERM
// drain, a graceful gRPC stop, then a wait for the replacement process to
// answer. The installer's budget has to clear it, or a restart that worked is
// killed halfway and reported as a failure.
const obeyDrainBudget = 60 * time.Second

func TestVerbTimeout_RestartClearsObeyOwnBudget(t *testing.T) {
	if got := verbTimeout(serviceVerbRestart); got < obeyDrainBudget {
		t.Fatalf("restart budget = %s, want at least %s", got, obeyDrainBudget)
	}
	for _, verb := range []string{serviceVerbInstall, serviceVerbUninstall, serviceVerbStatus} {
		got := verbTimeout(verb)
		if got != serviceTimeout {
			t.Fatalf("%s budget = %s, want the supervisor budget %s", verb, got, serviceTimeout)
		}
		if got >= verbTimeout(serviceVerbRestart) {
			t.Fatalf("%s budget = %s, want it shorter than the restart budget", verb, got)
		}
	}
}

// A verb the installer cut short says so. The old wording claimed the daemon
// would not start automatically, which is false: the supervisor is still
// working on it, and the user was being pointed at a problem that is not there.
func TestRunServiceVerb_TimeoutSaysTheVerbTimedOut(t *testing.T) {
	dir := t.TempDir()
	slow := filepath.Join(dir, "obey")
	if err := os.WriteFile(slow, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatalf("write fake obey: %v", err)
	}

	started := time.Now()
	err := runServiceVerbWithin(context.Background(), slow, serviceVerbRestart, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected the verb to be cut short")
	}
	// The fake sleeps far past its budget and holds the output pipe while it
	// does. A budget that only kills the process would still block here.
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("the verb took %s to give up on a 100ms budget", elapsed)
	}
	if code := errpkg.Code(err); code != codeServiceTimeout {
		t.Fatalf("code = %q, want %q", code, codeServiceTimeout)
	}
	if !strings.Contains(err.Error(), "obey service restart timed out") {
		t.Fatalf("error = %q, want it to name the verb that timed out", err)
	}

	note := serviceNote(&ServiceResult{Error: err.Error(), TimedOut: true}, "0.2.1", true)
	if !strings.Contains(note, "timed out") {
		t.Fatalf("note = %q, want it to say the verb timed out", note)
	}
	if strings.Contains(note, "will not start automatically") {
		t.Fatalf("note = %q, must not claim the daemon is not coming up", note)
	}
	if !strings.Contains(note, "obey service status") {
		t.Fatalf("note = %q, want it to point at the status check", note)
	}
}

func TestServiceNote_DeferredErrorAndQuiet(t *testing.T) {
	cases := []struct {
		name        string
		svc         *ServiceResult
		version     string
		want        string
		wantContain []string
	}{
		{
			name: "nil result says nothing",
			svc:  nil,
			want: "",
		},
		{
			name:        "deferred names the restart command",
			svc:         &ServiceResult{Deferred: true},
			version:     "0.2.1",
			wantContain: []string{"obey 0.2.1 is installed", "obey service restart"},
		},
		{
			name:        "error carries the failure and the reassurance",
			svc:         &ServiceResult{Error: "E_SERVICE_VERB: obey service install: Operation not permitted"},
			version:     "0.2.0",
			wantContain: []string{"Operation not permitted", "The binaries are installed"},
		},
		{
			name:    "clean result says nothing",
			svc:     &ServiceResult{Installed: true},
			version: "0.2.0",
			want:    "",
		},
		{
			name:        "started says the daemon came up rather than restarted",
			svc:         &ServiceResult{Installed: true, Started: true},
			version:     "0.2.1",
			wantContain: []string{"obey 0.2.1 is installed", "started on the new version"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := serviceNote(tc.svc, tc.version, true)
			if len(tc.wantContain) == 0 {
				if got != tc.want {
					t.Fatalf("serviceNote = %q, want %q", got, tc.want)
				}
				return
			}
			for _, want := range tc.wantContain {
				if !strings.Contains(got, want) {
					t.Fatalf("serviceNote = %q, want it to contain %q", got, want)
				}
			}
		})
	}
}

func TestAppendWarning_JoinsWithSemicolon(t *testing.T) {
	cases := []struct {
		existing string
		add      string
		want     string
	}{
		{existing: "", add: "", want: ""},
		{existing: "a", add: "", want: "a"},
		{existing: "", add: "b", want: "b"},
		{existing: "a", add: "b", want: "a; b"},
	}
	for _, tc := range cases {
		if got := appendWarning(tc.existing, tc.add); got != tc.want {
			t.Fatalf("appendWarning(%q, %q) = %q, want %q", tc.existing, tc.add, got, tc.want)
		}
	}
}

func TestServiceNoteFor_ReadsTheInstallResult(t *testing.T) {
	if got := ServiceNoteFor(InstallResult{Version: "0.2.0"}); got != "" {
		t.Fatalf("an install with no service step must be quiet, got %q", got)
	}
	res := InstallResult{Version: "0.2.0", Service: &ServiceResult{Error: "boom"}}
	if got := ServiceNoteFor(res); !strings.Contains(got, "boom") {
		t.Fatalf("ServiceNoteFor = %q, want it to carry the service error", got)
	}
}
