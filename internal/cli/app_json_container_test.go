//go:build container_fs && linux

package cli_test

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/source"
)

type envelope struct {
	OK            bool            `json:"ok"`
	Action        string          `json:"action"`
	SchemaVersion string          `json:"schema_version"`
	Warnings      []string        `json:"warnings"`
	Data          json.RawMessage `json:"data"`
	Error         struct {
		Code string `json:"code"`
	} `json:"error"`
}

func parseEnvelope(t *testing.T, out string) envelope {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(out))
	var env envelope
	if err := dec.Decode(&env); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		t.Fatalf("stdout holds more than one JSON object:\n%s", out)
	}
	return env
}

func TestAppJSONInstallUpdateUninstallEnvelopes(t *testing.T) {
	e := newCLIAppEnv(t)
	e.publish("0.2.0", []byte("appimage 0.2.0"))
	wantKeys := []string{"adopted", "format", "location", "package", "previous_version", "version"}
	for _, step := range []struct {
		action string
		args   []string
	}{
		{"install", []string{"install", "app", "--format", "appimage", "--json"}},
		{"update", []string{"update", "app", "--json"}},
		{"uninstall", []string{"uninstall", "app", "--json"}},
	} {
		out, errOut, err := runInstaller(t, step.args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", step.args, err, errOut)
		}
		env := parseEnvelope(t, out)
		if !env.OK || env.Action != step.action || env.SchemaVersion != "festival/v1alpha1" || env.Warnings == nil {
			t.Fatalf("%s envelope: %+v\n%s", step.action, env, out)
		}
		var data map[string]any
		if err := json.Unmarshal(env.Data, &data); err != nil {
			t.Fatal(err)
		}
		var keys []string
		for k := range data {
			keys = append(keys, k)
		}
		if len(keys) != len(wantKeys) {
			t.Fatalf("%s data keys = %v, want %v", step.action, keys, wantKeys)
		}
		for _, k := range wantKeys {
			if _, ok := data[k]; !ok {
				t.Fatalf("%s data lacks %q: %v", step.action, k, data)
			}
		}
		t.Logf("%s --json: %s", step.action, strings.TrimSpace(out))
	}
}

func TestAppJSONFailureEnvelopes(t *testing.T) {
	newCLIAppEnv(t)
	cases := []struct {
		name string
		args []string
		code string
	}{
		{"unknown format", []string{"install", "app", "--format", "rpm", "--json"}, "E_APP_FORMAT_UNSUPPORTED"},
		{"non-stable channel", []string{"install", "app", "--channel", "rc", "--json"}, "E_APP_NOT_AVAILABLE"},
		{"format on another target", []string{"install", "festival", "--format", "appimage", "--json"}, "E_APP_FORMAT_UNSUPPORTED"},
	}
	for _, tc := range cases {
		out, _, err := runInstaller(t, tc.args...)
		if err == nil {
			t.Errorf("%s: expected a failure", tc.name)
		}
		env := parseEnvelope(t, out)
		if env.OK || env.Error.Code != tc.code {
			t.Errorf("%s: %+v\n%s", tc.name, env, out)
		}
	}
}

func TestAppInstallTargetRoutes(t *testing.T) {
	e := newCLIAppEnv(t)
	e.publish("0.2.0", []byte("appimage 0.2.0"))
	// The container has apt-get, so a bare install picks the deb. Only the
	// AppImage is published, so the install fails with an App code, which proves
	// the target reached the App path without installing anything.
	for _, target := range []string{"festival-app", "obedience-corp/festival-app"} {
		_, err := app.InstallTarget(context.Background(), target, app.InstallOptions{Verify: source.DefaultVerifyOptions(io.Discard, false)})
		code := errpkg.Code(err)
		if err == nil || !strings.HasPrefix(code, "E_APP_") {
			t.Errorf("%s was not routed to the App path (an App code is expected here, the deb is not published): %v", target, err)
		}
	}
}
