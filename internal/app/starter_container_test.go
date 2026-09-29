//go:build container_fs

package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExecuteStarterProcess(t *testing.T) {
	dir := t.TempDir()
	camp := filepath.Join(dir, "camp")
	// The selected Camp and its sibling Fest must win over inherited PATH.
	t.Setenv("CAMP_ROOT", "/wrong-camp")
	script := `#!/bin/sh
[ "$1 $2" = 'setup --json' ] || exit 2
[ -z "$CAMP_ROOT" ] || exit 3
[ "${PATH%%:*}" = "$(dirname "$0")" ] || exit 4
printf '%s' '{"schema_version":1,"action":"created","path":"/camps/festival"}'
`
	if err := os.WriteFile(camp, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := executeStarter(context.Background(), camp); got.Action != "created" {
		t.Fatalf("%+v", got)
	}
	if err := os.WriteFile(camp, []byte("#!/bin/sh\necho 'disk full' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := executeStarter(context.Background(), camp)
	if got.Action != "failed" || !strings.Contains(got.Message, "disk full") {
		t.Fatalf("%+v", got)
	}
}

func TestExecuteStarterCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	got := executeStarter(ctx, "/must-not-execute")
	if got.Action != "failed" || time.Since(started) > time.Second {
		t.Fatalf("%+v", got)
	}
}
