//go:build fixtures

package verify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestGenerateFixtures regenerates expected.canonical.json files from input.json
// in every testdata/canonical/*/ directory. Run with:
//
//   go test -tags fixtures -run TestGenerateFixtures ./internal/verify/
//
// Keep this gated behind a build tag so normal `go test` runs don't write to
// the working tree.
func TestGenerateFixtures(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "canonical")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		inPath := filepath.Join(root, e.Name(), "input.json")
		outPath := filepath.Join(root, e.Name(), "expected.canonical.json")

		raw, err := os.ReadFile(inPath)
		if err != nil {
			t.Fatalf("read %s: %v", inPath, err)
		}
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatalf("decode %s: %v", inPath, err)
		}
		canon, err := Marshal(v)
		if err != nil {
			t.Fatalf("marshal %s: %v", inPath, err)
		}
		if err := os.WriteFile(outPath, canon, 0o644); err != nil {
			t.Fatalf("write %s: %v", outPath, err)
		}
		t.Logf("wrote %s (%d bytes)", outPath, len(canon))
	}
}
