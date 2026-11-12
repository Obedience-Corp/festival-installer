package cli

import (
	"bytes"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/app"
)

func TestAppRenderResultLines(t *testing.T) {
	cases := []struct {
		name   string
		action string
		res    app.AppResult
		want   string
	}{
		{"update", "update", app.AppResult{Version: "0.2.1", PreviousVersion: "0.2.0", Location: "/h/festival-app", Changed: true}, "Updated Festival App 0.2.0 -> 0.2.1 at /h/festival-app\n"},
		{"fresh install", "install", app.AppResult{Version: "0.2.1", Location: "/h/festival-app", Changed: true}, "Installed Festival App 0.2.1 at /h/festival-app\n"},
		{"uninstall", "uninstall", app.AppResult{PreviousVersion: "0.2.1", Location: "/h/festival-app", Changed: true}, "Removed Festival App 0.2.1 from /h/festival-app\n"},
		{"already current", "update", app.AppResult{Version: "0.2.1", Location: "/h/festival-app"}, "Festival App is already current at 0.2.1\n"},
		{"nothing to say", "update", app.AppResult{}, ""},
		{"notes", "update", app.AppResult{Version: "0.2.1", Location: "/h/x", Changed: true, PreviousVersion: "0.2.1", Notes: []string{"first", "second"}}, "Installed Festival App 0.2.1 at /h/x\nfirst\nsecond\n"},
	}
	for _, tc := range cases {
		var buf bytes.Buffer
		if err := renderAppResult(&buf, tc.action, tc.res); err != nil {
			t.Fatal(err)
		}
		if buf.String() != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, buf.String(), tc.want)
		}
	}
}
