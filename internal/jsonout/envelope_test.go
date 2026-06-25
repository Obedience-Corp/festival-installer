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
