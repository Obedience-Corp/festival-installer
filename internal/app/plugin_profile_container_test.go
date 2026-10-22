//go:build container_fs && !windows

package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/artifacts"
	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
)

func TestPluginOwnershipPreservesForeignReplacementAndAllowsMissingRepair(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "camp-app")
	original := []byte("receipt-owned binary")
	if err := os.WriteFile(path, original, 0o755); err != nil {
		t.Fatal(err)
	}
	hash, err := artifacts.SHA256(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	receipt := receipts.Receipt{OwnedFiles: []receipts.OwnedFile{{Path: path, Hash: hash, Mode: 0o755}}}
	if intact, err := pluginReceiptIntact(ctx, receipt); err != nil || !intact {
		t.Fatalf("owned: %v %v", intact, err)
	}
	foreign := []byte("user replacement")
	if err := os.WriteFile(path, foreign, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := checkPluginDestination(ctx, path, receipt); errpkg.Code(err) != "E_PLUGIN_FOREIGN" {
		t.Fatalf("foreign accepted: %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(foreign) {
		t.Fatalf("foreign bytes changed: %q %v", got, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if intact, err := pluginReceiptIntact(ctx, receipt); err != nil || intact {
		t.Fatalf("missing repair: %v %v", intact, err)
	}
	if err := os.Symlink("/not/a/plugin", path); err != nil {
		t.Fatal(err)
	}
	if err := checkPluginDestination(ctx, path, receipt); errpkg.Code(err) != "E_PLUGIN_FOREIGN" {
		t.Fatalf("symlink accepted: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := checkPluginDestination(cancelled, path, receipt); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
}

func TestSelectedCampAppStatusReadsProtocolAndLeavesUnrelatedPluginsAlone(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FESTIVAL_HOME", filepath.Join(home, "installer"))
	bin := filepath.Join(home, "tools")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	plugin := filepath.Join(bin, "camp-app")
	if err := os.WriteFile(plugin, []byte("#!/bin/sh\nprintf '%s\\n' '{\"ok\":true,\"data\":{\"version\":\"0.1.0\",\"protocolMajor\":1,\"capabilities\":[\"status\",\"open\"]}}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(home, "unrelated-probed")
	if err := os.WriteFile(filepath.Join(bin, "camp-buzz"), []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	target, err := ParsePluginTarget("obedience-corp/camp-app")
	if err != nil {
		t.Fatal(err)
	}
	status, err := StatusReportForPlugins(ctx, []string{target.PackageID})
	if err != nil {
		t.Fatal(err)
	}
	entry := status.Tools[len(status.Tools)-1]
	if !entry.Present || entry.Version != "0.1.0" || entry.ProtocolMajor != 1 || len(entry.Capabilities) != 2 {
		t.Fatalf("bad selected facts: %+v", entry)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("unrelated plugin executed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "installer")); !os.IsNotExist(err) {
		t.Fatalf("status created installer state: %v", err)
	}
	if err := os.WriteFile(plugin, []byte("#!/bin/sh\nprintf '%s\\n' '{\"ok\":false,\"data\":{\"version\":\"99.0.0\"}}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	entry = selectedPluginStatus(ctx, target)
	if entry.Version != "" || entry.Error == "" {
		t.Fatalf("failed capability payload accepted: %+v", entry)
	}
}

func TestPluginOwnershipPreservesRedirectedRuntimeDirectories(t *testing.T) {
	for _, nested := range []bool{false, true} {
		t.Run(map[bool]string{false: "plugin-root", true: "nested-assets"}[nested], func(t *testing.T) {
			ctx := context.Background()
			home := t.TempDir()
			t.Setenv("HOME", home)
			extracted := t.TempDir()
			source := filepath.Join(extracted, "assets", "support")
			if err := os.MkdirAll(source, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(source, "new-file"), []byte("archive asset"), 0o644); err != nil {
				t.Fatal(err)
			}
			root := filepath.Join(home, ".obey", "plugins", "camp-app")
			redirected := root
			if nested {
				redirected = filepath.Join(root, "support")
			}
			if err := os.MkdirAll(filepath.Dir(redirected), 0o755); err != nil {
				t.Fatal(err)
			}
			external := t.TempDir()
			if err := os.Symlink(external, redirected); err != nil {
				t.Fatal(err)
			}
			// Ownership must fail before transaction staging, including when the
			// archive adds a file that has never existed under the redirected tree.
			_, err := stagePluginAssetsChecked(ctx, nil, extracted, "camp-app", nil)
			if errpkg.Code(err) != "E_PLUGIN_FOREIGN" {
				t.Fatalf("redirected directory accepted: %v", err)
			}
			entries, err := os.ReadDir(external)
			if err != nil || len(entries) != 0 {
				t.Fatalf("external runtime changed: %v %v", entries, err)
			}
			if info, err := os.Lstat(redirected); err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("symlink changed: %v", err)
			}
		})
	}
}
