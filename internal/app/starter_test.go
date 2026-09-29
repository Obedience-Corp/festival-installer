package app

import "testing"

func TestStarterResultContract(t *testing.T) {
	for _, tt := range []struct{ input, action string }{
		{`{"schema_version":1,"action":"created","path":"/home/me/camps/festival"}`, "created"},
		{`{"schema_version":1,"action":"complete"}`, "complete"},
		{`{"schema_version":1,"action":"skipped","message":"occupied"}`, "skipped"},
		{`{"schema_version":1,"action":"created"}`, "failed"},
		{`{"schema_version":2,"action":"complete"}`, "failed"},
		{`{"schema_version":1,"action":"mystery"}`, "failed"},
		{`not JSON`, "failed"},
	} {
		t.Run(tt.input, func(t *testing.T) {
			if got := parseStarterResult([]byte(tt.input)); got.Action != tt.action {
				t.Fatalf("got %+v, want %s", got, tt.action)
			}
		})
	}
}
