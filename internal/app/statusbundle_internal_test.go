package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStatusReport_ResolvedSuiteBundle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	t.Setenv("HOMEBREW_PREFIX", "")
	active := t.TempDir()
	shadow := t.TempDir()
	// Component and suite versions deliberately use different namespaces.
	writeProbeScript(t, active, "camp", "#!/bin/sh\nif [ \"$2\" = --json ]; then echo '{\"version\":\"v0.9.1\",\"bundle\":\"v0.3.9\"}'; else echo v0.9.1; fi\n")
	writeProbeScript(t, shadow, "camp", "#!/bin/sh\nif [ \"$2\" = --json ]; then echo '{\"version\":\"v0.16.2\",\"bundle\":\"v0.3.24\"}'; else echo v0.16.2; fi\n")
	writeProbeScript(t, shadow, "fest", "#!/bin/sh\nif [ \"$2\" = --json ]; then echo '{\"version\":\"v0.9.8\",\"bundle\":\"v0.3.24\"}'; else echo v0.9.8; fi\n")
	t.Setenv("PATH", active+string(os.PathListSeparator)+shadow)

	report, err := StatusReportFor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct{ tool, path, version, bundle string }{
		{"camp", filepath.Join(active, "camp"), "0.9.1", "v0.3.9"},
		{"fest", filepath.Join(shadow, "fest"), "0.9.8", "v0.3.24"},
	} {
		var found bool
		for _, got := range report.Tools {
			if got.Tool != want.tool {
				continue
			}
			found = true
			if got.Path != want.path || got.Version != want.version || got.Bundle != want.bundle || got.ReceiptVersion != "" {
				t.Errorf("%s: got %+v; want path=%s component=%s bundle=%s without receipt", want.tool, got, want.path, want.version, want.bundle)
			}
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			var payload map[string]any
			if err := json.Unmarshal(encoded, &payload); err != nil {
				t.Fatal(err)
			}
			if payload["bundle"] != want.bundle {
				t.Errorf("wire bundle = %v, want %s", payload["bundle"], want.bundle)
			}
		}
		if !found {
			t.Errorf("missing %s status", want.tool)
		}
	}
}

func TestStatusReport_UnseenManagedCopyDoesNotInheritPathBundle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	t.Setenv("HOMEBREW_PREFIX", "")
	managed := filepath.Join(home, "bin")
	if err := os.MkdirAll(managed, 0o700); err != nil {
		t.Fatal(err)
	}
	writeProbeScript(t, managed, "camp", "#!/bin/sh\necho v0.16.2\n")
	live := t.TempDir()
	writeProbeScript(t, live, "camp", "#!/bin/sh\nif [ \"$2\" = --json ]; then echo '{\"version\":\"v0.9.1\",\"bundle\":\"v0.3.9\"}'; else echo v0.9.1; fi\n")
	t.Setenv("PATH", live)
	report, err := StatusReportFor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, got := range report.Tools {
		if got.Tool == "camp" {
			if got.Path != filepath.Join(managed, "camp") || got.Version != "0.16.2" || got.Bundle != "" {
				t.Fatalf("managed copy inherited unrelated metadata: %+v", got)
			}
			return
		}
	}
	t.Fatal("missing camp status")
}
