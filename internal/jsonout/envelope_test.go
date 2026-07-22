package jsonout_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Obedience-Corp/obey-installer/internal/jsonout"
)

func TestSuccess_EnvelopeShape(t *testing.T) {
	var buf bytes.Buffer
	if err := jsonout.Success(&buf, "install", map[string]any{"version": "0.2.10"}, nil); err != nil {
		t.Fatalf("Success: %v", err)
	}
	out := buf.String()
	var env struct {
		OK            bool            `json:"ok"`
		Action        string          `json:"action"`
		SchemaVersion string          `json:"schema_version"`
		Warnings      []string        `json:"warnings"`
		Data          json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !env.OK || env.Action != "install" || env.SchemaVersion != jsonout.SchemaVersion {
		t.Fatalf("unexpected envelope: %+v", env)
	}
	if env.Warnings == nil {
		t.Fatal("warnings should be non-nil")
	}
	if !strings.Contains(out, "\"warnings\": []") {
		t.Fatalf("nil warnings must serialize as [], got: %s", out)
	}
}

func TestFailure_ErrorShape(t *testing.T) {
	var buf bytes.Buffer
	if err := jsonout.Failure(&buf, "install", "selector_ambiguous", "two matches"); err != nil {
		t.Fatalf("Failure: %v", err)
	}
	var env struct {
		OK    bool `json:"ok"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.OK || env.Error == nil || env.Error.Code != "selector_ambiguous" {
		t.Fatalf("unexpected failure envelope: %+v", env)
	}
	if !strings.Contains(buf.String(), "\"warnings\": []") {
		t.Fatalf("failure warnings must be [], got: %s", buf.String())
	}
}

func TestFailure_MatchesSuccessConventions(t *testing.T) {
	var success, failure bytes.Buffer
	if err := jsonout.Success(&success, "install", map[string]any{"version": "0.2.10"}, nil); err != nil {
		t.Fatalf("Success: %v", err)
	}
	if err := jsonout.Failure(&failure, "install", "E_INSTALL_TARGET", "unknown target"); err != nil {
		t.Fatalf("Failure: %v", err)
	}

	decode := func(name string, buf *bytes.Buffer) map[string]any {
		var m map[string]any
		if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
			t.Fatalf("%s decode: %v\n%s", name, err, buf.String())
		}
		return m
	}
	s := decode("success", &success)
	f := decode("failure", &failure)

	for _, key := range []string{"ok", "action", "schema_version", "warnings"} {
		if _, ok := s[key]; !ok {
			t.Fatalf("success envelope missing shared key %q: %v", key, s)
		}
		if _, ok := f[key]; !ok {
			t.Fatalf("failure envelope missing shared key %q: %v", key, f)
		}
	}
	if s["action"] != f["action"] || s["schema_version"] != f["schema_version"] {
		t.Fatalf("action/schema_version diverge: success=%v failure=%v", s, f)
	}
	if s["schema_version"] != jsonout.SchemaVersion {
		t.Fatalf("schema_version = %v, want %q", s["schema_version"], jsonout.SchemaVersion)
	}
	if s["ok"] != true || f["ok"] != false {
		t.Fatalf("ok must be true for success and false for failure: success=%v failure=%v", s["ok"], f["ok"])
	}

	// Success carries data and no error; failure carries error and no data.
	if _, ok := s["data"]; !ok {
		t.Fatalf("success envelope must carry data: %v", s)
	}
	if _, ok := s["error"]; ok {
		t.Fatalf("success envelope must omit error: %v", s)
	}
	if _, ok := f["error"]; !ok {
		t.Fatalf("failure envelope must carry error: %v", f)
	}
	if _, ok := f["data"]; ok {
		t.Fatalf("failure envelope must omit data: %v", f)
	}
}
