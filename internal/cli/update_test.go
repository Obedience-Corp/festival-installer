package cli_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
)

func writeManagedBinary(t *testing.T, binDir, name, version string) {
	t.Helper()
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	script := "#!/bin/sh\necho " + version + "\n"
	p := filepath.Join(binDir, name)
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatalf("write managed binary: %v", err)
	}
}

func writeFestivalReceipt(t *testing.T, ctx context.Context, home, version, sourceName, binDir string) {
	t.Helper()
	rec := receipts.Receipt{
		PackageID:   festivalPackageIDForTest,
		Version:     version,
		Source:      sourceName,
		Channel:     "stable",
		InstalledAt: time.Now().UTC(),
		OwnedFiles: []receipts.OwnedFile{
			{Path: filepath.Join(binDir, "camp"), Hash: "deadbeef", Mode: 0o755},
			{Path: filepath.Join(binDir, "fest"), Hash: "deadbeef", Mode: 0o755},
		},
		Metadata: map[string]string{},
	}
	if err := receipts.Write(ctx, mustDB(t, ctx, home), rec); err != nil {
		t.Fatalf("write receipt: %v", err)
	}
}

func writeFestivalReceiptWithHub(t *testing.T, ctx context.Context, home, version, sourceName, binDir string) {
	t.Helper()
	rec := receipts.Receipt{
		PackageID:   festivalPackageIDForTest,
		Version:     version,
		Source:      sourceName,
		Channel:     "stable",
		InstalledAt: time.Now().UTC(),
		OwnedFiles: []receipts.OwnedFile{
			{Path: filepath.Join(binDir, "camp"), Hash: "deadbeef", Mode: 0o755},
			{Path: filepath.Join(binDir, "fest"), Hash: "deadbeef", Mode: 0o755},
			{Path: filepath.Join(binDir, "festival"), Hash: "deadbeef", Mode: 0o755},
		},
		Metadata: map[string]string{},
	}
	if err := receipts.Write(ctx, mustDB(t, ctx, home), rec); err != nil {
		t.Fatalf("write receipt: %v", err)
	}
}

func TestUpdate_CompletionOffersOptionalTargets(t *testing.T) {
	t.Setenv("OBEY_INSTALLER_HOME", t.TempDir())

	out, errOut, err := runInstaller(t, "__complete", "update", "")
	if err != nil {
		t.Fatalf("complete: %v\n%s", err, errOut)
	}
	for _, target := range []string{"festival", "camp", "fest"} {
		if !strings.Contains(out, target+"\n") {
			t.Fatalf("expected completion to offer %q, got %q", target, out)
		}
	}

	out, errOut, err = runInstaller(t, "__complete", "update", "festival", "")
	if err != nil {
		t.Fatalf("complete second arg: %v\n%s", err, errOut)
	}
	if strings.Contains(out, "festival\n") || strings.Contains(out, "camp\n") || strings.Contains(out, "fest\n") {
		t.Fatalf("expected no target completions past the first argument, got %q", out)
	}
}

func TestUpdate_UnknownTargetRejected(t *testing.T) {
	t.Setenv("OBEY_INSTALLER_HOME", t.TempDir())

	_, _, err := runInstaller(t, "update", "widget")
	if err == nil {
		t.Fatal("expected unknown target to fail")
	}
	if !hasErrorCode(err, "E_UPDATE_TARGET") {
		t.Fatalf("expected E_UPDATE_TARGET, got %v", err)
	}
}

func TestUpdate_TargetContract(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantNotice string
	}{
		{name: "no arg defaults to festival"},
		{name: "festival explicit", args: []string{"festival"}},
		{name: "camp alias", args: []string{"camp"}, wantNotice: "camp is part of the festival suite; updating the suite"},
		{name: "fest alias", args: []string{"fest"}, wantNotice: "fest is part of the festival suite; updating the suite"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("OBEY_INSTALLER_HOME", home)
			ctx := context.Background()
			binDir := filepath.Join(home, "bin")

			writeManagedBinary(t, binDir, "camp", "0.2.10")
			writeManagedBinary(t, binDir, "fest", "0.2.10")

			repo := fixtureInstallMarketplace(t, "https://example.test/festival.tar.gz", strings.Repeat("a", 64))
			if _, errOut, err := runInstaller(t, "marketplace", "add", repo, "--name", "official-obey"); err != nil {
				t.Fatalf("marketplace add: %v\n%s", err, errOut)
			}
			writeFestivalReceipt(t, ctx, home, "0.2.10", "official-obey", binDir)

			args := append([]string{"update"}, tc.args...)
			args = append(args, "--allow-unverified", "--json")
			out, errOut, err := runInstaller(t, args...)
			if err != nil {
				t.Fatalf("update: %v\n%s", err, errOut)
			}

			var res struct {
				Action  string `json:"action"`
				Version string `json:"version"`
			}
			dataOf(t, out, &res)
			if res.Action != "current" || res.Version != "0.2.10" {
				t.Fatalf("expected current 0.2.10, got %+v", res)
			}

			if tc.wantNotice == "" {
				if strings.Contains(errOut, "is part of the festival suite") {
					t.Fatalf("unexpected alias notice: %q", errOut)
				}
				return
			}
			if !strings.Contains(errOut, tc.wantNotice) {
				t.Fatalf("expected alias notice %q, got %q", tc.wantNotice, errOut)
			}
		})
	}
}

func TestUpdate_UnmanagedExternalLeftUntouched(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)

	extDir := t.TempDir()
	writeManagedBinary(t, extDir, "camp", "0.9.9")
	before, _ := os.ReadFile(filepath.Join(extDir, "camp"))
	t.Setenv("PATH", extDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	out, errOut, err := runInstaller(t, "update", "festival", "--allow-unverified", "--json")
	if err != nil {
		t.Fatalf("update: %v\n%s", err, errOut)
	}
	var res struct {
		Action string `json:"action"`
	}
	dataOf(t, out, &res)
	if res.Action != "unmanaged" {
		t.Fatalf("expected unmanaged, got %q", res.Action)
	}
	if !strings.Contains(errOut, "not managed") {
		t.Fatalf("expected unmanaged warning, got %q", errOut)
	}
	after, _ := os.ReadFile(filepath.Join(extDir, "camp"))
	if string(before) != string(after) {
		t.Fatal("external camp was modified")
	}
}

func TestUpdate_CurrentNoOp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	ctx := context.Background()
	binDir := filepath.Join(home, "bin")

	writeManagedBinary(t, binDir, "camp", "0.2.10")
	writeManagedBinary(t, binDir, "fest", "0.2.10")
	campBefore, _ := os.ReadFile(filepath.Join(binDir, "camp"))

	repo := fixtureInstallMarketplace(t, "https://example.test/festival.tar.gz", strings.Repeat("a", 64))
	if _, errOut, err := runInstaller(t, "marketplace", "add", repo, "--name", "official-obey"); err != nil {
		t.Fatalf("marketplace add: %v\n%s", err, errOut)
	}
	writeFestivalReceipt(t, ctx, home, "0.2.10", "official-obey", binDir)

	out, _, err := runInstaller(t, "update", "festival", "--allow-unverified", "--json")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	var res struct {
		Action  string `json:"action"`
		Version string `json:"version"`
	}
	dataOf(t, out, &res)
	if res.Action != "current" || res.Version != "0.2.10" {
		t.Fatalf("expected current 0.2.10, got %+v", res)
	}
	campAfter, _ := os.ReadFile(filepath.Join(binDir, "camp"))
	if string(campBefore) != string(campAfter) {
		t.Fatal("camp changed on a no-op update")
	}
}

func TestUpdate_UpgradeReplacesPair(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	ctx := context.Background()
	binDir := filepath.Join(home, "bin")

	writeManagedBinary(t, binDir, "camp", "0.2.9")
	writeManagedBinary(t, binDir, "fest", "0.2.9")
	symlinkSelfAsManagedFestival(t, home)

	newCamp := "#!/bin/sh\necho new-camp\n"
	newFest := "#!/bin/sh\necho new-fest\n"
	newFestival := "#!/bin/sh\necho new-festival\n"
	tarball := buildSuiteTarGz(t, map[string]string{"camp": newCamp, "fest": newFest, "festival": newFestival})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(tarball)
	}))
	t.Cleanup(srv.Close)

	repo := fixtureInstallMarketplace(t, srv.URL+"/festival.tar.gz", sha256Hex(tarball))
	if _, errOut, err := runInstaller(t, "marketplace", "add", repo, "--name", "official-obey"); err != nil {
		t.Fatalf("marketplace add: %v\n%s", err, errOut)
	}
	writeFestivalReceipt(t, ctx, home, "0.2.9", "official-obey", binDir)

	out, _, err := runInstaller(t, "update", "festival", "--allow-unverified", "--json")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	var res struct {
		Action       string `json:"action"`
		Version      string `json:"version"`
		From         string `json:"from"`
		SelfReplaced bool   `json:"self_replaced"`
	}
	dataOf(t, out, &res)
	if res.Action != "upgraded" || res.Version != "0.2.10" || res.From != "0.2.9" {
		t.Fatalf("expected upgraded 0.2.9 -> 0.2.10, got %+v", res)
	}
	if !res.SelfReplaced {
		t.Fatalf("expected self_replaced true when the managed hub was placed, got %+v", res)
	}
	got, _ := os.ReadFile(filepath.Join(binDir, "camp"))
	if string(got) != newCamp {
		t.Fatalf("camp not replaced: %q", got)
	}
	gotFestival, _ := os.ReadFile(filepath.Join(binDir, "festival"))
	if string(gotFestival) != newFestival {
		t.Fatalf("festival not replaced: %q", gotFestival)
	}
	rec, err := receipts.Get(ctx, mustDB(t, ctx, home), festivalPackageIDForTest)
	if err != nil || rec.Version != "0.2.10" {
		t.Fatalf("receipt not updated to 0.2.10: %+v err=%v", rec, err)
	}
}

func TestUpdate_UpgradedSelfReplacedPrintsRestartLine(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	ctx := context.Background()
	binDir := filepath.Join(home, "bin")

	writeManagedBinary(t, binDir, "camp", "0.2.9")
	writeManagedBinary(t, binDir, "fest", "0.2.9")
	symlinkSelfAsManagedFestival(t, home)

	tarball := buildSuiteTarGz(t, map[string]string{
		"camp":     "#!/bin/sh\necho new-camp\n",
		"fest":     "#!/bin/sh\necho new-fest\n",
		"festival": "#!/bin/sh\necho new-festival\n",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(tarball)
	}))
	t.Cleanup(srv.Close)

	repo := fixtureInstallMarketplace(t, srv.URL+"/festival.tar.gz", sha256Hex(tarball))
	if _, errOut, err := runInstaller(t, "marketplace", "add", repo, "--name", "official-obey"); err != nil {
		t.Fatalf("marketplace add: %v\n%s", err, errOut)
	}
	writeFestivalReceipt(t, ctx, home, "0.2.9", "official-obey", binDir)

	out, _, err := runInstaller(t, "update", "festival", "--allow-unverified")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !strings.Contains(out, "restart it to use the new version") {
		t.Fatalf("expected a restart line for a self-replaced hub, got %q", out)
	}
}

func TestUpdate_CurrentActionHasNoRestartLine(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	ctx := context.Background()
	binDir := filepath.Join(home, "bin")

	writeManagedBinary(t, binDir, "camp", "0.2.10")
	writeManagedBinary(t, binDir, "fest", "0.2.10")
	symlinkSelfAsManagedFestival(t, home)

	repo := fixtureInstallMarketplace(t, "https://example.test/festival.tar.gz", strings.Repeat("a", 64))
	if _, errOut, err := runInstaller(t, "marketplace", "add", repo, "--name", "official-obey"); err != nil {
		t.Fatalf("marketplace add: %v\n%s", err, errOut)
	}
	writeFestivalReceipt(t, ctx, home, "0.2.10", "official-obey", binDir)

	out, _, err := runInstaller(t, "update", "festival", "--allow-unverified")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if strings.Contains(out, "restart it to use the new version") {
		t.Fatalf("expected no restart line for an already-current update, got %q", out)
	}
}

func TestUpdate_ExternalHubUpdatesPairAndWarns(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	ctx := context.Background()
	binDir := filepath.Join(home, "bin")

	writeManagedBinary(t, binDir, "camp", "0.2.9")
	writeManagedBinary(t, binDir, "fest", "0.2.9")
	// No symlinkSelfAsManagedFestival call: the running test binary is not
	// binDir/festival, exactly like a Homebrew cellar copy or a dev build.

	newCamp := "#!/bin/sh\necho new-camp\n"
	newFest := "#!/bin/sh\necho new-fest\n"
	newFestival := "#!/bin/sh\necho new-festival\n"
	tarball := buildSuiteTarGz(t, map[string]string{"camp": newCamp, "fest": newFest, "festival": newFestival})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(tarball)
	}))
	t.Cleanup(srv.Close)

	repo := fixtureInstallMarketplace(t, srv.URL+"/festival.tar.gz", sha256Hex(tarball))
	if _, errOut, err := runInstaller(t, "marketplace", "add", repo, "--name", "official-obey"); err != nil {
		t.Fatalf("marketplace add: %v\n%s", err, errOut)
	}
	writeFestivalReceipt(t, ctx, home, "0.2.9", "official-obey", binDir)

	out, errOut, err := runInstaller(t, "update", "festival", "--allow-unverified", "--json")
	if err != nil {
		t.Fatalf("update: %v\n%s", err, errOut)
	}
	var res struct {
		Action        string `json:"action"`
		Version       string `json:"version"`
		From          string `json:"from"`
		SelfPlacement string `json:"self_placement"`
		SelfPath      string `json:"self_path"`
		SelfReplaced  bool   `json:"self_replaced"`
	}
	dataOf(t, out, &res)
	if res.Action != "upgraded" || res.Version != "0.2.10" || res.From != "0.2.9" {
		t.Fatalf("expected upgraded 0.2.9 -> 0.2.10, got %+v", res)
	}
	if res.SelfPlacement != "external" {
		t.Fatalf("expected self_placement external, got %+v", res)
	}
	if res.SelfPath == "" {
		t.Fatalf("expected self_path to be set: %+v", res)
	}
	if res.SelfReplaced {
		t.Fatalf("expected self_replaced false for an external hub, got %+v", res)
	}
	if !strings.Contains(errOut, "left untouched") {
		t.Fatalf("expected a left-untouched warning on stderr, got %q", errOut)
	}

	campGot, _ := os.ReadFile(filepath.Join(binDir, "camp"))
	if string(campGot) != newCamp {
		t.Fatalf("camp not replaced: %q", campGot)
	}
	festGot, _ := os.ReadFile(filepath.Join(binDir, "fest"))
	if string(festGot) != newFest {
		t.Fatalf("fest not replaced: %q", festGot)
	}
	if _, statErr := os.Stat(filepath.Join(binDir, "festival")); !os.IsNotExist(statErr) {
		t.Fatalf("expected no managed festival binary to be written, stat err=%v", statErr)
	}

	rec, err := receipts.Get(ctx, mustDB(t, ctx, home), festivalPackageIDForTest)
	if err != nil || rec.Version != "0.2.10" {
		t.Fatalf("receipt not updated to 0.2.10: %+v err=%v", rec, err)
	}
	for _, of := range rec.OwnedFiles {
		if filepath.Base(of.Path) == "festival" {
			t.Fatalf("receipt should not own an external hub binary: %+v", rec.OwnedFiles)
		}
	}
}

func TestUpdate_LiveReceiptDisagreementPrefersLive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	ctx := context.Background()
	binDir := filepath.Join(home, "bin")

	writeManagedBinary(t, binDir, "camp", "0.3.0")
	writeManagedBinary(t, binDir, "fest", "0.3.0")

	repo := fixtureInstallMarketplace(t, "https://example.test/festival.tar.gz", strings.Repeat("a", 64))
	if _, errOut, err := runInstaller(t, "marketplace", "add", repo, "--name", "official-obey"); err != nil {
		t.Fatalf("marketplace add: %v\n%s", err, errOut)
	}
	writeFestivalReceipt(t, ctx, home, "0.2.10", "official-obey", binDir)

	out, errOut, err := runInstaller(t, "update", "festival", "--allow-unverified", "--json")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !strings.Contains(errOut, "live version") {
		t.Fatalf("expected live-disagreement warning, got %q", errOut)
	}
	var res struct {
		Action  string `json:"action"`
		Version string `json:"version"`
	}
	dataOf(t, out, &res)
	if res.Action != "current" || res.Version != "0.3.0" {
		t.Fatalf("expected current at live 0.3.0, got %+v", res)
	}
}
