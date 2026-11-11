package cli_test

import (
	"strings"
	"testing"
)

func TestAppHelpNamesAppTarget(t *testing.T) {
	t.Setenv("OBEY_INSTALLER_HOME", t.TempDir())
	for _, cmd := range []string{"install", "update", "uninstall"} {
		out, errOut, err := runInstaller(t, cmd, "--help")
		if err != nil {
			t.Fatalf("%s --help: %v\n%s", cmd, err, errOut)
		}
		if !strings.Contains(out, "|app|") {
			t.Errorf("%s --help does not name the app target:\n%s", cmd, out)
		}
		if comp, _, err := runInstaller(t, "__complete", cmd, ""); err != nil || !strings.Contains(comp, "app\n") {
			t.Errorf("%s completion does not offer app: %q %v", cmd, comp, err)
		}
	}
	out, _, _ := runInstaller(t, "install", "--help")
	if !strings.Contains(out, "--format") || !strings.Contains(out, "Quit Festival App before installing over it.") {
		t.Errorf("install --help lacks the --format flag or the app paragraph:\n%s", out)
	}
}
