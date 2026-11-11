package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/app"
)

func rowFor(t *testing.T, a app.StatusApp) string {
	t.Helper()
	var out bytes.Buffer
	if err := renderStatusTable(&out, app.StatusReport{Tools: []app.StatusToolEntry{{Tool: "camp", Origin: app.OriginAbsent}}, App: a}); err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.HasPrefix(line, "Festival App") {
			return strings.Join(strings.Fields(line), " ")
		}
	}
	t.Fatalf("no Festival App row in:\n%s", out.String())
	return ""
}

func ptr[T any](v T) *T { return &v }

func TestAppRenderStatusTableRow(t *testing.T) {
	cases := []struct {
		name string
		app  app.StatusApp
		want string
	}{
		{"installed with an update available", app.StatusApp{Installed: true, Version: ptr("0.2.0"), ReceiptVersion: ptr("0.2.0"), Location: ptr("/h/.local/bin/festival-app"), UpdatedByApp: ptr(false), Latest: ptr("0.2.1"), UpdateAvailable: ptr(true)},
			"Festival App /h/.local/bin/festival-app 0.2.0 (0.2.1 available) managed"},
		{"updated by the App", app.StatusApp{Installed: true, ReceiptVersion: ptr("0.2.0"), Location: ptr("/h/festival-app"), UpdatedByApp: ptr(true)},
			"Festival App /h/festival-app (receipt 0.2.0) (updated by the App) managed"},
		{"removed outside festival", app.StatusApp{ReceiptVersion: ptr("0.2.0"), Location: ptr("/h/festival-app"), Note: ptr("removed outside festival")},
			"Festival App /h/festival-app (receipt 0.2.0) removed outside festival"},
		{"absent", app.StatusApp{}, "Festival App - - absent"},
	}
	for _, tc := range cases {
		if got := rowFor(t, tc.app); got != tc.want {
			t.Errorf("%s:\n got %q\nwant %q", tc.name, got, tc.want)
		}
	}
	var out bytes.Buffer
	_ = renderStatusTable(&out, app.StatusReport{Tools: []app.StatusToolEntry{{Tool: "camp", Origin: app.OriginAbsent}}})
	if !strings.Contains(out.String(), "TOOL") || !strings.Contains(out.String(), "camp") {
		t.Errorf("tool rows changed:\n%s", out.String())
	}
}
