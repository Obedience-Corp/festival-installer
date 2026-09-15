package cli_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	"github.com/Obedience-Corp/festival-installer/internal/state"
	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
)

type statusToolEntry struct {
	Tool           string `json:"tool"`
	Path           string `json:"path"`
	Managed        string `json:"managed"`
	Present        bool   `json:"present"`
	Version        string `json:"version"`
	Package        string `json:"package"`
	ReceiptVersion string `json:"receipt_version"`
	ReceiptChannel string `json:"receipt_channel"`
	Origin         string `json:"origin"`
	Error          string `json:"error"`
}

type statusPrerequisite struct {
	Name    string `json:"name"`
	Present bool   `json:"present"`
	Path    string `json:"path"`
}

type statusData struct {
	Home              string               `json:"home"`
	BinDir            string               `json:"bin_dir"`
	MarketplaceSource string               `json:"marketplace_source"`
	Origin            string               `json:"origin"`
	Tools             []statusToolEntry    `json:"tools"`
	Prerequisites     []statusPrerequisite `json:"prerequisites"`
}

// writeFakeTool writes an executable at binDir/name that prints a version for
// the arguments its probe uses, so a status probe has something real to run.
func writeFakeTool(t *testing.T, binDir, name, script string) {
	t.Helper()
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", binDir, err)
	}
	if err := os.WriteFile(filepath.Join(binDir, name), []byte(script), 0o755); err != nil { //nolint:gosec // an executable fixture is the point
		t.Fatalf("write fake %s: %v", name, err)
	}
}

func versionScript(out string) string { return "#!/bin/sh\necho '" + out + "'\n" }

// isolatedStatusEnv points the installer at home, isolates $HOME, and reduces
// PATH to a toolbox holding only the general-purpose binaries the fixtures need.
// No suite tool, obey, or ob can reach it, so every status answer comes from the
// fixture rather than from the developer machine.
func isolatedStatusEnv(t *testing.T, home string) {
	t.Helper()
	toolbox := t.TempDir()
	for _, name := range []string{"git", "sh"} {
		if p, err := exec.LookPath(name); err == nil {
			if err := os.Symlink(p, filepath.Join(toolbox, name)); err != nil {
				t.Fatalf("symlink %s into the test toolbox: %v", name, err)
			}
		}
	}
	t.Setenv("OBEY_INSTALLER_HOME", home)
	t.Setenv("FESTIVAL_HOME", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", toolbox)
}

func statusReport(t *testing.T, args ...string) statusData {
	t.Helper()
	out, errOut, err := runInstaller(t, args...)
	if err != nil {
		t.Fatalf("%v: %v\n%s", args, err, errOut)
	}
	var data statusData
	dataOf(t, out, &data)
	return data
}

func toolNamed(t *testing.T, data statusData, name string) statusToolEntry {
	t.Helper()
	for _, e := range data.Tools {
		if e.Tool == name {
			return e
		}
	}
	t.Fatalf("no %q entry in tools: %+v", name, data.Tools)
	return statusToolEntry{}
}

func TestStatus_EmptyHomeReportsEveryToolAbsentAndCreatesNothing(t *testing.T) {
	home := filepath.Join(t.TempDir(), "never-created")
	isolatedStatusEnv(t, home)

	data := statusReport(t, "status", "--json")

	want := []string{"camp", "fest", "festival", "obey", "ob"}
	if len(data.Tools) != len(want) {
		t.Fatalf("got %d tools, want %d: %+v", len(data.Tools), len(want), data.Tools)
	}
	for i, name := range want {
		got := data.Tools[i]
		if got.Tool != name {
			t.Fatalf("tools[%d] = %q, want %q (order is part of the contract)", i, got.Tool, name)
		}
		if got.Present {
			t.Fatalf("%s must be absent on an empty home: %+v", name, got)
		}
		if got.Origin != string(app.OriginAbsent) {
			t.Fatalf("%s origin = %q, want absent", name, got.Origin)
		}
		if got.Error == "" {
			t.Fatalf("%s must carry the reason it did not resolve: %+v", name, got)
		}
		if got.Version != "" {
			t.Fatalf("%s must report no version when it is absent: %+v", name, got)
		}
	}
	if data.Tools[0].Package != "obedience-corp/festival" || data.Tools[3].Package != app.ObeyPackageID {
		t.Fatalf("package ownership is reported even on an empty home: %+v", data.Tools)
	}
	if data.Home != home {
		t.Fatalf("home = %q, want %q", data.Home, home)
	}
	if data.BinDir != filepath.Join(home, "bin") {
		t.Fatalf("bin_dir = %q, want %q", data.BinDir, filepath.Join(home, "bin"))
	}
	if data.MarketplaceSource != "official-obey" {
		t.Fatalf("marketplace_source = %q, want official-obey", data.MarketplaceSource)
	}
	if len(data.Prerequisites) != 1 || data.Prerequisites[0].Name != "git" {
		t.Fatalf("prerequisites = %+v, want one git entry", data.Prerequisites)
	}

	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatalf("status must create nothing; stat %s err = %v", home, err)
	}
}

// installedSuiteHome installs the suite from the existing fixture so a real
// festival receipt exists, then writes an obey receipt beside it. It returns the
// installer home and the managed bin dir.
func installedSuiteHome(t *testing.T) (string, string) {
	t.Helper()
	ctx := context.Background()
	home := t.TempDir()
	isolatedStatusEnv(t, home)

	tarball := buildSuiteTarGz(t, map[string]string{
		"camp":     "#!/bin/sh\necho camp\n",
		"fest":     "#!/bin/sh\necho fest\n",
		"festival": "#!/bin/sh\necho festival\n",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tarball)
	}))
	t.Cleanup(srv.Close)

	repo := fixtureInstallMarketplace(t, srv.URL+"/festival.tar.gz", sha256Hex(tarball))
	if _, errOut, err := runInstaller(t, "marketplace", "add", repo, "--name", "official-obey", "--allow-unverified"); err != nil {
		t.Fatalf("marketplace add: %v\n%s", err, errOut)
	}
	if _, errOut, err := runInstaller(t, "install", "festival", "--allow-unverified", "--json"); err != nil {
		t.Fatalf("install festival: %v\n%s", err, errOut)
	}

	binDir, err := state.BinDir(ctx)
	if err != nil {
		t.Fatalf("BinDir: %v", err)
	}
	writeObeyReceipt(t, ctx, home, binDir)
	return home, binDir
}

func writeObeyReceipt(t *testing.T, ctx context.Context, home, binDir string) {
	t.Helper()
	rec := receipts.Receipt{
		PackageID:   app.ObeyPackageID,
		Version:     "0.2.0",
		Source:      "official-obey",
		Channel:     "stable",
		InstalledAt: time.Now().UTC(),
		OwnedFiles: []receipts.OwnedFile{
			{Path: filepath.Join(binDir, "obey"), Hash: sha256Hex([]byte("obey")), Mode: 0o755},
			{Path: filepath.Join(binDir, "ob"), Hash: sha256Hex([]byte("ob")), Mode: 0o755},
		},
	}
	if err := receipts.Write(ctx, mustDB(t, ctx, home), rec); err != nil {
		t.Fatalf("write obey receipt: %v", err)
	}
}

func TestStatus_ReportsVersionsAndReceipts(t *testing.T) {
	_, binDir := installedSuiteHome(t)

	writeFakeTool(t, binDir, "camp", versionScript("0.6.0"))
	writeFakeTool(t, binDir, "fest", versionScript("0.6.6"))
	writeFakeTool(t, binDir, "festival", versionScript("0.2.2"))
	writeFakeTool(t, binDir, "obey", versionScript("obey version 0.2.0"))
	writeFakeTool(t, binDir, "ob", versionScript("ob version 0.2.0"))

	data := statusReport(t, "status", "--json")

	if data.BinDir != binDir {
		t.Fatalf("bin_dir = %q, want %q", data.BinDir, binDir)
	}
	if data.MarketplaceSource != "official-obey" {
		t.Fatalf("marketplace_source = %q, want official-obey", data.MarketplaceSource)
	}

	want := map[string]struct {
		version, pkg, receiptVersion string
	}{
		"camp":     {"0.6.0", "obedience-corp/festival", "0.2.10"},
		"fest":     {"0.6.6", "obedience-corp/festival", "0.2.10"},
		"festival": {"0.2.2", "obedience-corp/festival", "0.2.10"},
		"obey":     {"0.2.0", app.ObeyPackageID, "0.2.0"},
		"ob":       {"0.2.0", app.ObeyPackageID, "0.2.0"},
	}
	for name, w := range want {
		got := toolNamed(t, data, name)
		if !got.Present || got.Path != filepath.Join(binDir, name) {
			t.Fatalf("%s should resolve to the managed path: %+v", name, got)
		}
		if got.Managed != filepath.Join(binDir, name) {
			t.Fatalf("%s managed = %q, want %q", name, got.Managed, filepath.Join(binDir, name))
		}
		if got.Origin != string(app.OriginManaged) {
			t.Fatalf("%s origin = %q, want managed", name, got.Origin)
		}
		if got.Version != w.version {
			t.Fatalf("%s version = %q, want %q (error %q)", name, got.Version, w.version, got.Error)
		}
		if got.Package != w.pkg {
			t.Fatalf("%s package = %q, want %q", name, got.Package, w.pkg)
		}
		if got.ReceiptVersion != w.receiptVersion || got.ReceiptChannel != "stable" {
			t.Fatalf("%s receipt = %q/%q, want %q/stable", name, got.ReceiptVersion, got.ReceiptChannel, w.receiptVersion)
		}
		if got.Error != "" {
			t.Fatalf("%s must report no error when the probe succeeded: %q", name, got.Error)
		}
	}
}

func TestStatus_ObeyProbeFallsBackToVersionFlag(t *testing.T) {
	cases := map[string]struct {
		script string
		want   string
	}{
		"root flag only": {
			script: "#!/bin/sh\ncase \"$1\" in\n  --version) echo \"obey version 0.2.0\" ;;\n  *) exit 1 ;;\nesac\n",
			want:   "0.2.0",
		},
		"subcommand wins when both answer": {
			script: "#!/bin/sh\ncase \"$1\" in\n  version) echo 0.3.0 ;;\n  *) echo \"obey version 0.2.0\" ;;\nesac\n",
			want:   "0.3.0",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			isolatedStatusEnv(t, home)
			binDir := filepath.Join(home, "bin")
			writeFakeTool(t, binDir, "obey", tc.script)

			got := toolNamed(t, statusReport(t, "status", "--json"), "obey")
			if got.Version != tc.want {
				t.Fatalf("obey version = %q, want %q (error %q)", got.Version, tc.want, got.Error)
			}
			if got.Error != "" {
				t.Fatalf("obey must report no error when a probe succeeded: %q", got.Error)
			}
		})
	}
}

func TestStatus_UnparseableVersionIsReportedNotFailed(t *testing.T) {
	home := t.TempDir()
	isolatedStatusEnv(t, home)
	binDir := filepath.Join(home, "bin")
	writeFakeTool(t, binDir, "ob", "#!/bin/sh\necho 'Usage: ob [command]'\necho 'Run ob help for more.'\n")

	out, errOut, err := runInstaller(t, "status", "--json")
	if err != nil {
		t.Fatalf("a failed probe must not fail the command: %v\n%s", err, errOut)
	}
	var data statusData
	dataOf(t, out, &data)

	got := toolNamed(t, data, "ob")
	if !got.Present {
		t.Fatalf("ob resolved and must be reported present: %+v", got)
	}
	if got.Version != "" {
		t.Fatalf("ob must report no version when its output did not parse: %+v", got)
	}
	if !strings.Contains(got.Error, "unparseable") {
		t.Fatalf("ob error = %q, want it to name the unparseable output", got.Error)
	}
	if strings.Contains(out, `"version"`) {
		t.Fatalf("the version key must be omitted when empty:\n%s", out)
	}
}

func TestStatus_StdoutIsExactlyOneJSONObject(t *testing.T) {
	home := t.TempDir()
	isolatedStatusEnv(t, home)
	binDir := filepath.Join(home, "bin")
	writeFakeTool(t, binDir, "camp", versionScript("0.6.0"))

	out, _, err := runInstaller(t, "status", "--json")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	dec := json.NewDecoder(strings.NewReader(out))
	var first map[string]any
	if err := dec.Decode(&first); err != nil {
		t.Fatalf("decode the one status object: %v\n%s", err, out)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		t.Fatalf("stdout carried more than one JSON value (err %v, extra %v):\n%s", err, extra, out)
	}
}

func TestResolve_ManagedToolAndMissingTool(t *testing.T) {
	home := t.TempDir()
	isolatedStatusEnv(t, home)
	binDir := filepath.Join(home, "bin")
	writeFakeTool(t, binDir, "camp", versionScript("0.6.0"))

	out, errOut, err := runInstaller(t, "resolve", "camp", "--json")
	if err != nil {
		t.Fatalf("resolve camp: %v\n%s", err, errOut)
	}
	var res app.ResolveResult
	dataOf(t, out, &res)
	if res.Tool != "camp" || res.Path != filepath.Join(binDir, "camp") {
		t.Fatalf("resolve camp = %+v, want the managed path %s", res, filepath.Join(binDir, "camp"))
	}

	out, _, err = runInstaller(t, "resolve", "nosuchtool", "--json")
	if err == nil {
		t.Fatal("expected resolve of a missing tool to fail")
	}
	if !hasErrorCode(err, "E_LAUNCH_NOT_FOUND") {
		t.Fatalf("expected E_LAUNCH_NOT_FOUND, got %v", err)
	}
	var envelope struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if jerr := json.Unmarshal([]byte(out), &envelope); jerr != nil {
		t.Fatalf("decode failure envelope: %v\n%s", jerr, out)
	}
	if envelope.OK || envelope.Error.Code != "E_LAUNCH_NOT_FOUND" {
		t.Fatalf("expected a failure envelope carrying E_LAUNCH_NOT_FOUND, got:\n%s", out)
	}
}

func TestResolve_RejectsEmptyAndPathSeparator(t *testing.T) {
	home := t.TempDir()
	isolatedStatusEnv(t, home)

	for _, tool := range []string{"", "../bin/sh"} {
		_, _, err := runInstaller(t, "resolve", tool, "--json")
		if err == nil {
			t.Fatalf("expected resolve %q to be rejected", tool)
		}
		if !hasErrorCode(err, "E_LAUNCH_TOOL") {
			t.Fatalf("resolve %q: expected E_LAUNCH_TOOL, got %v", tool, err)
		}
	}
}

func TestResolve_BarePathWithoutJSON(t *testing.T) {
	home := t.TempDir()
	isolatedStatusEnv(t, home)
	binDir := filepath.Join(home, "bin")
	writeFakeTool(t, binDir, "camp", versionScript("0.6.0"))

	out, errOut, err := runInstaller(t, "resolve", "camp")
	if err != nil {
		t.Fatalf("resolve camp: %v\n%s", err, errOut)
	}
	if out != filepath.Join(binDir, "camp")+"\n" {
		t.Fatalf("stdout = %q, want exactly the path plus a newline", out)
	}
	if errOut != "" {
		t.Fatalf("stderr = %q, want nothing", errOut)
	}
}
