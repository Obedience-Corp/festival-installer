package app

import (
	"context"
	"strings"
	"testing"
)

// The service step install and update share, driven against a staged obey for
// every answer the daemon probe can give. The install and update columns are
// the same result read two ways: the payload is identical, and only the
// started sentence is an update's.
func TestObeyServiceSwap_EveryDaemonStateOnInstallAndUpdate(t *testing.T) {
	cases := []struct {
		name       string
		supervised bool
		daemon     daemonState
		noRestart  bool
		wantVerbs  []string
		want       ServiceResult
		installSay []string
		updateSay  []string
	}{
		{
			name:       "stopped daemon is started by the install verb",
			supervised: true,
			daemon:     daemonStateStopped,
			wantVerbs:  []string{"install"},
			want:       ServiceResult{Installed: true, Started: true},
			updateSay:  []string{"started on the new version"},
		},
		{
			name:       "stopped daemon with --no-restart has nothing to defer",
			supervised: true,
			daemon:     daemonStateStopped,
			noRestart:  true,
			wantVerbs:  []string{"install"},
			want:       ServiceResult{Installed: true, Started: true},
			updateSay:  []string{"started on the new version"},
		},
		{
			name:       "running daemon is restarted onto the staged binary",
			supervised: true,
			daemon:     daemonStateRunning,
			wantVerbs:  []string{"install", "restart"},
			want:       ServiceResult{Installed: true, Restarted: true},
		},
		{
			name:       "running daemon with --no-restart defers and says so",
			supervised: true,
			daemon:     daemonStateRunning,
			noRestart:  true,
			wantVerbs:  []string{"install"},
			want:       ServiceResult{Installed: true, Deferred: true},
			installSay: []string{"the running daemon is still the previous version", "obey service restart"},
			updateSay:  []string{"the running daemon is still the previous version", "obey service restart"},
		},
		{
			name:       "unknown daemon is restarted rather than assumed stopped",
			supervised: true,
			daemon:     daemonStateUnknown,
			wantVerbs:  []string{"install", "restart"},
			want:       ServiceResult{Installed: true, Restarted: true, DaemonStateUnknown: true},
		},
		{
			name:       "unknown daemon with --no-restart defers on an unknown, not a no",
			supervised: true,
			daemon:     daemonStateUnknown,
			noRestart:  true,
			wantVerbs:  []string{"install"},
			want:       ServiceResult{Installed: true, Deferred: true, DaemonStateUnknown: true},
			installSay: []string{"could not be determined", "obey service status"},
			updateSay:  []string{"could not be determined", "obey service status"},
		},
		{
			name:      "an obey without the contract gets no verb and no claim",
			daemon:    daemonStateRunning,
			wantVerbs: nil,
			want: ServiceResult{
				Unsupported:    true,
				ContractReason: "it has no obey service restart",
			},
			installSay: []string{"predates the supervised daemon service", "obey service install"},
			updateSay:  []string{"predates the supervised daemon service", "obey service install"},
		},
		{
			name:      "an obey without the contract is not deferred either",
			daemon:    daemonStateRunning,
			noRestart: true,
			wantVerbs: nil,
			want: ServiceResult{
				Unsupported:    true,
				ContractReason: "it has no obey service restart",
			},
			installSay: []string{"predates the supervised daemon service"},
			updateSay:  []string{"predates the supervised daemon service"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := writeFakeObey(t, tc.supervised, "0.2.1")

			got := obeyServiceSwap(context.Background(), tc.daemon, tc.noRestart)

			if got != tc.want {
				t.Fatalf("service result = %+v, want %+v", got, tc.want)
			}
			if verbs := fake.verbs(t); !equalStrings(verbs, tc.wantVerbs) {
				t.Fatalf("verbs run = %v, want %v", verbs, tc.wantVerbs)
			}

			installNote := ServiceNoteFor(InstallResult{Version: "0.2.1", Service: &got})
			assertNote(t, "install", installNote, tc.installSay)
			updateNote := serviceNote(&got, "0.2.1", true)
			assertNote(t, "update", updateNote, tc.updateSay)
		})
	}
}

// A fresh install that brings the daemon up did what it was asked, so it stays
// quiet; the payload still reports it, because an agent reading service.started
// has no way to know the sentence is update-only.
func TestServiceStarted_ReportedInTheJSONOnBothPathsAndSpokenOnlyOnUpdate(t *testing.T) {
	fake := writeFakeObey(t, true, "0.2.1")
	got := obeyServiceSwap(context.Background(), daemonStateStopped, false)

	if !got.Started {
		t.Fatalf("service result = %+v, want started true for a daemon the install brought up", got)
	}
	if verbs := fake.verbs(t); !equalStrings(verbs, []string{"install"}) {
		t.Fatalf("verbs run = %v, want only the install verb", verbs)
	}
	if note := ServiceNoteFor(InstallResult{Version: "0.2.1", Service: &got}); note != "" {
		t.Fatalf("install note = %q, want an install that started the daemon to stay quiet", note)
	}
	if note := serviceNote(&got, "0.2.1", true); !strings.Contains(note, "started on the new version") {
		t.Fatalf("update note = %q, want it to say the daemon was started rather than restarted", note)
	}
}

// A failed install verb leaves the process alone: there is no unit to restart
// through, and nothing was started.
func TestObeyServiceSwap_FailedInstallVerbStartsNothing(t *testing.T) {
	fake := writeFakeObey(t, true, "0.2.1")
	writeFile(t, fake.path, refusingObeyScript)

	got := obeyServiceSwap(context.Background(), daemonStateStopped, false)
	if got.Installed || got.Started || got.Restarted || got.Deferred {
		t.Fatalf("service result = %+v, want nothing claimed after a failed install verb", got)
	}
	if !strings.Contains(got.Error, "Operation not permitted") {
		t.Fatalf("error = %q, want the supervisor's own refusal", got.Error)
	}
}

// refusingObeyScript answers the contract probe and then refuses every verb,
// which is the supervisor saying no rather than obey failing to run.
const refusingObeyScript = `#!/bin/sh
dir=$(dirname "$0")
case "$1 $2 $3" in
  "--version  ") echo "obey version 0.2.1"; exit 0;;
  "service --help ") cat "$dir/service-help.txt"; exit 0;;
  "service install --help") cat "$dir/service-install-help.txt"; exit 0;;
esac
echo "launchctl: Operation not permitted" >&2
exit 1
`

func assertNote(t *testing.T, path, note string, want []string) {
	t.Helper()
	if len(want) == 0 {
		if note != "" {
			t.Fatalf("%s note = %q, want it quiet", path, note)
		}
		return
	}
	for _, phrase := range want {
		if !strings.Contains(note, phrase) {
			t.Fatalf("%s note = %q, want it to contain %q", path, note, phrase)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
