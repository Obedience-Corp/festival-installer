package cli_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// hostileManifestBytes builds a marketplace manifest whose package id and
// display name carry real terminal control runes: the id holds a CSI colour
// sequence and the display name holds an OSC window-title set terminated by
// BEL. json.Marshal encodes those bytes as their JSON unicode escapes, which
// parses back into the runes the renderer must strip. The manifest schema only
// enforces minLength 1 on these fields, so they pass validation.
func hostileManifestBytes(t *testing.T) []byte {
	t.Helper()
	m := map[string]any{
		"id":             "acme/hostile",
		"name":           "Hostile",
		"schema_version": "1",
		"packages": []any{
			map[string]any{
				"id":            "acme/\x1b[31mpkg",
				"display_name":  "\x1b]0;PWNED\x07Demo",
				"class":         "plugin",
				"host_runtimes": []string{"fest-cli"},
				"channels":      []string{"latest"},
				"manifest_path": "packages/pkg/obey-package.json",
			},
		},
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal hostile manifest: %v", err)
	}
	return b
}

func hostileFixtureRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-b", "main")
	writeFile(t, filepath.Join(dir, "obey-marketplace.json"), string(hostileManifestBytes(t)))
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-m", "init")
	return dir
}

// assertNoTerminalControls fails if out carries any control rune other than the
// tab and newline the renderer itself emits for table and line layout.
func assertNoTerminalControls(t *testing.T, label, out string) {
	t.Helper()
	for _, r := range out {
		if r == '\t' || r == '\n' {
			continue
		}
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			t.Fatalf("%s output contains control rune %U:\n%q", label, r, out)
		}
	}
}

func TestBrowseRenderStripsMarketplaceEscapes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	repo := hostileFixtureRepo(t)
	if _, errOut, err := runInstaller(t, "marketplace", "add", repo, "--name", "hostile"); err != nil {
		t.Fatalf("marketplace add: %v\n%s", err, errOut)
	}

	out, _, err := runInstaller(t, "browse")
	if err != nil {
		t.Fatalf("browse: %v", err)
	}
	assertNoTerminalControls(t, "browse", out)
	for _, want := range []string{"pkg", "PWNED", "Demo"} {
		if !strings.Contains(out, want) {
			t.Fatalf("browse output dropped visible text %q:\n%q", want, out)
		}
	}
}

func TestMarketplaceListRenderStripsHostileSourceName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	repo := hostileFixtureRepo(t)
	// A hostile source label reaches the list table via the source name. ESC is
	// a legal filesystem byte, so the clone still lands under marketplaces/.
	hostileName := "ho\x1b[31mstile"
	if _, errOut, err := runInstaller(t, "marketplace", "add", repo, "--name", hostileName); err != nil {
		t.Fatalf("marketplace add: %v\n%s", err, errOut)
	}

	out, _, err := runInstaller(t, "marketplace", "list")
	if err != nil {
		t.Fatalf("marketplace list: %v", err)
	}
	assertNoTerminalControls(t, "marketplace list", out)
	if !strings.Contains(out, "stile") {
		t.Fatalf("marketplace list dropped source name text:\n%q", out)
	}
}

func TestBrowseJSONPreservesRawValues(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	repo := hostileFixtureRepo(t)
	if _, errOut, err := runInstaller(t, "marketplace", "add", repo, "--name", "hostile"); err != nil {
		t.Fatalf("marketplace add: %v\n%s", err, errOut)
	}

	out, _, err := runInstaller(t, "browse", "--json")
	if err != nil {
		t.Fatalf("browse --json: %v", err)
	}
	var res struct {
		Groups []struct {
			Packages []struct {
				ID          string `json:"id"`
				DisplayName string `json:"display_name"`
			} `json:"packages"`
		} `json:"groups"`
	}
	dataOf(t, out, &res)
	var found bool
	for _, g := range res.Groups {
		for _, p := range g.Packages {
			if strings.ContainsRune(p.ID, 0x1b) && strings.ContainsRune(p.DisplayName, 0x1b) {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("JSON output must preserve raw escape bytes for machine consumers, got: %s", out)
	}
}
