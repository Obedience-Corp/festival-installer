package main

import "testing"

func TestTUILaunchDecisionRequiresBothStreams(t *testing.T) {
	tests := []struct {
		name       string
		force      bool
		stdinTTY   bool
		stdoutTTY  bool
		wantLaunch bool
		wantError  bool
	}{
		{name: "bare interactive", stdinTTY: true, stdoutTTY: true, wantLaunch: true},
		{name: "stdin redirected", stdoutTTY: true},
		{name: "stdout redirected", stdinTTY: true},
		{name: "bare noninteractive"},
		{name: "forced interactive", force: true, stdinTTY: true, stdoutTTY: true, wantLaunch: true},
		{name: "forced stdin redirected", force: true, stdoutTTY: true, wantError: true},
		{name: "forced stdout redirected", force: true, stdinTTY: true, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tuiLaunchDecision(tt.force, tt.stdinTTY, tt.stdoutTTY)
			if (err != nil) != tt.wantError {
				t.Fatalf("error = %v, want error %v", err, tt.wantError)
			}
			if got != tt.wantLaunch {
				t.Fatalf("launch = %v, want %v", got, tt.wantLaunch)
			}
		})
	}
}
