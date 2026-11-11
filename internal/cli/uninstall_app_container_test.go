//go:build container_fs && linux

package cli_test

import (
	"os"
	"strings"
	"testing"
)

func TestAppUninstallJSONEnvelope(t *testing.T) {
	e := newCLIAppEnv(t)
	e.publish("0.2.0", []byte("appimage 0.2.0"))
	res := e.install()
	out, errOut, err := runInstaller(t, "uninstall", "app", "--json")
	if err != nil {
		t.Fatalf("%v\n%s", err, errOut)
	}
	var data updateAppData
	dataOf(t, out, &data)
	if !strings.Contains(out, `"action": "uninstall"`) && !strings.Contains(out, `"action":"uninstall"`) {
		t.Fatalf("not an uninstall envelope: %s", out)
	}
	d := data
	if d.Package != "obedience-corp/festival-app" || d.PreviousVersion != "0.2.0" || d.Format != "appimage" || d.Location != res.Location || d.Adopted {
		t.Fatalf("%+v", d)
	}
	for _, key := range []string{`"package"`, `"version"`, `"previous_version"`, `"format"`, `"location"`, `"adopted"`} {
		if !strings.Contains(out, key) {
			t.Fatalf("missing key %s in %s", key, out)
		}
	}
	if _, statErr := os.Lstat(res.Location); !os.IsNotExist(statErr) {
		t.Fatal("AppImage still present")
	}
	t.Logf("uninstall app --json: %s", strings.TrimSpace(out))
}
