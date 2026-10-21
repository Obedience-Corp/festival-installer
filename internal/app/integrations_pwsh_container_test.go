//go:build !windows && container_fs

package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// The pwsh executable selects its own profile, but helper generators receive
// "powershell", the dialect understood by camp and compatible fest releases.
func TestPwshIntegrationUsesPowerShellDialect(t *testing.T) {
	_, managed := integrationFixture(t, "pwsh")
	// This fixture includes only camp; do not imply that a fest version without
	// a PowerShell generator can provide fgo.
	for _, tool := range []string{"fest", "obey"} {
		if err := os.Remove(filepath.Join(managed, "bin", tool)); err != nil {
			t.Fatal(err)
		}
	}
	script := "#!/bin/sh\ncase \"$*\" in\n'shell-init powershell') echo 'function cgo { }';;\n*shell-init*) exit 24;;\n*) echo 'camp version 9.0.0';;\nesac\n"
	if err := os.WriteFile(filepath.Join(managed, "bin", "camp"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	// pwsh from Microsoft's container lives outside the GUI PATH used by the
	// fixture; an absolute --shell must also use that executable for probing.
	r, err := ReconcileIntegrations(context.Background(), IntegrationOptions{Shell: "/opt/microsoft/powershell/7/pwsh"})
	if err != nil || !r.Ready {
		t.Fatalf("%+v %v", r, err)
	}
	if r.Shell != "pwsh" {
		t.Fatalf("wrong edition: %s", r.Shell)
	}
}
