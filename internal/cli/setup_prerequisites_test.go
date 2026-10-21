package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/app"
)

func TestPrerequisitesCommandReportsIncompleteSetup(t *testing.T) {
	for _, tc := range []struct {
		name    string
		report  app.PrerequisiteReport
		asJSON  bool
		wantErr bool
	}{
		{"ready", app.PrerequisiteReport{Ready: true, Message: "Git is available."}, false, false},
		{"failed", app.PrerequisiteReport{Message: "Git installer unavailable."}, false, true},
		{"pending", app.PrerequisiteReport{Pending: true, Message: "Complete the system installer."}, false, true},
		{"pending JSON", app.PrerequisiteReport{Pending: true, Message: "Complete the system installer."}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newPrerequisitesCommand(func(context.Context) app.PrerequisiteReport { return tc.report })
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			if tc.asJSON {
				cmd.SetArgs([]string{"--json"})
			}
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			err := cmd.Execute()
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, output = %q", err, out.String())
			}
			if tc.asJSON {
				var envelope struct {
					Data app.PrerequisiteReport `json:"data"`
				}
				if err := json.Unmarshal(out.Bytes(), &envelope); err != nil || envelope.Data != tc.report {
					t.Fatalf("JSON report = %+v, error = %v", envelope.Data, err)
				}
			} else if !strings.Contains(out.String(), tc.report.Message) {
				t.Fatalf("missing setup message: %q", out.String())
			}
		})
	}
}
