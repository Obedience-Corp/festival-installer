package cli_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUninstallObey_RejectsObSelector(t *testing.T) {
	t.Setenv("OBEY_INSTALLER_HOME", t.TempDir())
	t.Setenv("FESTIVAL_HOME", "")
	t.Setenv("HOME", t.TempDir())

	_, _, err := runInstaller(t, "install", "ob", "--allow-unverified", "--json")
	if err == nil {
		t.Fatal("ob is a binary inside the obey package, not an install target")
	}
	if !hasErrorCode(err, "E_INSTALL_TARGET") {
		t.Fatalf("expected E_INSTALL_TARGET for ob, got %v", err)
	}
	if !strings.Contains(err.Error(), "obey") {
		t.Fatalf("the unknown-target message must name obey, got %v", err)
	}

	_, _, err = runInstaller(t, "uninstall", "ob", "--json")
	if err == nil {
		t.Fatal("ob is not an uninstall target")
	}
	if !hasErrorCode(err, "E_UNINSTALL_TARGET") || !strings.Contains(err.Error(), "obey") {
		t.Fatalf("expected E_UNINSTALL_TARGET naming obey, got %v", err)
	}

	_, _, err = runInstaller(t, "update", "ob", "--json")
	if err == nil {
		t.Fatal("ob is not an update target")
	}
	if !hasErrorCode(err, "E_UPDATE_TARGET") || !strings.Contains(err.Error(), "obey") {
		t.Fatalf("expected E_UPDATE_TARGET naming obey, got %v", err)
	}
}

func TestInstallFestival_PayloadHasNoServiceKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	t.Setenv("FESTIVAL_HOME", "")
	t.Setenv("HOME", t.TempDir())

	tarball := buildSuiteTarGz(t, map[string]string{
		"camp":     "#!/bin/sh\necho camp\n",
		"fest":     "#!/bin/sh\necho fest\n",
		"festival": "#!/bin/sh\necho festival\n",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(tarball)
	}))
	t.Cleanup(srv.Close)

	repo := fixtureInstallMarketplace(t, srv.URL+"/festival.tar.gz", sha256Hex(tarball))
	if _, errOut, err := runInstaller(t, "marketplace", "add", repo, "--name", "official-obey", "--allow-unverified"); err != nil {
		t.Fatalf("marketplace add: %v\n%s", err, errOut)
	}

	out, errOut, err := runInstaller(t, "install", "festival", "--allow-unverified", "--json")
	if err != nil {
		t.Fatalf("install festival: %v\n%s", err, errOut)
	}
	if strings.Contains(out, `"service"`) {
		t.Fatalf("the suite install payload must carry no service key:\n%s", out)
	}
}

// The human (non-JSON) absent path must name the obey install command. The
// suite default (`festival install festival`) would send a user with no daemon
// to reinstall camp and fest instead.
func TestUpdateObey_AbsentHumanOutput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	t.Setenv("FESTIVAL_HOME", "")
	t.Setenv("HOME", t.TempDir())
	// An empty PATH entry keeps a host-installed obey out of ResolveTool, so
	// "absent" is measured against the fixture and not the developer machine.
	t.Setenv("PATH", t.TempDir())

	out, errOut, err := runInstaller(t, "update", "obey")
	if err != nil {
		t.Fatalf("update obey with nothing installed: %v\n%s", err, errOut)
	}
	if !strings.Contains(out, "run `festival install obey`") {
		t.Fatalf("expected stdout to name `festival install obey`, got %q", out)
	}
	if strings.Contains(out, "festival install festival") {
		t.Fatalf("an absent obey must not point at the suite install, got %q", out)
	}
	if !strings.Contains(errOut, "festival install obey") {
		t.Fatalf("expected the absent warning on stderr, got %q", errOut)
	}
}
