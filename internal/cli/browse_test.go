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
	if _, errOut, err := runInstaller(t, "marketplace", "add", repo, "--name", "existing-source"); err != nil {
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
	if _, errOut, err := runInstaller(t, "marketplace", "add", repo, "--name", "existing-source"); err != nil {
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
	if _, errOut, err := runInstaller(t, "marketplace", "add", repo, "--name", "existing-source"); err != nil {
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
//
// Since sequence 05, browse also verifies every registered source, and this
// fixture is an unsigned third-party source, so it now legitimately carries
// the loud "UNVERIFIED content" warning on stderr under PolicyWarnAllow: that
// is the intended behavior this festival adds, not a seed warning. This test
// distinguishes the two: the seed warning (envelope "warnings" field) must
// stay empty, while stderr carries only the unverified-content warning and
// never a seed-related message.
func TestBrowse_AlreadySeededNoSeedWarning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	repo := browseFixtureRepo(t)
	if _, errOut, err := runInstaller(t, "marketplace", "add", repo, "--name", "existing-source"); err != nil {
		t.Fatalf("marketplace add: %v\n%s", err, errOut)
	}

	jsonOut, jsonErrOut, err := runInstaller(t, "browse", "--json")
	if err != nil {
		t.Fatalf("browse --json: %v\n%s", err, jsonErrOut)
	}
	if !strings.Contains(jsonErrOut, "UNVERIFIED") {
		t.Fatalf("expected the unverified-content warning on stderr, got %q", jsonErrOut)
	}
	if strings.Contains(jsonErrOut, seedFriendlyWarning) {
		t.Fatalf("expected no seed warning on stderr, got %q", jsonErrOut)
	}
	var env struct {
		Warnings []string `json:"warnings"`
	}
	if jsonErr := json.Unmarshal([]byte(jsonOut), &env); jsonErr != nil {
		t.Fatalf("decode envelope: %v\n%s", jsonErr, jsonOut)
	}
	if len(env.Warnings) != 0 {
		t.Fatalf("expected no seed warnings in the envelope, got %v", env.Warnings)
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
	if !strings.Contains(tableErrOut, "UNVERIFIED") {
		t.Fatalf("expected the unverified-content warning on stderr in table mode, got %q", tableErrOut)
	}
	if strings.Contains(tableErrOut, seedFriendlyWarning) {
		t.Fatalf("expected no seed warning in table mode, got %q", tableErrOut)
	}
	if !strings.Contains(tableOut, "obedience-corp/fest") {
		t.Fatalf("expected table output populated, got: %s", tableOut)
	}
}

// Case 4 of the sequence 05 task 04 table: browse --json against an unsigned
// source registered under the official-policy name, with --allow-unverified,
// succeeds with clean JSON on stdout and no warning text leaking into it.
// This catches the classic bug where a warning is printed to stdout and
// breaks every JSON consumer.
func TestBrowse_AllowUnverifiedOfficialPolicySourceCleanStdout(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	repo := browseFixtureRepo(t)
	if _, errOut, err := runInstaller(t, "marketplace", "add", repo, "--name", "official-obey", "--allow-unverified"); err != nil {
		t.Fatalf("marketplace add: %v\n%s", err, errOut)
	}

	out, errOut, err := runInstaller(t, "browse", "--json", "--allow-unverified")
	if err != nil {
		t.Fatalf("browse --json --allow-unverified: %v\n%s", err, errOut)
	}
	if strings.Contains(out, "UNVERIFIED") || strings.Contains(out, "WARNING") {
		t.Fatalf("expected no warning text in stdout, got: %s", out)
	}
	var res browseJSON
	dataOf(t, out, &res)
	if len(res.Groups) == 0 {
		t.Fatalf("expected the catalog populated from the unverified official-policy source, got none: %s", out)
	}
	if errOut != "" && !strings.Contains(errOut, "UNVERIFIED") {
		t.Fatalf("expected only the unverified-content warning on stderr, if anything, got %q", errOut)
	}
}

// Case 6 (browse half): --help lists --allow-unverified.
func TestBrowse_HelpListsAllowUnverified(t *testing.T) {
	out, _, err := runInstaller(t, "browse", "--help")
	if err != nil {
		t.Fatalf("browse --help: %v", err)
	}
	if !strings.Contains(out, "--allow-unverified") {
		t.Fatalf("expected --allow-unverified in help output, got:\n%s", out)
	}
}
