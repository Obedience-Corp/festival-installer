package cli_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/state"
)

// recordingObeyScript is an obey stand-in that appends every service verb it
// was called with to a log file and exits 0. It answers the version probe from
// its own arguments so a version query never lands in the log, and it records
// whether its own file is still on disk, which is how the uninstall ordering is
// proven rather than assumed.
const recordingObeyScript = `#!/bin/sh
case "$1" in
version|--version) echo %s; exit 0;;
esac
if [ -x "$0" ]; then present=yes; else present=no; fi
echo "$* present=$present" >> %s
exit 0
`

// failingObeyScript refuses every service verb the way a sandboxed launchd
// does, with the message on stderr and a nonzero exit.
const failingObeyScript = `#!/bin/sh
echo 'launchctl bootstrap gui/501: Operation not permitted' >&2
exit 1
`

func recordingObey(version, logPath string) string {
	return fmt.Sprintf(recordingObeyScript, version, logPath)
}

// obeyTarballWith publishes an obey archive whose obey entry is body and whose
// ob entry is the ordinary version echo, so a test can swap the daemon for a
// fake without losing the second binary.
func obeyTarballWith(t *testing.T, version, body string) []byte {
	t.Helper()
	return buildSuiteTarGz(t, map[string]string{
		"obey":      body,
		"ob":        obBodyPrefix + version + "\n",
		"README.md": "obey " + version,
	})
}

// obeyServiceFixture is obeyFixture with the daemon replaced by a fake. It
// returns the release repository, the release host, and the service log path.
// The log lives in its own temp dir so uninstall never removes it.
func obeyServiceFixture(t *testing.T, version string) (string, *obeyReleaseHost, string) {
	t.Helper()
	_, releaseRepo, host := obeyFixture(t, version)
	logPath := filepath.Join(t.TempDir(), "service.log")
	host.publish(version, obeyTarballWith(t, version, recordingObey(version, logPath)))
	return releaseRepo, host, logPath
}

func readServiceLog(t *testing.T, logPath string) []string {
	t.Helper()
	raw, err := os.ReadFile(logPath)
	if err != nil {
		return nil
	}
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

// serviceVerbs drops the present= marker so a caller can compare the verbs
// alone; the marker itself is asserted where the ordering matters.
func serviceVerbs(lines []string) []string {
	verbs := make([]string, 0, len(lines))
	for _, l := range lines {
		if i := strings.Index(l, " present="); i >= 0 {
			l = l[:i]
		}
		verbs = append(verbs, l)
	}
	return verbs
}

type obeyServicePayload struct {
	Installed bool   `json:"installed"`
	Restarted bool   `json:"restarted"`
	Deferred  bool   `json:"deferred"`
	Error     string `json:"error"`
}

type obeyEnvelope struct {
	OK       bool            `json:"ok"`
	Warnings []string        `json:"warnings"`
	Error    json.RawMessage `json:"error"`
	Data     json.RawMessage `json:"data"`
}

func envelopeOf(t *testing.T, out string) obeyEnvelope {
	t.Helper()
	var env obeyEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("decode envelope: %v\n%s", err, out)
	}
	return env
}

func TestInstallObey_RunsServiceInstall(t *testing.T) {
	_, _, logPath := obeyServiceFixture(t, "0.2.0")

	out, errOut, err := runInstaller(t, "install", "obey", "--allow-unverified", "--json")
	if err != nil {
		t.Fatalf("install obey: %v\n%s", err, errOut)
	}

	var res struct {
		Service *obeyServicePayload `json:"service"`
	}
	dataOf(t, out, &res)
	if res.Service == nil || !res.Service.Installed {
		t.Fatalf("expected data.service.installed true, got %+v\n%s", res.Service, out)
	}
	if res.Service.Error != "" {
		t.Fatalf("a successful service install must carry no error, got %q", res.Service.Error)
	}
	if strings.Contains(out, `"error"`) {
		t.Fatalf("a clean install payload must carry no error key:\n%s", out)
	}

	if got := serviceVerbs(readServiceLog(t, logPath)); len(got) != 1 || got[0] != "service install" {
		t.Fatalf("service log = %v, want exactly [\"service install\"]", got)
	}
}

func TestInstallObey_ServiceFailureIsAWarningNotAFailure(t *testing.T) {
	_, _, host := obeyFixture(t, "0.2.0")
	host.publish("0.2.0", obeyTarballWith(t, "0.2.0", failingObeyScript))

	out, errOut, err := runInstaller(t, "install", "obey", "--allow-unverified", "--json")
	if err != nil {
		t.Fatalf("a service failure must not fail the install: %v\n%s", err, errOut)
	}

	env := envelopeOf(t, out)
	if !env.OK {
		t.Fatalf("ok must stay true when only the service step failed:\n%s", out)
	}
	if len(env.Error) != 0 {
		t.Fatalf("the envelope must carry no error:\n%s", out)
	}
	if len(env.Warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly one entry", env.Warnings)
	}
	if !strings.Contains(env.Warnings[0], "The binaries are installed") {
		t.Fatalf("warning = %q, want it to say the binaries are installed", env.Warnings[0])
	}

	var res struct {
		Files   []string            `json:"files"`
		Service *obeyServicePayload `json:"service"`
	}
	dataOf(t, out, &res)
	if res.Service == nil || res.Service.Installed {
		t.Fatalf("expected data.service.installed false, got %+v", res.Service)
	}
	if !strings.Contains(res.Service.Error, "Operation not permitted") {
		t.Fatalf("data.service.error = %q, want the supervisor's message", res.Service.Error)
	}
	if !strings.Contains(errOut, "install: ") {
		t.Fatalf("the human path must print the same sentence to stderr, got %q", errOut)
	}

	binDir, err := state.BinDir(context.Background())
	if err != nil {
		t.Fatalf("BinDir: %v", err)
	}
	for _, name := range []string{"obey", "ob"} {
		if _, statErr := os.Stat(filepath.Join(binDir, name)); statErr != nil {
			t.Fatalf("%s must be on disk after a service failure: %v", name, statErr)
		}
	}
	if len(res.Files) != 2 {
		t.Fatalf("expected both binaries reported, got %v", res.Files)
	}
}

// publishObeyUpgrade tags and publishes a newer obey whose fake records to the
// same log, so an update's service verb lands beside the install's.
func publishObeyUpgrade(t *testing.T, releaseRepo string, host *obeyReleaseHost, version, logPath string) {
	t.Helper()
	host.publish(version, obeyTarballWith(t, version, recordingObey(version, logPath)))
	git(t, releaseRepo, "tag", "v"+version)
}

func TestUpdateObey_RestartsByDefault(t *testing.T) {
	releaseRepo, host, logPath := obeyServiceFixture(t, "0.2.0")
	if _, errOut, err := runInstaller(t, "install", "obey", "--allow-unverified", "--json"); err != nil {
		t.Fatalf("install obey: %v\n%s", err, errOut)
	}
	publishObeyUpgrade(t, releaseRepo, host, "0.2.1", logPath)

	out, errOut, err := runInstaller(t, "update", "obey", "--allow-unverified", "--json")
	if err != nil {
		t.Fatalf("update obey: %v\n%s", err, errOut)
	}

	var res struct {
		Action  string              `json:"action"`
		Service *obeyServicePayload `json:"service"`
	}
	dataOf(t, out, &res)
	if res.Action != "upgraded" {
		t.Fatalf("action = %q, want upgraded", res.Action)
	}
	if res.Service == nil || !res.Service.Restarted || res.Service.Deferred {
		t.Fatalf("expected data.service.restarted true and deferred false, got %+v", res.Service)
	}

	verbs := serviceVerbs(readServiceLog(t, logPath))
	if len(verbs) == 0 || verbs[len(verbs)-1] != "service restart" {
		t.Fatalf("service log = %v, want it to end with \"service restart\"", verbs)
	}
}

func TestUpdateObey_NoRestartDefers(t *testing.T) {
	releaseRepo, host, logPath := obeyServiceFixture(t, "0.2.0")
	if _, errOut, err := runInstaller(t, "install", "obey", "--allow-unverified", "--json"); err != nil {
		t.Fatalf("install obey: %v\n%s", err, errOut)
	}
	publishObeyUpgrade(t, releaseRepo, host, "0.2.1", logPath)

	out, errOut, err := runInstaller(t, "update", "obey", "--allow-unverified", "--no-restart", "--json")
	if err != nil {
		t.Fatalf("update obey --no-restart: %v\n%s", err, errOut)
	}

	var res struct {
		Action  string              `json:"action"`
		Version string              `json:"version"`
		Service *obeyServicePayload `json:"service"`
	}
	dataOf(t, out, &res)
	if res.Action != "upgraded" || res.Version != "0.2.1" {
		t.Fatalf("expected upgraded to 0.2.1, got %+v", res)
	}
	if res.Service == nil || !res.Service.Deferred || res.Service.Restarted {
		t.Fatalf("expected data.service.deferred true and restarted false, got %+v", res.Service)
	}

	verbs := serviceVerbs(readServiceLog(t, logPath))
	for _, v := range verbs {
		if v == "service restart" {
			t.Fatalf("--no-restart must run no restart, log = %v", verbs)
		}
	}

	env := envelopeOf(t, out)
	if len(env.Warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly one entry", env.Warnings)
	}
	if !strings.Contains(env.Warnings[0], "obey service restart") {
		t.Fatalf("warning = %q, want it to name the restart command", env.Warnings[0])
	}
}

func TestUpdateObey_CurrentRunsNoServiceVerb(t *testing.T) {
	_, _, logPath := obeyServiceFixture(t, "0.2.0")
	if _, errOut, err := runInstaller(t, "install", "obey", "--allow-unverified", "--json"); err != nil {
		t.Fatalf("install obey: %v\n%s", err, errOut)
	}
	afterInstall := serviceVerbs(readServiceLog(t, logPath))

	out, errOut, err := runInstaller(t, "update", "obey", "--allow-unverified", "--json")
	if err != nil {
		t.Fatalf("update obey: %v\n%s", err, errOut)
	}

	var res struct {
		Action  string              `json:"action"`
		Service *obeyServicePayload `json:"service"`
	}
	dataOf(t, out, &res)
	if res.Action != "current" {
		t.Fatalf("action = %q, want current", res.Action)
	}
	if res.Service != nil {
		t.Fatalf("a current update changed nothing on disk and must carry no service object, got %+v", res.Service)
	}
	if strings.Contains(out, `"service"`) {
		t.Fatalf("a current update payload must carry no service key:\n%s", out)
	}

	if got := serviceVerbs(readServiceLog(t, logPath)); len(got) != len(afterInstall) {
		t.Fatalf("service log grew from %v to %v on a current update", afterInstall, got)
	}
}

func TestUninstallObey_UnregistersBeforeRemoving(t *testing.T) {
	_, _, logPath := obeyServiceFixture(t, "0.2.0")
	if _, errOut, err := runInstaller(t, "install", "obey", "--allow-unverified", "--json"); err != nil {
		t.Fatalf("install obey: %v\n%s", err, errOut)
	}

	if _, errOut, err := runInstaller(t, "uninstall", "obey", "--json"); err != nil {
		t.Fatalf("uninstall obey: %v\n%s", err, errOut)
	}

	lines := readServiceLog(t, logPath)
	want := []string{"service install", "service uninstall"}
	got := serviceVerbs(lines)
	if len(got) != len(want) {
		t.Fatalf("service log = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("service log = %v, want %v", got, want)
		}
	}
	if !strings.Contains(lines[1], "present=yes") {
		t.Fatalf("uninstall ran after the binaries were removed: %q", lines[1])
	}

	binDir, err := state.BinDir(context.Background())
	if err != nil {
		t.Fatalf("BinDir: %v", err)
	}
	for _, name := range []string{"obey", "ob"} {
		if _, statErr := os.Stat(filepath.Join(binDir, name)); !os.IsNotExist(statErr) {
			t.Fatalf("%s should be removed, stat err=%v", name, statErr)
		}
	}
}
