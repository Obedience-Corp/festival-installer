package cli_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
)

func campBinary(version, bundle string) string {
	jsonOut := `{"version":"v` + version + `"}`
	textBundle := ""
	if bundle != "" {
		jsonOut = `{"version":"v` + version + `","bundle":"v` + bundle + `"}`
		textBundle = "  echo 'bundle: festival v" + bundle + "'\n"
	}
	return "#!/bin/sh\n" +
		"if [ \"$1\" = version ] && [ \"$2\" = --json ]; then\n  echo '" + jsonOut + "'\n  exit 0\nfi\n" +
		"if [ \"$1\" = version ] && [ \"$2\" = --short ]; then\n  echo v" + version + "\n  exit 0\nfi\n" +
		"if [ \"$1\" = version ]; then\n  echo 'camp v" + version + "'\n" + textBundle + "  exit 0\nfi\n" +
		"echo camp-" + version + "\n"
}

type suiteUpdateCase struct {
	receipt     string
	latest      string
	campVersion string
	campBundle  string
}

type suiteUpdateOutcome struct {
	Action   string
	Version  string
	From     string
	Warnings []string
	Stderr   string
}

func setupSuiteUpdate(t *testing.T, c suiteUpdateCase) (home, binDir, publishedCamp string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	binDir = filepath.Join(home, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "camp"), []byte(campBinary(c.campVersion, c.campBundle)), 0o755); err != nil {
		t.Fatalf("write managed camp: %v", err)
	}
	writeManagedBinary(t, binDir, "fest", "0.9.0")
	symlinkSelfAsManagedFestival(t, home)

	publishedCamp = campBinary("0.12.0", c.latest)
	tarball := buildSuiteTarGz(t, map[string]string{
		"camp":     publishedCamp,
		"fest":     versionedBinary("fest", "0.9.1"),
		"festival": versionedBinary("festival", "0.3.1"),
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(tarball)
	}))
	t.Cleanup(srv.Close)

	repo := fixtureInstallMarketplaceManifest(t, e2eManifest(c.latest, srv.URL+"/festival.tar.gz", sha256Hex(tarball)))
	if _, errOut, err := runInstaller(t, "marketplace", "add", repo, "--name", "official-obey", "--allow-unverified"); err != nil {
		t.Fatalf("marketplace add: %v\n%s", err, errOut)
	}
	writeFestivalReceipt(t, context.Background(), home, c.receipt, "official-obey", binDir)
	return home, binDir, publishedCamp
}

func runSuiteUpdate(t *testing.T, target string) suiteUpdateOutcome {
	t.Helper()
	out, errOut, err := runInstaller(t, "update", target, "--allow-unverified", "--json")
	if err != nil {
		t.Fatalf("update %s: %v\n%s\n%s", target, err, out, errOut)
	}
	var env struct {
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("decode envelope: %v\n%s", err, out)
	}
	var res struct {
		Action  string `json:"action"`
		Version string `json:"version"`
		From    string `json:"from"`
	}
	dataOf(t, out, &res)
	return suiteUpdateOutcome{Action: res.Action, Version: res.Version, From: res.From, Warnings: env.Warnings, Stderr: errOut}
}

func skewWarnings(o suiteUpdateOutcome) []string {
	var hits []string
	for _, w := range o.Warnings {
		if strings.Contains(w, "managed camp") {
			hits = append(hits, w)
		}
	}
	return hits
}

func TestUpdate_SuiteUpgradeComparesReceiptNotCampVersion(t *testing.T) {
	for _, target := range []string{"festival", "camp", "fest"} {
		t.Run(target, func(t *testing.T) {
			home, binDir, publishedCamp := setupSuiteUpdate(t, suiteUpdateCase{
				receipt: "0.3.9", latest: "0.3.17", campVersion: "0.9.1", campBundle: "0.3.9",
			})

			got := runSuiteUpdate(t, target)

			if got.Action != "upgraded" || got.From != "0.3.9" || got.Version != "0.3.17" {
				t.Fatalf("update %s = %s %s -> %s, want upgraded 0.3.9 -> 0.3.17 (warnings %q)", target, got.Action, got.From, got.Version, got.Warnings)
			}
			if hits := skewWarnings(got); len(hits) != 0 {
				t.Fatalf("camp shipped in the receipted suite, want no skew warning, got %q", hits)
			}
			camp, err := os.ReadFile(filepath.Join(binDir, "camp"))
			if err != nil || string(camp) != publishedCamp {
				t.Fatalf("managed camp not replaced by the 0.3.17 suite: %q err=%v", camp, err)
			}
			ctx := context.Background()
			rec, err := receipts.Get(ctx, mustDB(t, ctx, home), festivalPackageIDForTest)
			if err != nil || rec.Version != "0.3.17" {
				t.Fatalf("receipt not moved to 0.3.17: %+v err=%v", rec, err)
			}
		})
	}
}

func TestUpdate_SuiteCurrentIgnoresCampOwnVersion(t *testing.T) {
	for name, campVersion := range map[string]string{
		"camp version above the suite": "0.12.0",
		"camp version below the suite": "0.2.0",
	} {
		t.Run(name, func(t *testing.T) {
			_, binDir, _ := setupSuiteUpdate(t, suiteUpdateCase{
				receipt: "0.3.17", latest: "0.3.17", campVersion: campVersion, campBundle: "0.3.17",
			})
			before, err := os.ReadFile(filepath.Join(binDir, "camp"))
			if err != nil {
				t.Fatalf("read camp: %v", err)
			}

			got := runSuiteUpdate(t, "festival")

			if got.Action != "current" || got.Version != "0.3.17" {
				t.Fatalf("update = %s at %s, want current at 0.3.17 (warnings %q)", got.Action, got.Version, got.Warnings)
			}
			if hits := skewWarnings(got); len(hits) != 0 {
				t.Fatalf("want no skew warning, got %q", hits)
			}
			after, err := os.ReadFile(filepath.Join(binDir, "camp"))
			if err != nil || string(after) != string(before) {
				t.Fatalf("camp changed on a current suite: err=%v", err)
			}
		})
	}
}

func TestUpdate_SuiteWarnsWhenManagedCampShippedInAnotherSuite(t *testing.T) {
	_, _, _ = setupSuiteUpdate(t, suiteUpdateCase{
		receipt: "0.3.9", latest: "0.3.17", campVersion: "0.9.1", campBundle: "0.3.12",
	})

	got := runSuiteUpdate(t, "festival")

	hits := skewWarnings(got)
	if len(hits) != 1 || !strings.Contains(hits[0], "0.3.9") || !strings.Contains(hits[0], "0.3.12") {
		t.Fatalf("want one skew warning naming receipt 0.3.9 and camp's suite 0.3.12, got %q", got.Warnings)
	}
	if strings.Contains(hits[0], "0.9.1") {
		t.Fatalf("skew warning compares camp's own version, not its suite: %q", hits[0])
	}
	if !strings.Contains(got.Stderr, hits[0]) {
		t.Fatalf("skew warning missing from stderr: %q", got.Stderr)
	}
	if got.Action != "upgraded" || got.From != "0.3.9" || got.Version != "0.3.17" {
		t.Fatalf("update = %s %s -> %s, want upgraded 0.3.9 -> 0.3.17 from the receipt", got.Action, got.From, got.Version)
	}
}

func TestUpdate_SuiteNoSkewWarningWithoutBundleStamp(t *testing.T) {
	_, _, _ = setupSuiteUpdate(t, suiteUpdateCase{
		receipt: "0.3.17", latest: "0.3.17", campVersion: "0.12.0-3-gabc1234",
	})

	got := runSuiteUpdate(t, "festival")

	if hits := skewWarnings(got); len(hits) != 0 {
		t.Fatalf("a camp without a bundle stamp cannot disagree with the receipt, got %q", hits)
	}
	if got.Action != "current" || got.Version != "0.3.17" {
		t.Fatalf("update = %s at %s, want current at 0.3.17", got.Action, got.Version)
	}
}
