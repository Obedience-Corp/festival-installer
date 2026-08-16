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

	newCamp := "#!/bin/sh\necho new-camp\n"
	newFest := "#!/bin/sh\necho new-fest\n"
	tarball := buildSuiteTarGz(t, map[string]string{"camp": newCamp, "fest": newFest})
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
		Action  string `json:"action"`
		Version string `json:"version"`
		From    string `json:"from"`
	}
	dataOf(t, out, &res)
	if res.Action != "upgraded" || res.Version != "0.2.10" || res.From != "0.2.9" {
		t.Fatalf("expected upgraded 0.2.9 -> 0.2.10, got %+v", res)
	}
	got, _ := os.ReadFile(filepath.Join(binDir, "camp"))
	if string(got) != newCamp {
		t.Fatalf("camp not replaced: %q", got)
	}
	rec, err := receipts.Get(ctx, mustDB(t, ctx, home), festivalPackageIDForTest)
	if err != nil || rec.Version != "0.2.10" {
		t.Fatalf("receipt not updated to 0.2.10: %+v err=%v", rec, err)
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
