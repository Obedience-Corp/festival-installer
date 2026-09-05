package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/source"
	"github.com/Obedience-Corp/festival-installer/internal/state"
	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
)

func TestResolveSetupState_CancelledContext(t *testing.T) {
	t.Setenv("FESTIVAL_HOME", t.TempDir())
	t.Setenv("OBEY_INSTALLER_HOME", "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := ResolveSetupState(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v, want context.Canceled in the chain", err)
	}
	if got != (SetupState{}) {
		t.Fatalf("cancelled call must not report signals, got %+v", got)
	}
}

func TestSetupState_Predicates(t *testing.T) {
	tests := []struct {
		name       string
		state      SetupState
		firstRun   bool
		needsSetup bool
	}{
		{"nothing", SetupState{}, true, true},
		{"path only", SetupState{ManagedBinOnPath: true}, false, true},
		{"marketplace only", SetupState{HasMarketplaces: true}, false, true},
		{"marketplace and path", SetupState{HasMarketplaces: true, ManagedBinOnPath: true}, false, true},
		{"receipts only", SetupState{HasReceipts: true}, false, true},
		{"receipts and path", SetupState{HasReceipts: true, ManagedBinOnPath: true}, false, false},
		{"receipts and marketplace", SetupState{HasReceipts: true, HasMarketplaces: true}, false, true},
		{
			"everything",
			SetupState{HasReceipts: true, HasMarketplaces: true, ManagedBinOnPath: true},
			false,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.IsFirstRun(); got != tt.firstRun {
				t.Errorf("IsFirstRun()=%v, want %v", got, tt.firstRun)
			}
			if got := tt.state.NeedsSetup(); got != tt.needsSetup {
				t.Errorf("NeedsSetup()=%v, want %v", got, tt.needsSetup)
			}
		})
	}
}

func TestResolveSetupState_FreshHomeIsFirstRun(t *testing.T) {
	home := freshHome(t)

	got, err := ResolveSetupState(context.Background())
	if err != nil {
		t.Fatalf("ResolveSetupState: %v", err)
	}
	if !got.IsFirstRun() {
		t.Fatalf("fresh home must be a first run, got %+v", got)
	}
	if got.ManagedBin != filepath.Join(home, "bin") {
		t.Fatalf("ManagedBin=%q, want the managed bin dir under home", got.ManagedBin)
	}
	assertNoInstallerState(t, home)
}

func TestResolveSetupState_MarketplaceOnlyIsMidSetup(t *testing.T) {
	freshHome(t)
	t.Setenv("PATH", gitOnlyPath(t))
	ctx := context.Background()
	fixture := browseSeedFixtureRepo(t)
	if _, err := source.AddMarketplace(ctx, fixture, "acme", source.DefaultVerifyOptions(nil, false)); err != nil {
		t.Fatalf("register fixture marketplace: %v", err)
	}

	got, err := ResolveSetupState(ctx)
	if err != nil {
		t.Fatalf("ResolveSetupState: %v", err)
	}
	if !got.HasMarketplaces {
		t.Fatalf("registered source not seen, got %+v", got)
	}
	if got.IsFirstRun() {
		t.Fatalf("a home with a registered source is mid-setup, not a first run: %+v", got)
	}
	if !got.NeedsSetup() {
		t.Fatalf("a source alone does not finish setup: %+v", got)
	}
}

func TestResolveSetupState_ReceiptAndPathIsSetUp(t *testing.T) {
	home := freshHome(t)
	ctx := context.Background()
	binDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	t.Setenv("PATH", binDir)
	writeInstalledReceipt(t, home)

	got, err := ResolveSetupState(ctx)
	if err != nil {
		t.Fatalf("ResolveSetupState: %v", err)
	}
	if !got.HasReceipts || !got.ManagedBinOnPath {
		t.Fatalf("set-up home misread: %+v", got)
	}
	if got.IsFirstRun() || got.NeedsSetup() {
		t.Fatalf("set-up home must need nothing further: %+v", got)
	}
}

func TestStatus_CarriesSetupState(t *testing.T) {
	home := freshHome(t)

	sum, err := Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !sum.Setup.IsFirstRun() {
		t.Fatalf("Status must carry the fresh-home setup state, got %+v", sum.Setup)
	}
	if sum.Setup.ManagedBin != sum.ManagedBin || sum.Setup.ManagedBinOnPath != sum.ManagedBinOnPath {
		t.Fatalf("Setup disagrees with the legacy fields: %+v", sum)
	}
	assertNoInstallerState(t, home)
}

// freshHome points every home lookup at an empty directory and clears PATH of
// anything the host happens to have installed, so a test observes only what it
// sets up itself.
func freshHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	t.Setenv("OBEY_INSTALLER_HOME", "")
	t.Setenv("PATH", t.TempDir())
	return home
}

// gitOnlyPath is an isolated PATH that still resolves git, which the fixture
// marketplace repo needs. The managed bin dir lives under a temp home, so it is
// still absent from PATH and the state under test stays deterministic.
func gitOnlyPath(t *testing.T) string {
	t.Helper()
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skipf("git is not available: %v", err)
	}
	return filepath.Dir(gitPath)
}

func writeInstalledReceipt(t *testing.T, home string) {
	t.Helper()
	ctx := context.Background()
	db, err := state.OpenDB(ctx, home)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close(ctx) })
	rec := receipts.Receipt{
		PackageID: FestivalPackageID,
		Version:   "0.1.0",
		Channel:   "stable",
		Source:    state.OfficialSeedKey,
	}
	if err := receipts.Write(ctx, db.Raw(), rec); err != nil {
		t.Fatalf("write receipt: %v", err)
	}
}
