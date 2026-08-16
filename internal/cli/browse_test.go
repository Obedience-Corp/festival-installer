package cli_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

const browseMarketplaceJSON = `{
  "id": "obedience-corp/official",
  "name": "Official",
  "schema_version": "1",
  "packages": [
    {"id":"obedience-corp/fest","display_name":"Fest","class":"tool","host_runtimes":["fest-cli","fest-extension"],"channels":["stable"],"manifest_path":"packages/fest/obey-package.json"},
    {"id":"acme/fest-demo","display_name":"Fest Demo","class":"plugin","host_runtimes":["fest-cli"],"targets":[{"package":"obedience-corp/festival","runtime":"fest-cli","version_constraint":">=0.4.0"}],"channels":["latest"],"manifest_path":"packages/fest-demo/obey-package.json"},
    {"id":"acme/camp-graph","display_name":"Camp Graph","class":"plugin","host_runtimes":["camp-cli"],"channels":["latest"],"manifest_path":"packages/camp-graph/obey-package.json"}
  ]
}
`

func browseFixtureRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-b", "main")
	writeFile(t, filepath.Join(dir, "obey-marketplace.json"), browseMarketplaceJSON)
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-m", "init")
	return dir
}

type browseJSON struct {
	Groups []struct {
		HostRuntime string `json:"host_runtime"`
		Packages    []struct {
			ID           string   `json:"id"`
			HostRuntimes []string `json:"host_runtimes"`
			Channels     []string `json:"channels"`
		} `json:"packages"`
	} `json:"groups"`
}

func TestBrowse_ProductKindFilterJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	repo := browseFixtureRepo(t)
	if _, errOut, err := runInstaller(t, "marketplace", "add", repo, "--name", "official-obey"); err != nil {
		t.Fatalf("marketplace add: %v\n%s", err, errOut)
	}

	out, _, err := runInstaller(t, "browse", "--product", "fest", "--kind", "plugin", "--json")
	if err != nil {
		t.Fatalf("browse: %v", err)
	}
	var res browseJSON
	dataOf(t, out, &res)
	var ids []string
	for _, g := range res.Groups {
		for _, p := range g.Packages {
			ids = append(ids, p.ID)
		}
	}
	if len(ids) != 1 || ids[0] != "acme/fest-demo" {
		t.Fatalf("expected only acme/fest-demo, got %v", ids)
	}
}

func TestBrowse_EmptyResultSerializesAsArray(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	repo := browseFixtureRepo(t)
	if _, errOut, err := runInstaller(t, "marketplace", "add", repo, "--name", "official-obey"); err != nil {
		t.Fatalf("marketplace add: %v\n%s", err, errOut)
	}

	out, _, err := runInstaller(t, "browse", "--product", "no-such-product", "--json")
	if err != nil {
		t.Fatalf("browse: %v", err)
	}
	if !strings.Contains(out, "\"groups\": []") {
		t.Fatalf("expected empty groups to serialize as [], got: %s", out)
	}
}

func TestBrowse_AllGroupedJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	repo := browseFixtureRepo(t)
	if _, errOut, err := runInstaller(t, "marketplace", "add", repo, "--name", "official-obey"); err != nil {
		t.Fatalf("marketplace add: %v\n%s", err, errOut)
	}

	out, _, err := runInstaller(t, "browse", "--json")
	if err != nil {
		t.Fatalf("browse: %v", err)
	}
	var res browseJSON
	dataOf(t, out, &res)
	groups := map[string]bool{}
	for _, g := range res.Groups {
		groups[g.HostRuntime] = true
		for _, p := range g.Packages {
			if p.HostRuntimes == nil || p.Channels == nil {
				t.Fatalf("expected non-null slice fields, got %+v", p)
			}
		}
	}
	for _, want := range []string{"camp-cli", "fest-cli", "fest-extension"} {
		if !groups[want] {
			t.Fatalf("missing group %s in %v", want, groups)
		}
	}
}

// When the official source is already registered, ensureOfficialSeed's
// len(sources) > 0 guard skips the clone entirely, so browse must render the
// catalog with no seed warning at all (JSON envelope and table stderr alike).
func TestBrowse_AlreadySeededOfficialSourceNoWarning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	repo := browseFixtureRepo(t)
	if _, errOut, err := runInstaller(t, "marketplace", "add", repo, "--name", "official-obey"); err != nil {
		t.Fatalf("marketplace add: %v\n%s", err, errOut)
	}

	jsonOut, jsonErrOut, err := runInstaller(t, "browse", "--json")
	if err != nil {
		t.Fatalf("browse --json: %v\n%s", err, jsonErrOut)
	}
	if jsonErrOut != "" {
		t.Fatalf("expected no stderr for an already-seeded home, got %q", jsonErrOut)
	}
	var env struct {
		Warnings []string `json:"warnings"`
	}
	if jsonErr := json.Unmarshal([]byte(jsonOut), &env); jsonErr != nil {
		t.Fatalf("decode envelope: %v\n%s", jsonErr, jsonOut)
	}
	if len(env.Warnings) != 0 {
		t.Fatalf("expected no warnings in the envelope, got %v", env.Warnings)
	}
	var res browseJSON
	dataOf(t, jsonOut, &res)
	if len(res.Groups) == 0 {
		t.Fatalf("expected catalog populated from the pre-registered source, got none: %s", jsonOut)
	}

	tableOut, tableErrOut, err := runInstaller(t, "browse")
	if err != nil {
		t.Fatalf("browse: %v\n%s", err, tableErrOut)
	}
	if tableErrOut != "" {
		t.Fatalf("expected no stderr warning in table mode, got %q", tableErrOut)
	}
	if !strings.Contains(tableOut, "obedience-corp/fest") {
		t.Fatalf("expected table output populated, got: %s", tableOut)
	}
}
