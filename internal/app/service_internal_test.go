package app

import (
	"strings"
	"testing"
)

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
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := serviceNote(tc.svc, tc.version)
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
