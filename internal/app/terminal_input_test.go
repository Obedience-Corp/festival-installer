package app

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/state"
	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
)

func terminalInputArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	writer := tar.NewWriter(gz)
	for name, contents := range files {
		if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: int64(len(contents))}); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(contents)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestLinuxTerminalPayloadInstallUpdateAndReceipt(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux payload")
	}
	ctx := context.Background()
	home := filepath.Join(t.TempDir(), "installer")
	t.Setenv("FESTIVAL_HOME", home)
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"obey": "obey binary", "ob": "ob binary", "obey-pty-acquire": "first helper", "install-terminal-input.sh": "installer"}
	for _, version := range []string{"1.0.0", "1.0.1"} {
		files["obey-pty-acquire"] = "helper " + version
		data := terminalInputArchive(t, files)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(data) }))
		resolved := productResolved{version: version, url: server.URL + "/obey.tar.gz", sha256: fmt.Sprintf("%x", sha256.Sum256(data)), archive: true, binaries: []string{"obey", "ob"}}
		placed, err := placeProduct(ctx, home, ObeyPackageID, "test", "stable", resolved, nil)
		server.Close()
		if err != nil || len(placed) != 4 {
			t.Fatalf("placement: %v %v", placed, err)
		}
		helper := filepath.Join(home, "bin", "libexec", "obey", "obey-pty-acquire")
		got, err := os.ReadFile(helper)
		if err != nil || string(got) != files["obey-pty-acquire"] {
			t.Fatalf("stale payload: %q %v", got, err)
		}
		db, err := state.OpenDB(ctx, home)
		if err != nil {
			t.Fatal(err)
		}
		receipt, err := receipts.Get(ctx, db.Raw(), ObeyPackageID)
		_ = db.Close(ctx)
		if err != nil || len(receipt.OwnedFiles) != 4 {
			t.Fatalf("payload missing from receipt: %+v %v", receipt, err)
		}
	}
	// A partial next release must leave the existing binaries and receipt intact.
	delete(files, "install-terminal-input.sh")
	files["obey"] = "must not replace good binary"
	data := terminalInputArchive(t, files)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(data) }))
	defer server.Close()
	_, err := placeProduct(ctx, home, ObeyPackageID, "test", "stable", productResolved{version: "1.0.2", url: server.URL + "/obey.tar.gz", sha256: fmt.Sprintf("%x", sha256.Sum256(data)), archive: true, binaries: []string{"obey", "ob"}}, nil)
	if err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("partial release accepted: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(home, "bin", "obey"))
	if err != nil || string(got) != "obey binary" {
		t.Fatalf("failed update changed binary: %q %v", got, err)
	}
	if _, err := UninstallPackage(ctx, ObeyPackageID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "bin", "libexec", "obey", "obey-pty-acquire")); !os.IsNotExist(err) {
		t.Fatalf("receipt removal left payload: %v", err)
	}
}

func TestTerminalInputAuthorizationIsNotReportedReady(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux setup")
	}
	_, managed := integrationFixture(t, "bash")
	path := filepath.Join(managed, "bin", "obey")
	for _, status := range []string{"authorization_required", "payload_missing", "update_required"} {
		script := "#!/bin/sh\nprintf '%s\\n' '{\"schema\":\"obey-terminal-input/v1\",\"status\":\"" + status + "\",\"ready\":false,\"detail\":\"Authorize input on this host.\"}'\n"
		if err := os.WriteFile(path, []byte(script), 0755); err != nil {
			t.Fatal(err)
		}
		r := IntegrationReport{Ready: true}
		reconcileTerminalInput(context.Background(), &r, true)
		expected := status
		if status == "payload_missing" {
			expected = "upgrade_required"
		}
		if r.Ready || len(r.Checks) != 1 || r.Checks[0].Status != expected || !r.Checks[0].Repairable {
			t.Fatalf("false readiness: %+v", r)
		}
	}
}

func TestTerminalPayloadRemovalCannotEscapeManagedTree(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "bin")
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(bin, "libexec"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(bin, "libexec", "obey")); err != nil {
		t.Fatal(err)
	}
	if isTerminalInputPayloadPath(filepath.Join(bin, "libexec", "obey", "obey-pty-acquire"), bin) {
		t.Fatal("payload symlink escape accepted")
	}
	if isTerminalInputPayloadPath(filepath.Join(bin, "libexec", "obey", "unrelated"), bin) {
		t.Fatal("arbitrary payload accepted")
	}
}

func TestCurrentVersionDetectsMissingPayloadForRepair(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux payload")
	}
	_, managed := integrationFixture(t, "bash")
	if !needsTerminalInputPayloadRepair(context.Background()) {
		t.Fatal("current version skipped missing payload")
	}
	directory := filepath.Join(managed, "bin", "libexec", "obey")
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"obey-pty-acquire", "install-terminal-input.sh"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("packaged"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if needsTerminalInputPayloadRepair(context.Background()) {
		t.Fatal("complete payload requested repeated download")
	}
	if err := os.Chmod(filepath.Join(directory, "obey-pty-acquire"), 0644); err != nil {
		t.Fatal(err)
	}
	if !needsTerminalInputPayloadRepair(context.Background()) {
		t.Fatal("non-executable payload skipped repair")
	}
}
