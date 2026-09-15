//go:build notty

package cli_test

import (
	"os"
	"slices"
	"strings"
	"testing"
)

// TestNoTTY_EveryAppFacingCommandCompletes drives every command festival-app
// calls through the real binary with no controlling terminal, asserting each
// one completes with the expected exit code and one parseable object on stdout.
func TestNoTTY_EveryAppFacingCommandCompletes(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary and runs it; not a short test")
	}
	t.Parallel()
	ctx := t.Context()
	fx := newHeadlessFixture(t)

	cases := []struct {
		name      string
		args      []string
		wantExit  []int
		action    string
		requireOK bool
		assert    func(t *testing.T, obj map[string]any)
	}{
		{
			name:     "marketplace add",
			args:     []string{"marketplace", "add", fx.Marketplace, "--name", "official-obey", "--allow-unverified"},
			wantExit: []int{0},
		},
		{
			name:      "status",
			args:      []string{"status", "--json"},
			wantExit:  []int{0},
			action:    "status",
			requireOK: true,
		},
		{
			name:      "install festival",
			args:      []string{"install", "festival", "--allow-unverified", "--json"},
			wantExit:  []int{0},
			action:    "install",
			requireOK: true,
		},
		{
			name:      "resolve camp",
			args:      []string{"resolve", "camp", "--json"},
			wantExit:  []int{0},
			action:    "resolve",
			requireOK: true,
		},
		{
			name:      "list",
			args:      []string{"list", "--json"},
			wantExit:  []int{0},
			action:    "list",
			requireOK: true,
		},
		{
			name:      "install obey",
			args:      []string{"install", "obey", "--allow-unverified", "--json"},
			wantExit:  []int{0},
			action:    "install",
			requireOK: true,
		},
		{
			name:     "update obey with no restart",
			args:     []string{"update", "obey", "--allow-unverified", "--json", "--no-restart"},
			wantExit: []int{0},
			action:   "update",
		},
		{
			name:     "update suite",
			args:     []string{"update", "--allow-unverified", "--json"},
			wantExit: []int{0},
			action:   "update",
		},
		{
			// doctor is the one row whose exit code is not fixed: it returns
			// E_DOCTOR_FAIL when any check fails, and managed_bin_on_path fails
			// on a home whose bin dir is not on PATH.
			name:     "doctor",
			args:     []string{"doctor", "--json"},
			wantExit: []int{0, 1},
			action:   "doctor",
			assert:   assertDoctorChecksPresent,
		},
		{
			name:      "uninstall obey",
			args:      []string{"uninstall", "obey", "--json"},
			wantExit:  []int{0},
			action:    "uninstall",
			requireOK: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			run := fx.run(ctx, t, tc.args...)
			if !slices.Contains(tc.wantExit, run.ExitCode) {
				t.Fatalf("exit = %d, want one of %v\nstdout:\n%s\nstderr:\n%s",
					run.ExitCode, tc.wantExit, run.Stdout, run.Stderr)
			}
			if tc.action == "" {
				return
			}
			obj := assertSingleJSONObject(t, run.Stdout)
			if got := obj["action"]; got != tc.action {
				t.Fatalf("action = %v, want %q\n%s", got, tc.action, run.Stdout)
			}
			if tc.requireOK && obj["ok"] != true {
				t.Fatalf("ok = %v, want true\n%s", obj["ok"], run.Stdout)
			}
			if tc.assert != nil {
				tc.assert(t, obj)
			}
		})
	}
}

func assertDoctorChecksPresent(t *testing.T, obj map[string]any) {
	t.Helper()
	data, ok := obj["data"].(map[string]any)
	if !ok {
		t.Fatalf("doctor envelope carries no data object: %v", obj)
	}
	checks, ok := data["checks"].([]any)
	if !ok || len(checks) == 0 {
		t.Fatalf("doctor envelope carries no checks: %v", data)
	}
}

// TestNoTTY_BareInvocationPrintsHelpNotTUI proves tuiLaunchDecision cannot open
// the TUI when stdin and stdout are not terminals, on both home shapes
// BareInvocation branches on.
func TestNoTTY_BareInvocationPrintsHelpNotTUI(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary and runs it; not a short test")
	}
	t.Parallel()
	ctx := t.Context()
	fx := newHeadlessFixture(t)

	t.Run("first run home prints the setup steps", func(t *testing.T) {
		run := fx.run(ctx, t)
		if run.ExitCode != 0 {
			t.Fatalf("exit = %d, want 0\nstdout:\n%s\nstderr:\n%s", run.ExitCode, run.Stdout, run.Stderr)
		}
		for _, want := range []string{"festival is not set up on this machine yet.", "festival install festival"} {
			if !strings.Contains(run.Stdout, want) {
				t.Fatalf("stdout is missing %q:\n%s", want, run.Stdout)
			}
		}
	})

	t.Run("set up home prints the command list", func(t *testing.T) {
		fx.addMarketplace(ctx, t)
		run := fx.run(ctx, t)
		if run.ExitCode != 0 {
			t.Fatalf("exit = %d, want 0\nstdout:\n%s\nstderr:\n%s", run.ExitCode, run.Stdout, run.Stderr)
		}
		if !strings.Contains(run.Stderr, "no interactive terminal detected") {
			t.Fatalf("stderr is missing the no-terminal notice:\n%s", run.Stderr)
		}
		if !strings.Contains(run.Stdout, "Available Commands:") {
			t.Fatalf("stdout is missing the help command list:\n%s", run.Stdout)
		}
	})
}

// TestNoTTY_ForcedTUIFailsCleanly proves the forced path errors rather than
// hanging on a terminal query.
func TestNoTTY_ForcedTUIFailsCleanly(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary and runs it; not a short test")
	}
	t.Parallel()
	fx := newHeadlessFixture(t)

	run := fx.run(t.Context(), t, "--tui")
	if run.ExitCode == 0 {
		t.Fatalf("festival --tui exited 0 with no terminal\nstdout:\n%s\nstderr:\n%s", run.Stdout, run.Stderr)
	}
	if !strings.Contains(run.Stderr, "TUI requires an interactive terminal") {
		t.Fatalf("stderr does not name the terminal requirement:\n%s", run.Stderr)
	}
}

// TestNoTTY_PackageOriginRefusesSuiteInstallAndAllowsForce pins the three
// answers festival-app depends on for a Homebrew suite user: the suite install
// is refused, --force overrides the refusal, and the obey install is never
// refused so the user still gets a managed daemon.
func TestNoTTY_PackageOriginRefusesSuiteInstallAndAllowsForce(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary and runs it; not a short test")
	}
	t.Parallel()
	ctx := t.Context()
	fx := newPackageOriginFixture(t)
	fx.addMarketplace(ctx, t)

	which := fx.run(ctx, t, "which", "camp", "--json")
	if which.ExitCode != 0 {
		t.Fatalf("which camp exit = %d\nstdout:\n%s\nstderr:\n%s", which.ExitCode, which.Stdout, which.Stderr)
	}
	whichObj := assertSingleJSONObject(t, which.Stdout)
	if whichObj["origin"] != "package" {
		t.Fatalf("fixture did not classify as a package origin:\n%s", which.Stdout)
	}
	t.Logf("which camp --json:\n%s", which.Stdout)

	refused := fx.run(ctx, t, "install", "festival", "--allow-unverified", "--json")
	if refused.ExitCode != 1 {
		t.Fatalf("suite install exit = %d, want 1\nstdout:\n%s\nstderr:\n%s",
			refused.ExitCode, refused.Stdout, refused.Stderr)
	}
	if code := envelopeErrorCode(t, assertSingleJSONObject(t, refused.Stdout)); code != "E_INSTALL_PACKAGE_CHANNEL" {
		t.Fatalf("error.code = %q, want E_INSTALL_PACKAGE_CHANNEL\n%s", code, refused.Stdout)
	}

	forced := fx.run(ctx, t, "install", "festival", "--force", "--allow-unverified", "--json")
	if forced.ExitCode != 0 {
		t.Fatalf("--force suite install exit = %d, want 0\nstdout:\n%s\nstderr:\n%s",
			forced.ExitCode, forced.Stdout, forced.Stderr)
	}
	if obj := assertSingleJSONObject(t, forced.Stdout); obj["ok"] != true {
		t.Fatalf("--force suite install envelope not ok:\n%s", forced.Stdout)
	}

	obey := fx.run(ctx, t, "install", "obey", "--allow-unverified", "--json")
	if obey.ExitCode != 0 {
		t.Fatalf("obey install exit = %d, want 0 on a package-origin machine\nstdout:\n%s\nstderr:\n%s",
			obey.ExitCode, obey.Stdout, obey.Stderr)
	}
	obeyObj := assertSingleJSONObject(t, obey.Stdout)
	if obeyObj["ok"] != true {
		t.Fatalf("obey install envelope not ok:\n%s", obey.Stdout)
	}
	for _, name := range []string{"obey", "ob"} {
		if _, err := os.Stat(fx.BinDir + string(os.PathSeparator) + name); err != nil {
			t.Fatalf("obey install did not place %s: %v", name, err)
		}
	}
}

// TestNoTTY_PackageOriginUpdatePrintsCommandAndDoesNotRunIt proves the update
// path hands the package-manager command back instead of running it, including
// under --force, which UpdateFestival deliberately excludes from this route.
func TestNoTTY_PackageOriginUpdatePrintsCommandAndDoesNotRunIt(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary and runs it; not a short test")
	}
	t.Parallel()
	ctx := t.Context()
	fx := newPackageOriginFixture(t)

	const wantUpgrade = "brew upgrade --cask festival"

	for _, args := range [][]string{
		{"update", "--json"},
		{"update", "--force", "--json"},
	} {
		run := fx.run(ctx, t, args...)
		if run.ExitCode != 0 {
			t.Fatalf("exit = %d, want 0\nstdout:\n%s\nstderr:\n%s", run.ExitCode, run.Stdout, run.Stderr)
		}
		obj := assertSingleJSONObject(t, run.Stdout)
		data, ok := obj["data"].(map[string]any)
		if !ok {
			t.Fatalf("update envelope carries no data object:\n%s", run.Stdout)
		}
		if data["action"] != "package" {
			t.Fatalf("data.action = %v, want \"package\"\n%s", data["action"], run.Stdout)
		}
		if data["upgrade"] != wantUpgrade {
			t.Fatalf("data.upgrade = %v, want %q\n%s", data["upgrade"], wantUpgrade, run.Stdout)
		}
		t.Logf("data.version=%v data.latest=%v data.upgrade=%v", data["version"], data["latest"], data["upgrade"])
		fx.assertNoPackageManagerRan(t)
	}

	// The human path is the one that reaches maybeRunPackageUpgrade at all;
	// --json returns before it. cmdStdioIsTTY is false here in a real process
	// with real pipes, so the command is printed and never run.
	human := fx.run(ctx, t, "update")
	if human.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0\nstdout:\n%s\nstderr:\n%s", human.ExitCode, human.Stdout, human.Stderr)
	}
	if !strings.Contains(human.Stderr, "upgrade with: "+wantUpgrade) {
		t.Fatalf("stderr does not hand back the upgrade command:\n%s", human.Stderr)
	}
	if strings.Contains(human.Stdout, "package upgraded") {
		t.Fatalf("the package upgrade ran headlessly:\n%s", human.Stdout)
	}
	t.Logf("human update stdout:\n%s\nhuman update stderr:\n%s", human.Stdout, human.Stderr)
	fx.assertNoPackageManagerRan(t)
}

func envelopeErrorCode(t *testing.T, obj map[string]any) string {
	t.Helper()
	errObj, ok := obj["error"].(map[string]any)
	if !ok {
		t.Fatalf("envelope carries no error object: %v", obj)
	}
	code, _ := errObj["code"].(string)
	return code
}
