package fest_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Obedience-Corp/obey-installer/internal/hosts/fest"
	"github.com/Obedience-Corp/obey-installer/internal/metadata"
)

var featureOn = []string{fest.FeatureMarketplaceExtensionSource}

func extensionEntry() metadata.InstallEntry {
	return metadata.InstallEntry{Kind: "extension_dir", ExtensionName: "demo"}
}

func stagedExtension(t *testing.T, withManifest bool) string {
	t.Helper()
	dir := t.TempDir()
	if withManifest {
		if err := os.WriteFile(filepath.Join(dir, "extension.yml"), []byte("name: demo\nversion: \"1.0.0\"\n"), 0o644); err != nil {
			t.Fatalf("write manifest: %v", err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "templates"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "templates", "a.md"), []byte("body"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	return dir
}

func TestActivateExtension_PlacesTreeWithReceipt(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("FEST_CONFIG_DIR", cfg)
	staged := stagedExtension(t, true)

	records, err := fest.ActivateExtension(context.Background(), staged, extensionEntry(), featureOn)
	if err != nil {
		t.Fatalf("ActivateExtension: %v", err)
	}
	root := filepath.Join(cfg, "marketplace", "extensions", "demo")
	if _, err := os.Stat(filepath.Join(root, "extension.yml")); err != nil {
		t.Fatalf("extension.yml not placed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "templates", "a.md")); err != nil {
		t.Fatalf("nested file not placed: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 owned files, got %d", len(records))
	}
	want := sha256.Sum256([]byte("body"))
	wantHex := hex.EncodeToString(want[:])
	found := false
	for _, r := range records {
		if filepath.Base(r.Path) == "a.md" {
			found = true
			if r.Hash != wantHex {
				t.Fatalf("a.md hash = %s, want %s", r.Hash, wantHex)
			}
		}
	}
	if !found {
		t.Fatal("a.md not in receipt records")
	}
}

func TestActivateExtension_FeatureGateBlocks(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("FEST_CONFIG_DIR", cfg)
	staged := stagedExtension(t, true)

	_, err := fest.ActivateExtension(context.Background(), staged, extensionEntry(), nil)
	if err == nil || !strings.Contains(err.Error(), "E_HOST_FEATURE_MISSING") {
		t.Fatalf("expected E_HOST_FEATURE_MISSING, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(cfg, "marketplace", "extensions", "demo")); !os.IsNotExist(statErr) {
		t.Fatal("nothing should be written when the gate blocks")
	}
}

func TestActivateExtension_MissingManifest(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("FEST_CONFIG_DIR", cfg)
	staged := stagedExtension(t, false)

	_, err := fest.ActivateExtension(context.Background(), staged, extensionEntry(), featureOn)
	if err == nil || !strings.Contains(err.Error(), "E_EXTENSION_MANIFEST_MISSING") {
		t.Fatalf("expected E_EXTENSION_MANIFEST_MISSING, got %v", err)
	}
}

func TestActivateExtension_ConflictRejected(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("FEST_CONFIG_DIR", cfg)
	staged := stagedExtension(t, true)
	existing := filepath.Join(cfg, "marketplace", "extensions", "demo")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatalf("pre-create: %v", err)
	}

	_, err := fest.ActivateExtension(context.Background(), staged, extensionEntry(), featureOn)
	if err == nil || !strings.Contains(err.Error(), "E_EXTENSION_EXISTS") {
		t.Fatalf("expected E_EXTENSION_EXISTS, got %v", err)
	}
}

func TestActivateExtension_SymlinkRejected(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("FEST_CONFIG_DIR", cfg)
	staged := t.TempDir()
	if err := os.WriteFile(filepath.Join(staged, "extension.yml"), []byte("name: demo\n"), 0o644); err != nil {
		t.Fatalf("manifest: %v", err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(staged, "evil")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	_, err := fest.ActivateExtension(context.Background(), staged, extensionEntry(), featureOn)
	if err == nil || !strings.Contains(err.Error(), "E_TREE_UNSAFE") {
		t.Fatalf("expected E_TREE_UNSAFE, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(cfg, "marketplace", "extensions", "demo")); !os.IsNotExist(statErr) {
		t.Fatal("nothing should remain after a rejected unsafe tree")
	}
}

func TestRemoveExtension_RemovesOwnedAndPrunes(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("FEST_CONFIG_DIR", cfg)
	staged := stagedExtension(t, true)

	records, err := fest.ActivateExtension(context.Background(), staged, extensionEntry(), featureOn)
	if err != nil {
		t.Fatalf("ActivateExtension: %v", err)
	}
	sibling := filepath.Join(cfg, "marketplace", "extensions", "other")
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatalf("sibling: %v", err)
	}

	if err := fest.RemoveExtension(context.Background(), records); err != nil {
		t.Fatalf("RemoveExtension: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(cfg, "marketplace", "extensions", "demo")); !os.IsNotExist(statErr) {
		t.Fatal("demo extension dir should be pruned")
	}
	if _, statErr := os.Stat(sibling); statErr != nil {
		t.Fatalf("sibling extension should remain: %v", statErr)
	}
}
