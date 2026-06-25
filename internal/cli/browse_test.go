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
	if jErr := json.Unmarshal([]byte(out), &res); jErr != nil {
		t.Fatalf("decode: %v\n%s", jErr, out)
	}
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
	if jErr := json.Unmarshal([]byte(out), &res); jErr != nil {
		t.Fatalf("decode: %v\n%s", jErr, out)
	}
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
