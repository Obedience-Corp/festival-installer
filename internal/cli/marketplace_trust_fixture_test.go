package cli_test

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/source"
	"github.com/Obedience-Corp/festival-installer/internal/verify"
)

// buildFestivalMetadataBinary builds the real cmd/festival-metadata binary
// (the same tool the marketplace repo's signing pipeline uses) into a temp
// directory, so this test signs fixtures the same way a publisher does
// rather than reimplementing signing inline.
func buildFestivalMetadataBinary(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "festival-metadata")
	cmd := exec.Command("go", "build", "-o", out, "github.com/Obedience-Corp/festival-installer/cmd/festival-metadata")
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build festival-metadata: %v\n%s", err, combined)
	}
	return out
}

// generateThrowawayKey runs the real generate-key subcommand, returning the
// private key file path and the decoded public key. This key is never
// registered anywhere except the in-test key store below; it is not, and
// must never become, the production trust root.
func generateThrowawayKey(t *testing.T, metaBin string) (privatePath string, pub ed25519.PublicKey) {
	t.Helper()
	privatePath = filepath.Join(t.TempDir(), "fixture.key")
	cmd := exec.Command(metaBin, "generate-key", "--private-key", privatePath)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("generate-key: %v", err)
	}
	pubB64 := strings.TrimSpace(string(out))
	raw, err := base64.StdEncoding.DecodeString(pubB64)
	if err != nil {
		t.Fatalf("decode generated public key: %v", err)
	}
	return privatePath, ed25519.PublicKey(raw)
}

// signMarketplaceFixture runs the real sign subcommand against dir's
// obey-marketplace.json. sign rewrites the manifest to its canonical form
// and writes the detached .sig alongside it, exactly as the marketplace
// repo's publish pipeline does.
func signMarketplaceFixture(t *testing.T, metaBin, privateKeyPath, keyID, dir string) {
	t.Helper()
	manifest := filepath.Join(dir, "obey-marketplace.json")
	cmd := exec.Command(metaBin, "sign", "--private-key", privateKeyPath, "--key-id", keyID, manifest)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sign: %v\n%s", err, out)
	}
}

const fixtureMarketplaceDoc = `{
  "id": "fixture/trust-check",
  "name": "Fixture Trust Check Marketplace",
  "schema_version": "1",
  "packages": [
    {
      "id": "fixture/demo",
      "display_name": "Fixture Demo",
      "class": "tool",
      "manifest_path": "packages/fixture/demo/obey-package.json"
    }
  ]
}
`

// buildSignedFixture builds a git repository at a fresh temp dir holding a
// correctly-signed obey-marketplace.json (key id keyID, private key at
// privateKeyPath), and returns its path.
func buildSignedFixture(t *testing.T, metaBin, privateKeyPath, keyID string) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "obey-marketplace.json"), fixtureMarketplaceDoc)
	writeFile(t, filepath.Join(dir, "packages", "fixture", "demo", "obey-package.json"), "{}\n")
	signMarketplaceFixture(t, metaBin, privateKeyPath, keyID, dir)
	git(t, dir, "init", "-b", "main")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-m", "init")
	return dir
}

// buildTamperedFixture copies good's already-signed, already-canonicalized
// obey-marketplace.json and its .sig byte for byte, then mutates the
// marketplace document's content only, leaving the .sig exactly as it was.
// That is the whole point: a valid-looking signature over different bytes.
func buildTamperedFixture(t *testing.T, goodDir string) string {
	t.Helper()
	dir := t.TempDir()

	manifest, err := os.ReadFile(filepath.Join(goodDir, "obey-marketplace.json"))
	if err != nil {
		t.Fatalf("read good manifest: %v", err)
	}
	sig, err := os.ReadFile(filepath.Join(goodDir, "obey-marketplace.json.sig"))
	if err != nil {
		t.Fatalf("read good signature: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(manifest, &doc); err != nil {
		t.Fatalf("decode good manifest: %v", err)
	}
	packages, ok := doc["packages"].([]any)
	if !ok || len(packages) == 0 {
		t.Fatalf("fixture manifest missing packages: %s", manifest)
	}
	pkg, ok := packages[0].(map[string]any)
	if !ok {
		t.Fatalf("fixture package entry has unexpected shape: %v", packages[0])
	}
	pkg["id"] = "fixture/evil"
	mutated, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("re-encode mutated manifest: %v", err)
	}

	writeFile(t, filepath.Join(dir, "obey-marketplace.json"), string(mutated))
	writeFile(t, filepath.Join(dir, "obey-marketplace.json.sig"), string(sig))
	writeFile(t, filepath.Join(dir, "packages", "fixture", "demo", "obey-package.json"), "{}\n")
	git(t, dir, "init", "-b", "main")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-m", "init with tampered content, original signature")
	return dir
}

// TestMarketplaceTrustFixture_GoodAndBadFixtures drives the real CLI command
// tree (the same cli.New*Command constructors cmd/festival wires up, via
// runInstaller) against a locally signed marketplace and a copy mutated
// after signing with its .sig left intact. It proves the verification
// mechanism: signature checking, refusal, clone unwinding, and the
// "verified" field.
//
// It does NOT prove the production trust root: the fixture is signed with a
// throwaway key that this test's process never has (and must never have)
// access to sign with the real Obedience Corp private key, so the CLI's
// default pinned key store is swapped for a store that trusts the throwaway
// key for the duration of this test only (source.WithPinnedKeyStoreForTest).
// The swap is scoped to internal/source's DefaultVerifyOptions, which is
// what marketplace add/list/refresh and browse call; internal/app/doctor.go
// builds its own literal pinned key store and is deliberately left
// unswapped, so this test also observes what a real operator's `doctor`
// would show for a fixture registered this way: a source doctor considers
// reachable but not trust-verified. See task 04 for a trust-root-level
// check against the real, publicly signed marketplace.
func TestMarketplaceTrustFixture_GoodAndBadFixtures(t *testing.T) {
	metaBin := buildFestivalMetadataBinary(t)
	privateKeyPath, pub := generateThrowawayKey(t, metaBin)
	const keyID = "fixture-trust-check-1"

	goodDir := buildSignedFixture(t, metaBin, privateKeyPath, keyID)
	badDir := buildTamperedFixture(t, goodDir)

	restore := source.WithPinnedKeyStoreForTest(verify.NewStaticStore(map[string]ed25519.PublicKey{keyID: pub}))
	defer restore()

	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)

	// bad: refused, no clone left behind.
	badOut, badErrOut, badErr := runInstaller(t, "marketplace", "add", badDir, "--name", "bad")
	if badErr == nil {
		t.Fatalf("expected marketplace add bad to fail, got success: stdout=%s stderr=%s", badOut, badErrOut)
	}
	if !strings.Contains(badErr.Error(), "E_SIG_INVALID") {
		t.Fatalf("expected E_SIG_INVALID for tampered content under an intact signature, got: %v", badErr)
	}
	if _, err := os.Stat(filepath.Join(home, "marketplaces", "bad")); !os.IsNotExist(err) {
		t.Fatalf("bad clone should not remain under FESTIVAL_HOME: stat err=%v", err)
	}

	// good: succeeds and reports verified.
	goodOut, goodErrOut, goodErr := runInstaller(t, "marketplace", "add", goodDir, "--name", "good")
	if goodErr != nil {
		t.Fatalf("marketplace add good: %v\nstdout=%s stderr=%s", goodErr, goodOut, goodErrOut)
	}
	if !strings.Contains(goodOut, "added good (") {
		t.Fatalf("unexpected add-good output: %s", goodOut)
	}

	// list --json: good present and verified, bad entirely absent.
	listOut, _, listErr := runInstaller(t, "marketplace", "list", "--json")
	if listErr != nil {
		t.Fatalf("marketplace list --json: %v\n%s", listErr, listOut)
	}
	var listEnv struct {
		Data []struct {
			Name     string `json:"name"`
			Verified bool   `json:"verified"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(listOut), &listEnv); err != nil {
		t.Fatalf("decode list --json: %v\n%s", err, listOut)
	}
	var sawGood bool
	for _, v := range listEnv.Data {
		if v.Name == "bad" {
			t.Fatalf("bad should never have been registered, but list shows it: %+v", v)
		}
		if v.Name == "good" {
			sawGood = true
			if !v.Verified {
				t.Fatalf("good should report verified: %+v", v)
			}
		}
	}
	if !sawGood {
		t.Fatalf("good missing from list --json: %s", listOut)
	}

	// browse --json: valid JSON, no warning text leaked into it.
	browseOut, browseErrOut, browseErr := runInstaller(t, "browse", "--json")
	if browseErr != nil {
		t.Fatalf("browse --json: %v\nstdout=%s stderr=%s", browseErr, browseOut, browseErrOut)
	}
	var browseEnv map[string]any
	if err := json.Unmarshal([]byte(browseOut), &browseEnv); err != nil {
		t.Fatalf("browse --json is not valid JSON: %v\n%s", err, browseOut)
	}
	if strings.Contains(browseOut, "WARNING") || strings.Contains(browseOut, "UNVERIFIED") {
		t.Fatalf("browse --json output should not carry warning text: %s", browseOut)
	}

	// doctor --json: marketplace_trust present.
	doctorOut, _, _ := runInstaller(t, "doctor", "--json")
	status := doctorChecks(t, doctorOut)
	if _, ok := status["marketplace_trust"]; !ok {
		t.Fatalf("doctor --json missing marketplace_trust: %s", doctorOut)
	}

	t.Logf("fixture transcript:\nadd bad: exit-error=%v stdout=%s stderr=%s\nadd good: stdout=%s\nlist --json: %s\nbrowse --json: %s\ndoctor --json: %s\ndoctor marketplace_trust status: %s",
		badErr, badOut, badErrOut, goodOut, listOut, browseOut, doctorOut, status["marketplace_trust"])
}

// buildFestivalBinary builds the real cmd/festival binary (what `just build`
// produces) into a temp directory.
func buildFestivalBinary(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "festival")
	cmd := exec.Command("go", "build", "-o", out, "github.com/Obedience-Corp/festival-installer/cmd/festival")
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build festival: %v\n%s", err, combined)
	}
	return out
}

// runRealBinary execs the real compiled binary as a subprocess (not the
// in-process cobra tree runInstaller drives), with home as FESTIVAL_HOME.
func runRealBinary(t *testing.T, bin, home string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "FESTIVAL_HOME="+home)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

// buildUnsignedFixture builds a git repository holding an unsigned
// obey-marketplace.json (no .sig at all), to exercise the refuse-by-default
// vs warn-and-proceed policy split by source name rather than the signature
// mechanism itself.
func buildUnsignedFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "obey-marketplace.json"), fixtureMarketplaceDoc)
	writeFile(t, filepath.Join(dir, "packages", "fixture", "demo", "obey-package.json"), "{}\n")
	git(t, dir, "init", "-b", "main")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-m", "init, unsigned")
	return dir
}

// TestMarketplaceTrustFixture_UnsignedWarnsVsRefuses drives the real,
// separately-built bin/festival binary as a subprocess (not go run, not the
// in-process cobra tree) against one unsigned fixture registered under two
// different source names. This needs no key-store swap at all: it exercises
// policyFor's official-vs-third-party split against the real, unmodified
// pinned key store, since an absent signature never touches KeyStore lookup.
func TestMarketplaceTrustFixture_UnsignedWarnsVsRefuses(t *testing.T) {
	bin := buildFestivalBinary(t)
	fixture := buildUnsignedFixture(t)
	home := t.TempDir()

	// Third-party name: warns to stderr, succeeds.
	acmeOut, acmeErrOut, acmeErr := runRealBinary(t, bin, home, "marketplace", "add", fixture, "--name", "acme")
	if acmeErr != nil {
		t.Fatalf("expected unsigned third-party add to succeed with a warning, got error: %v\nstdout=%s stderr=%s", acmeErr, acmeOut, acmeErrOut)
	}
	if !strings.Contains(acmeErrOut, "WARNING") || !strings.Contains(acmeErrOut, "UNVERIFIED") {
		t.Fatalf("expected an UNVERIFIED warning on stderr, got: %s", acmeErrOut)
	}
	t.Logf("unsigned third-party (acme) transcript:\nstdout=%s\nstderr=%s", acmeOut, acmeErrOut)

	// Official name: refuses.
	offOut, offErrOut, offErr := runRealBinary(t, bin, home, "marketplace", "add", fixture, "--name", "official-obey")
	if offErr == nil {
		t.Fatalf("expected unsigned content under the official source name to be refused, got success: stdout=%s stderr=%s", offOut, offErrOut)
	}
	if !strings.Contains(offErrOut, "E_UNVERIFIED_REFUSED") {
		t.Fatalf("expected E_UNVERIFIED_REFUSED on stderr, got: %s", offErrOut)
	}
	t.Logf("unsigned official-name transcript:\nstdout=%s\nstderr=%s", offOut, offErrOut)

	// Confirm the refused official-obey clone was not left behind.
	if _, err := os.Stat(filepath.Join(home, "marketplaces", "official-obey")); !os.IsNotExist(err) {
		t.Fatalf("official-obey clone should not remain after refusal: stat err=%v", err)
	}

	// Confirm doctor --json still reflects state after this sequence, and
	// list --json/ls -la layout for the results transcript.
	listOut, _, listErr := runRealBinary(t, bin, home, "marketplace", "list", "--json")
	if listErr != nil {
		t.Fatalf("marketplace list --json: %v\n%s", listErr, listOut)
	}
	doctorOut, _, _ := runRealBinary(t, bin, home, "doctor", "--json")
	entries, err := os.ReadDir(filepath.Join(home, "marketplaces"))
	if err != nil {
		t.Fatalf("read marketplaces dir: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	t.Logf("list --json: %s\ndoctor --json: %s\nFESTIVAL_HOME/marketplaces entries: %v", listOut, doctorOut, names)
	if len(names) != 1 || names[0] != "acme" {
		t.Fatalf("expected exactly one clone (acme) to remain, got: %v", names)
	}
}
