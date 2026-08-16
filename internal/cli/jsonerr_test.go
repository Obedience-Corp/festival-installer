package cli_test

import (
	"encoding/json"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/jsonout"
)

type failureEnvelope struct {
	OK            bool     `json:"ok"`
	Action        string   `json:"action"`
	SchemaVersion string   `json:"schema_version"`
	Warnings      []string `json:"warnings"`
	Error         *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func TestJSONFailureEnvelope_EmittedOnStdout(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantAction string
		wantCode   string
	}{
		{
			name:       "unknown update target",
			args:       []string{"update", "bogus", "--json"},
			wantAction: "update",
			wantCode:   "E_UPDATE_TARGET",
		},
		{
			name:       "invalid install channel",
			args:       []string{"install", "festival", "--channel", "bogus", "--json"},
			wantAction: "install",
			wantCode:   "E_INSTALL_CHANNEL",
		},
		{
			// Nested command: must not collide with top-level "list".
			name:       "marketplace refresh unknown source",
			args:       []string{"marketplace", "refresh", "no-such-source", "--json"},
			wantAction: "marketplace refresh",
			wantCode:   "E_SOURCE_NOT_FOUND",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OBEY_INSTALLER_HOME", t.TempDir())

			out, _, err := runInstaller(t, tc.args...)
			if err == nil {
				t.Fatal("expected a non-nil error so the process exits non-zero")
			}
			if !hasErrorCode(err, tc.wantCode) {
				t.Fatalf("returned error missing code %s: %v", tc.wantCode, err)
			}

			var env failureEnvelope
			if decErr := json.Unmarshal([]byte(out), &env); decErr != nil {
				t.Fatalf("stdout is not a single JSON envelope: %v\n%s", decErr, out)
			}
			if env.OK {
				t.Fatalf("failure envelope must have ok=false: %s", out)
			}
			if env.Action != tc.wantAction {
				t.Fatalf("action = %q, want %q", env.Action, tc.wantAction)
			}
			if env.SchemaVersion != jsonout.SchemaVersion {
				t.Fatalf("schema_version = %q, want %q", env.SchemaVersion, jsonout.SchemaVersion)
			}
			if env.Warnings == nil {
				t.Fatalf("warnings must serialize as [], got null: %s", out)
			}
			if env.Error == nil || env.Error.Code != tc.wantCode {
				t.Fatalf("error payload = %+v, want code %s", env.Error, tc.wantCode)
			}
			if env.Error.Message == "" {
				t.Fatalf("failure envelope must carry a message: %s", out)
			}
		})
	}
}

func TestJSONFailure_HumanPathUnchanged(t *testing.T) {
	t.Setenv("OBEY_INSTALLER_HOME", t.TempDir())

	out, _, err := runInstaller(t, "update", "bogus")
	if err == nil {
		t.Fatal("expected a non-nil error on the human path")
	}
	if !hasErrorCode(err, "E_UPDATE_TARGET") {
		t.Fatalf("returned error missing code E_UPDATE_TARGET: %v", err)
	}
	if out != "" {
		t.Fatalf("human path must not print a JSON envelope on stdout, got: %q", out)
	}
}

func TestJSONFailure_DoctorEmitsSingleEnvelope(t *testing.T) {
	t.Setenv("OBEY_INSTALLER_HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())

	out, _, err := runInstaller(t, "doctor", "--json")
	if err == nil {
		t.Fatal("expected doctor to exit non-zero when a check fails")
	}
	if !hasErrorCode(err, "E_DOCTOR_FAIL") {
		t.Fatalf("returned error missing code E_DOCTOR_FAIL: %v", err)
	}

	var env struct {
		OK     bool   `json:"ok"`
		Action string `json:"action"`
	}
	if decErr := json.Unmarshal([]byte(out), &env); decErr != nil {
		t.Fatalf("doctor stdout must be a single JSON envelope, not two: %v\n%s", decErr, out)
	}
	if !env.OK || env.Action != "doctor" {
		t.Fatalf("doctor keeps its own data envelope (ok=true): %+v", env)
	}
}
