package cli_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/state"
)

// obeyHelpFunctions is the part of every obey stand-in that answers the
// contract probe: the two cobra help screens the installer reads to decide
// whether this obey supervises its daemon. __RESTART_VERB__ and
// __RESTART_FLAG__ are filled in for an obey carrying the supervised contract
// and left empty for one carrying obey main's service family.
const obeyHelpFunctions = `service_help() {
  echo "Usage:"
  echo "  obey service [command]"
  echo ""
  echo "Available Commands:"
  echo "  install     Install the daemon service"
__RESTART_VERB__  echo "  status      Report whether the daemon service is installed"
  echo "  uninstall   Stop the daemon service and remove its unit file"
}
install_help() {
  echo "Usage:"
  echo "  obey service install [flags]"
  echo ""
  echo "Flags:"
__RESTART_FLAG__  echo "  -h, --help   help for install"
}
answer_help() {
  if [ "$1" = "service" ] && [ "$2" = "--help" ]; then service_help; exit 0; fi
  if [ "$1" = "service" ] && [ "$3" = "--help" ]; then install_help; exit 0; fi
}
`

// recordingObeyScript is an obey stand-in that appends every service verb it
// was called with to a log file and exits 0. It answers the version probe and
// the contract probe from its own arguments so neither lands in the log, and it
// records whether its own file is still on disk, which is how the uninstall
// ordering is proven rather than assumed.
//
// `service status --json` answers from marker files the test controls and is
// deliberately kept out of the log: it is the read the installer makes to
// decide whether a daemon is up, not a verb that changes anything, and the log
// is what the verb assertions compare against. A second marker makes that read
// fail, which is the machine where the daemon state cannot be established.
const recordingObeyScript = `#!/bin/sh
__HELP_FUNCTIONS__
case "$1" in
version|--version) echo __VERSION__; exit 0;;
esac
answer_help "$@"
if [ "$1" = "service" ] && [ "$2" = "status" ]; then
  if [ -f "__BROKEN_MARKER__" ]; then
    echo "load daemon config: unexpected end of JSON input" >&2
    exit 1
  fi
  if [ -f "__RUNNING_MARKER__" ]; then
    echo '{"installed":true,"loaded":true,"socket_reachable":true,"daemon_pid":4242}'
  else
    echo '{"installed":false,"loaded":false,"socket_reachable":false}'
  fi
  exit 0
fi
if [ -x "$0" ]; then present=yes; else present=no; fi
echo "$* present=$present" >> __LOG_PATH__
exit 0
`

// failingObeyScript refuses every service verb the way a sandboxed launchd
// does, with the message on stderr and a nonzero exit. It still answers the
// contract probe, because --help never reaches launchctl: a supervisor that
// refuses to bootstrap says nothing about which verbs obey has.
const failingObeyScript = `#!/bin/sh
__HELP_FUNCTIONS__
case "$1" in
version|--version) echo 0.2.0; exit 0;;
esac
answer_help "$@"
echo 'launchctl bootstrap gui/501: Operation not permitted' >&2
exit 1
`

// helpFunctions renders the contract probe's answers. supervised chooses
// between the verb set obey #381 adds and the one obey main has.
func helpFunctions(supervised bool) string {
	verb, flag := "", ""
	if supervised {
		verb = `  echo "  restart     Restart the supervised daemon in place"` + "\n"
		flag = `  echo "      --restart   restart a daemon that is already running"` + "\n"
	}
	return strings.NewReplacer("__RESTART_VERB__", verb, "__RESTART_FLAG__", flag).Replace(obeyHelpFunctions)
}

func recordingObey(version, runningMarker, brokenMarker, logPath string, supervised bool) string {
	return strings.NewReplacer(
		"__HELP_FUNCTIONS__", helpFunctions(supervised),
		"__VERSION__", version,
		"__RUNNING_MARKER__", runningMarker,
		"__BROKEN_MARKER__", brokenMarker,
		"__LOG_PATH__", logPath,
	).Replace(recordingObeyScript)
}

// failingObey is failingObeyScript with the contract answers filled in.
func failingObey() string {
	return strings.ReplaceAll(failingObeyScript, "__HELP_FUNCTIONS__", helpFunctions(true))
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

// obeyServiceFake is obeyFixture with the daemon replaced by a fake: the same
// isolated home, release repository and release host, plus the two paths the
// fake reads and writes. Both live in their own temp dir, so uninstall never
// removes them.
type obeyServiceFake struct {
	home          string
	releaseRepo   string
	host          *obeyReleaseHost
	logPath       string
	runningMarker string
	brokenMarker  string
	supervised    bool
}

func obeyServiceFixture(t *testing.T, version string) obeyServiceFake {
	t.Helper()
	return newObeyServiceFixture(t, version, true)
}

// obeyLegacyServiceFixture stages an obey carrying obey main's service family:
// install, status and uninstall, with no restart verb and an install that
// replaces a running daemon on its own.
func obeyLegacyServiceFixture(t *testing.T, version string) obeyServiceFake {
	t.Helper()
	return newObeyServiceFixture(t, version, false)
}

func newObeyServiceFixture(t *testing.T, version string, supervised bool) obeyServiceFake {
	t.Helper()
	home, releaseRepo, host := obeyFixture(t, version)
	scratch := t.TempDir()
	f := obeyServiceFake{
		home:          home,
		releaseRepo:   releaseRepo,
		host:          host,
		logPath:       filepath.Join(scratch, "service.log"),
		runningMarker: filepath.Join(scratch, "daemon.running"),
		brokenMarker:  filepath.Join(scratch, "daemon.unreadable"),
		supervised:    supervised,
	}
	host.publish(version, obeyTarballWith(t, version, f.body(version)))
	return f
}

func (f obeyServiceFake) body(version string) string {
	return recordingObey(version, f.runningMarker, f.brokenMarker, f.logPath, f.supervised)
}

// markDaemonRunning makes the installed fake answer `service status` the way an
// obey with a live daemon does.
func (f obeyServiceFake) markDaemonRunning(t *testing.T) {
	t.Helper()
	writeFile(t, f.runningMarker, "running\n")
}

// breakDaemonProbe makes `service status --json` fail the way an obey that
// cannot load its config does, so the daemon state cannot be established.
func (f obeyServiceFake) breakDaemonProbe(t *testing.T) {
	t.Helper()
	writeFile(t, f.brokenMarker, "unreadable\n")
}

func (f obeyServiceFake) verbs(t *testing.T) []string {
	t.Helper()
	return serviceVerbs(readServiceLog(t, f.logPath))
}

func wantVerbs(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("service log = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("service log = %v, want %v", got, want)
		}
	}
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
	Installed          bool   `json:"installed"`
	Restarted          bool   `json:"restarted"`
	Deferred           bool   `json:"deferred"`
	Started            bool   `json:"started"`
	Error              string `json:"error"`
	DaemonStateUnknown bool   `json:"daemon_state_unknown"`
	Unsupported        bool   `json:"unsupported"`
	ContractReason     string `json:"contract_reason"`
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

// A fresh machine has no daemon to preserve, so installing the unit is the
// whole job: the supervisor starts it. Nothing restarts, nothing is deferred,
// and the user is told nothing, because nothing is pending.
func TestInstallObey_FreshInstallRunsInstallOnly(t *testing.T) {
	f := obeyServiceFixture(t, "0.2.0")

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
	if res.Service.Restarted || res.Service.Deferred {
		t.Fatalf("a fresh install has no daemon to restart or defer, got %+v", res.Service)
	}
	if res.Service.Error != "" {
		t.Fatalf("a successful service install must carry no error, got %q", res.Service.Error)
	}
	if strings.Contains(out, `"error"`) {
		t.Fatalf("a clean install payload must carry no error key:\n%s", out)
	}
	if env := envelopeOf(t, out); len(env.Warnings) != 0 {
		t.Fatalf("a fresh install has nothing pending to warn about, got %v", env.Warnings)
	}

	wantVerbs(t, f.verbs(t), []string{"service install"})
}

// Installing over a machine that already has a daemon up replaces the binaries
// under a running process. `obey service install` refreshes the unit and leaves
// that process alone, so without the restart the user would be told obey was
// installed while the daemon kept serving the previous image.
func TestInstallObey_OverRunningDaemonRestartsIt(t *testing.T) {
	f := obeyServiceFixture(t, "0.2.0")
	if _, errOut, err := runInstaller(t, "install", "obey", "--allow-unverified", "--json"); err != nil {
		t.Fatalf("first install: %v\n%s", err, errOut)
	}
	f.markDaemonRunning(t)

	out, errOut, err := runInstaller(t, "install", "obey", "--allow-unverified", "--json")
	if err != nil {
		t.Fatalf("second install: %v\n%s", err, errOut)
	}

	var res struct {
		Service *obeyServicePayload `json:"service"`
	}
	dataOf(t, out, &res)
	if res.Service == nil || !res.Service.Installed || !res.Service.Restarted {
		t.Fatalf("expected installed and restarted true, got %+v\n%s", res.Service, out)
	}
	if res.Service.Deferred || res.Service.Error != "" {
		t.Fatalf("a completed restart must not be deferred or carry an error, got %+v", res.Service)
	}

	wantVerbs(t, f.verbs(t), []string{"service install", "service install", "service restart"})
}

// --no-restart is the flag a caller with live sessions reaches for. The
// binaries land, the daemon is left alone, and the pending restart is reported
// rather than silently skipped.
func TestInstallObey_OverRunningDaemonNoRestartReportsPending(t *testing.T) {
	f := obeyServiceFixture(t, "0.2.0")
	if _, errOut, err := runInstaller(t, "install", "obey", "--allow-unverified", "--json"); err != nil {
		t.Fatalf("first install: %v\n%s", err, errOut)
	}
	f.markDaemonRunning(t)

	out, errOut, err := runInstaller(t, "install", "obey", "--allow-unverified", "--no-restart", "--json")
	if err != nil {
		t.Fatalf("install obey --no-restart: %v\n%s", err, errOut)
	}

	var res struct {
		Service *obeyServicePayload `json:"service"`
	}
	dataOf(t, out, &res)
	if res.Service == nil || !res.Service.Installed || !res.Service.Deferred {
		t.Fatalf("expected installed and deferred true, got %+v\n%s", res.Service, out)
	}
	if res.Service.Restarted {
		t.Fatalf("--no-restart must not restart, got %+v", res.Service)
	}

	env := envelopeOf(t, out)
	if len(env.Warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly one entry", env.Warnings)
	}
	if !strings.Contains(env.Warnings[0], "obey service restart") {
		t.Fatalf("warning = %q, want it to name the restart command", env.Warnings[0])
	}
	if !strings.Contains(errOut, "obey service restart") {
		t.Fatalf("the human path must name the pending restart on stderr, got %q", errOut)
	}

	wantVerbs(t, f.verbs(t), []string{"service install", "service install"})
}

func TestInstallObey_ServiceFailureIsAWarningNotAFailure(t *testing.T) {
	_, _, host := obeyFixture(t, "0.2.0")
	host.publish("0.2.0", obeyTarballWith(t, "0.2.0", failingObey()))

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
func (f obeyServiceFake) publishUpgrade(t *testing.T, version string) {
	t.Helper()
	f.host.publish(version, obeyTarballWith(t, version, f.body(version)))
	git(t, f.releaseRepo, "tag", "v"+version)
}

func TestUpdateObey_RestartsByDefault(t *testing.T) {
	f := obeyServiceFixture(t, "0.2.0")
	if _, errOut, err := runInstaller(t, "install", "obey", "--allow-unverified", "--json"); err != nil {
		t.Fatalf("install obey: %v\n%s", err, errOut)
	}
	f.markDaemonRunning(t)
	f.publishUpgrade(t, "0.2.1")

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
	if res.Service == nil || !res.Service.Restarted || res.Service.Deferred || res.Service.Started {
		t.Fatalf("expected data.service.restarted true, deferred and started false, got %+v", res.Service)
	}

	wantVerbs(t, f.verbs(t), []string{"service install", "service install", "service restart"})
}

// A machine whose daemon is stopped has nothing to restart. The unit refresh is
// what brings it up on the new binary, and the caller is told the daemon it
// left stopped is now running.
func TestUpdateObey_StoppedDaemonIsStartedNotRestarted(t *testing.T) {
	f := obeyServiceFixture(t, "0.2.0")
	if _, errOut, err := runInstaller(t, "install", "obey", "--allow-unverified", "--json"); err != nil {
		t.Fatalf("install obey: %v\n%s", err, errOut)
	}
	f.publishUpgrade(t, "0.2.1")

	out, errOut, err := runInstaller(t, "update", "obey", "--allow-unverified", "--json")
	if err != nil {
		t.Fatalf("update obey: %v\n%s", err, errOut)
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
	if res.Service == nil || !res.Service.Installed || !res.Service.Started {
		t.Fatalf("expected data.service.installed and started true, got %+v\n%s", res.Service, out)
	}
	if res.Service.Restarted || res.Service.Deferred {
		t.Fatalf("a stopped daemon is neither restarted nor deferred, got %+v", res.Service)
	}

	for _, v := range f.verbs(t) {
		if v == "service restart" {
			t.Fatalf("a stopped daemon must not be restarted, log = %v", f.verbs(t))
		}
	}
	wantVerbs(t, f.verbs(t), []string{"service install", "service install"})

	env := envelopeOf(t, out)
	if len(env.Warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly one entry", env.Warnings)
	}
	if !strings.Contains(env.Warnings[0], "started on the new version") {
		t.Fatalf("warning = %q, want it to say the daemon was started rather than restarted", env.Warnings[0])
	}
}

func TestUpdateObey_NoRestartDefers(t *testing.T) {
	f := obeyServiceFixture(t, "0.2.0")
	if _, errOut, err := runInstaller(t, "install", "obey", "--allow-unverified", "--json"); err != nil {
		t.Fatalf("install obey: %v\n%s", err, errOut)
	}
	f.markDaemonRunning(t)
	f.publishUpgrade(t, "0.2.1")

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
	if res.Service == nil || !res.Service.Deferred || res.Service.Restarted || res.Service.Started {
		t.Fatalf("expected data.service.deferred true, restarted and started false, got %+v", res.Service)
	}

	verbs := f.verbs(t)
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
	f := obeyServiceFixture(t, "0.2.0")
	if _, errOut, err := runInstaller(t, "install", "obey", "--allow-unverified", "--json"); err != nil {
		t.Fatalf("install obey: %v\n%s", err, errOut)
	}
	afterInstall := f.verbs(t)

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

	if got := f.verbs(t); len(got) != len(afterInstall) {
		t.Fatalf("service log grew from %v to %v on a current update", afterInstall, got)
	}
}

func TestUninstallObey_UnregistersBeforeRemoving(t *testing.T) {
	f := obeyServiceFixture(t, "0.2.0")
	if _, errOut, err := runInstaller(t, "install", "obey", "--allow-unverified", "--json"); err != nil {
		t.Fatalf("install obey: %v\n%s", err, errOut)
	}

	if _, errOut, err := runInstaller(t, "uninstall", "obey", "--json"); err != nil {
		t.Fatalf("uninstall obey: %v\n%s", err, errOut)
	}

	lines := readServiceLog(t, f.logPath)
	wantVerbs(t, serviceVerbs(lines), []string{"service install", "service uninstall"})
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

// An obey in the managed bin dir with no receipt is not festival's to take
// apart. Uninstall reports that it owns nothing, and it must not have stopped
// the daemon or removed its unit on the way to saying so: a reset state.db, a
// deleted receipt, or an obey placed there by another route all reach this,
// and the daemon would not come back at login.
func TestUninstallObey_NoReceiptLeavesTheServiceAlone(t *testing.T) {
	ctx := context.Background()
	f := obeyServiceFixture(t, "0.2.0")

	binDir, err := state.BinDir(ctx)
	if err != nil {
		t.Fatalf("BinDir: %v", err)
	}
	for _, name := range []string{"obey", "ob"} {
		p := filepath.Join(binDir, name)
		writeFile(t, p, f.body("0.2.0"))
		if err := os.Chmod(p, 0o755); err != nil {
			t.Fatalf("chmod %s: %v", p, err)
		}
	}
	f.markDaemonRunning(t)

	out, errOut, err := runInstaller(t, "uninstall", "obey", "--json")
	if err != nil {
		t.Fatalf("uninstall obey with no receipt: %v\n%s", err, errOut)
	}

	if got := readServiceLog(t, f.logPath); len(got) != 0 {
		t.Fatalf("uninstall ran service verbs for an install it does not own: %v", got)
	}

	env := envelopeOf(t, out)
	if !env.OK || len(env.Warnings) != 0 {
		t.Fatalf("expected a clean no-op envelope, got ok=%v warnings=%v", env.OK, env.Warnings)
	}
	var res struct {
		Note    string   `json:"note"`
		Removed []string `json:"removed"`
	}
	dataOf(t, out, &res)
	if !strings.Contains(res.Note, "nothing to uninstall") {
		t.Fatalf("note = %q, want it to say nothing was uninstalled", res.Note)
	}
	if len(res.Removed) != 0 {
		t.Fatalf("expected nothing removed, got %v", res.Removed)
	}

	for _, name := range []string{"obey", "ob"} {
		if _, statErr := os.Stat(filepath.Join(binDir, name)); statErr != nil {
			t.Fatalf("%s must stay on disk when festival owns nothing: %v", name, statErr)
		}
	}
}

// A refused unregister is the user's problem to finish: the unit outlives the
// binary, so the failure is reported instead of dropped.
func TestUninstallObey_ServiceFailureIsReported(t *testing.T) {
	ctx := context.Background()
	f := obeyServiceFixture(t, "0.2.0")
	if _, errOut, err := runInstaller(t, "install", "obey", "--allow-unverified", "--json"); err != nil {
		t.Fatalf("install obey: %v\n%s", err, errOut)
	}

	binDir, err := state.BinDir(ctx)
	if err != nil {
		t.Fatalf("BinDir: %v", err)
	}
	obeyPath := filepath.Join(binDir, "obey")
	writeFile(t, obeyPath, failingObey())
	if err := os.Chmod(obeyPath, 0o755); err != nil {
		t.Fatalf("chmod obey: %v", err)
	}

	out, errOut, err := runInstaller(t, "uninstall", "obey", "--json")
	if err != nil {
		t.Fatalf("a failed unregister must not fail the uninstall: %v\n%s", err, errOut)
	}

	env := envelopeOf(t, out)
	if len(env.Warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly one entry", env.Warnings)
	}
	if !strings.Contains(env.Warnings[0], "Operation not permitted") {
		t.Fatalf("warning = %q, want the supervisor's message", env.Warnings[0])
	}
	if !strings.Contains(errOut, "Operation not permitted") {
		t.Fatalf("the human path must print the failure to stderr, got %q", errOut)
	}

	var res struct {
		Removed []string `json:"removed"`
	}
	dataOf(t, out, &res)
	if len(res.Removed) != 2 {
		t.Fatalf("the receipt-owned files must still be removed, got %v", res.Removed)
	}
	_ = f
}

// An obey whose service family predates the supervised contract gets no verb
// at all. Driving it would report a restart that obey's own install had
// already performed, or call a verb that command does not have, and the user
// would be told the daemon was handled either way.
func TestInstallObey_ObeyWithoutTheSupervisedContractRunsNoServiceVerb(t *testing.T) {
	f := obeyLegacyServiceFixture(t, "0.2.0")

	out, errOut, err := runInstaller(t, "install", "obey", "--allow-unverified", "--json")
	if err != nil {
		t.Fatalf("install obey: %v\n%s", err, errOut)
	}

	var res struct {
		Files   []string            `json:"files"`
		Service *obeyServicePayload `json:"service"`
	}
	dataOf(t, out, &res)
	if res.Service == nil || !res.Service.Unsupported {
		t.Fatalf("expected data.service.unsupported true, got %+v\n%s", res.Service, out)
	}
	if res.Service.Installed || res.Service.Restarted || res.Service.Deferred || res.Service.Started {
		t.Fatalf("an obey below the contract must carry no claim about the daemon, got %+v", res.Service)
	}
	if !strings.Contains(res.Service.ContractReason, "obey service restart") {
		t.Fatalf("contract_reason = %q, want it to name the missing verb", res.Service.ContractReason)
	}
	if len(res.Files) != 2 {
		t.Fatalf("the binaries must still land, got %v", res.Files)
	}

	env := envelopeOf(t, out)
	if !env.OK || len(env.Warnings) != 1 {
		t.Fatalf("expected one warning on a successful install, got ok=%v warnings=%v", env.OK, env.Warnings)
	}
	if !strings.Contains(env.Warnings[0], "predates the supervised daemon service") {
		t.Fatalf("warning = %q, want it to say this obey predates the supervised service", env.Warnings[0])
	}
	if !strings.Contains(env.Warnings[0], "obey service install") {
		t.Fatalf("warning = %q, want it to name the command to run by hand", env.Warnings[0])
	}
	if !strings.Contains(errOut, "predates the supervised daemon service") {
		t.Fatalf("the human path must carry the same sentence on stderr, got %q", errOut)
	}

	if got := f.verbs(t); len(got) != 0 {
		t.Fatalf("no service verb may run against an obey below the contract, log = %v", got)
	}
}

// --no-restart against that obey must not report a deferred restart: the
// installer never touched the daemon, and pointing the user at
// `obey service restart` would name a command this obey does not have.
func TestInstallObey_ObeyWithoutTheSupervisedContractNeverClaimsDeferred(t *testing.T) {
	f := obeyLegacyServiceFixture(t, "0.2.0")
	if _, errOut, err := runInstaller(t, "install", "obey", "--allow-unverified", "--json"); err != nil {
		t.Fatalf("first install: %v\n%s", err, errOut)
	}
	f.markDaemonRunning(t)

	out, errOut, err := runInstaller(t, "install", "obey", "--allow-unverified", "--no-restart", "--json")
	if err != nil {
		t.Fatalf("install obey --no-restart: %v\n%s", err, errOut)
	}

	var res struct {
		Service *obeyServicePayload `json:"service"`
	}
	dataOf(t, out, &res)
	if res.Service == nil || !res.Service.Unsupported || res.Service.Deferred {
		t.Fatalf("expected unsupported true and deferred false, got %+v\n%s", res.Service, out)
	}

	env := envelopeOf(t, out)
	if len(env.Warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly one entry", env.Warnings)
	}
	if strings.Contains(env.Warnings[0], "Restart it with: obey service restart") {
		t.Fatalf("warning = %q, must not send the user to a verb this obey does not have", env.Warnings[0])
	}
	if !strings.Contains(env.Warnings[0], "obey service install") {
		t.Fatalf("warning = %q, want the command this obey actually has", env.Warnings[0])
	}

	if got := f.verbs(t); len(got) != 0 {
		t.Fatalf("no service verb may run against an obey below the contract, log = %v", got)
	}
}

func TestUpdateObey_ObeyWithoutTheSupervisedContractRunsNoServiceVerb(t *testing.T) {
	f := obeyLegacyServiceFixture(t, "0.2.0")
	if _, errOut, err := runInstaller(t, "install", "obey", "--allow-unverified", "--json"); err != nil {
		t.Fatalf("install obey: %v\n%s", err, errOut)
	}
	f.markDaemonRunning(t)
	f.publishUpgrade(t, "0.2.1")

	out, errOut, err := runInstaller(t, "update", "obey", "--allow-unverified", "--json")
	if err != nil {
		t.Fatalf("update obey: %v\n%s", err, errOut)
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
	if res.Service == nil || !res.Service.Unsupported {
		t.Fatalf("expected data.service.unsupported true, got %+v\n%s", res.Service, out)
	}
	if res.Service.Restarted || res.Service.Deferred || res.Service.Started {
		t.Fatalf("an obey below the contract must carry no claim about the daemon, got %+v", res.Service)
	}

	if got := f.verbs(t); len(got) != 0 {
		t.Fatalf("no service verb may run against an obey below the contract, log = %v", got)
	}
}

// A daemon whose state could not be read is not a daemon that was not running.
// With --no-restart the step defers and says the state is unknown, rather than
// reporting that the install started a daemon on the new version.
func TestInstallObey_UnreadableDaemonStateDefersWithoutClaimingStarted(t *testing.T) {
	f := obeyServiceFixture(t, "0.2.0")
	if _, errOut, err := runInstaller(t, "install", "obey", "--allow-unverified", "--json"); err != nil {
		t.Fatalf("first install: %v\n%s", err, errOut)
	}
	f.breakDaemonProbe(t)

	out, errOut, err := runInstaller(t, "install", "obey", "--allow-unverified", "--no-restart", "--json")
	if err != nil {
		t.Fatalf("install obey --no-restart: %v\n%s", err, errOut)
	}

	var res struct {
		Service *obeyServicePayload `json:"service"`
	}
	dataOf(t, out, &res)
	if res.Service == nil || !res.Service.Deferred || !res.Service.DaemonStateUnknown {
		t.Fatalf("expected deferred and daemon_state_unknown true, got %+v\n%s", res.Service, out)
	}
	if res.Service.Started || res.Service.Restarted {
		t.Fatalf("a probe that could not answer must claim neither a start nor a restart, got %+v", res.Service)
	}

	env := envelopeOf(t, out)
	if len(env.Warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly one entry", env.Warnings)
	}
	if !strings.Contains(env.Warnings[0], "could not be determined") {
		t.Fatalf("warning = %q, want it to say the daemon state could not be determined", env.Warnings[0])
	}
	if !strings.Contains(env.Warnings[0], "obey service status") {
		t.Fatalf("warning = %q, want it to point at the status check", env.Warnings[0])
	}

	wantVerbs(t, f.verbs(t), []string{"service install", "service install"})
}

// Without --no-restart the same unknown is settled by restarting: the cost of
// a restart nobody needed is one supervised process swap, and the cost of the
// other guess is a live daemon left on a replaced binary.
func TestUpdateObey_UnreadableDaemonStateRestartsRatherThanClaimingStarted(t *testing.T) {
	f := obeyServiceFixture(t, "0.2.0")
	if _, errOut, err := runInstaller(t, "install", "obey", "--allow-unverified", "--json"); err != nil {
		t.Fatalf("install obey: %v\n%s", err, errOut)
	}
	f.breakDaemonProbe(t)
	f.publishUpgrade(t, "0.2.1")

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
	if res.Service == nil || !res.Service.Restarted || !res.Service.DaemonStateUnknown {
		t.Fatalf("expected restarted and daemon_state_unknown true, got %+v\n%s", res.Service, out)
	}
	if res.Service.Started {
		t.Fatalf("started must stay unset when the probe could not tell, got %+v", res.Service)
	}

	if env := envelopeOf(t, out); len(env.Warnings) != 0 {
		t.Fatalf("a completed restart leaves nothing pending, got %v", env.Warnings)
	}
	wantVerbs(t, f.verbs(t), []string{"service install", "service install", "service restart"})
}
