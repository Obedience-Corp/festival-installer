//go:build app_native || (container_fs && linux)

package cli_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/source"
	"github.com/Obedience-Corp/festival-installer/internal/verify"
)

const (
	appNativeTag       = "app-test-25628d122be1ab6795737af192171d227ec3f2f2"
	appNativeVersion   = "0.1.7"
	appNativePublished = "2026-11-10T19:35:26Z"
	appNativeRoot      = "https://github.com/Obedience-Corp/obey-releases/releases/download/" + appNativeTag + "/"
	appNativeKeyID     = "fixture-app-native"
	appNativeBundleID  = "com.obediencecorp.festival-app"
	appNativeHomeMark  = "FESTIVAL_APP_NATIVE_HOME_SET"
)

type appNativeAsset struct{ Kind, OS, Arch, Name, SHA256, SigSHA256 string }

// sha256 values from the release's SHA256SUMS; .sig hashes from checksums.txt.
var appNativeAssets = []appNativeAsset{
	{"app-bundle", "darwin", "arm64", "Festival.App_0.1.7_aarch64.app.tar.gz",
		"6922d7bc662f121590dfc4a34694f78d4d0076f00f5d3cc1185980453864a46b", "117b5cdee280e04a5bf86178c114656b7c380959be9a35098516fa42f55785aa"},
	{"app-bundle", "darwin", "amd64", "Festival.App_0.1.7_x64.app.tar.gz",
		"f2bb219fafbc9cc26e6c3a271c18718a2009f1f4d53f09878d9ae495df7282ad", "3331faaf4adc264a36b2f3016f4a2c1d05419f9160c7c37e9495d83e41183109"},
	{"nsis", "windows", "amd64", "Festival.App_0.1.7_x64-setup.exe",
		"0aaa73c2b1696915ca48c819cce0e09026fc7aa04815a06c78733c1f60a2c537", "9a0dd821d00271a2e1268c6abbac295c2c8d1d81344979e617d35b98bd625a10"},
	{"deb", "linux", "amd64", "Festival.App_0.1.7_amd64.deb",
		"a8a24cdb723f9e87a9070548dd2ea9c0a80136fb1a17a9be306d3529655fc852", "0ddf08aa422e59be24e29bec4e459be444b28892a5ba9b732143420d33419209"},
	{"appimage", "linux", "amd64", "Festival.App_0.1.7_amd64.AppImage",
		"45a7ca9d8ea98d0947d15b4ca4bfcd0442a970798bdc17e9ef2babf98a21fd9d", "0900fd6f707fb22699ab5c3a1c454289cb99a8d6d6c5558d5d24749d7e160dde"},
}

type appEnvelope struct {
	OK       bool            `json:"ok"`
	Action   string          `json:"action"`
	Warnings []string        `json:"warnings"`
	Data     json.RawMessage `json:"data"`
	Error    struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// appNativeAssetFor returns the asset for kind on this host's architecture.
func appNativeAssetFor(t *testing.T, kind string) appNativeAsset {
	t.Helper()
	for _, a := range appNativeAssets {
		if a.Kind == kind && a.OS == runtime.GOOS && a.Arch == runtime.GOARCH {
			return a
		}
	}
	t.Fatalf("no %s asset for %s/%s", kind, runtime.GOOS, runtime.GOARCH)
	return appNativeAsset{}
}

// appNativeOtherAsset is any asset that is not a.
func appNativeOtherAsset(t *testing.T, a appNativeAsset) appNativeAsset {
	t.Helper()
	for _, o := range appNativeAssets {
		if o.Name != a.Name {
			return o
		}
	}
	t.Fatal("no other asset")
	return appNativeAsset{}
}

// appNativeSig reads the pinned .sig, checks its hash, and returns it unchanged.
func appNativeSig(t *testing.T, a appNativeAsset) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "app_native", a.Name+".sig"))
	if err != nil {
		t.Fatal(err)
	}
	if got := appNativeSHA(raw); got != a.SigSHA256 {
		t.Fatalf("%s.sig sha256 %s, want %s", a.Name, got, a.SigSHA256)
	}
	return string(raw)
}

// appNativeArtifact builds the manifest artifact for a, signed with sigFrom's .sig.
func appNativeArtifact(t *testing.T, a, sigFrom appNativeAsset) map[string]any {
	t.Helper()
	return map[string]any{
		"kind": a.Kind, "os": a.OS, "arch": a.Arch, "url": appNativeRoot + a.Name, "sha256": a.SHA256,
		"filename": a.Name, "updater_signature": appNativeSig(t, sigFrom),
	}
}

// appNativeKey makes the one fixture key and swaps the marketplace trust root to it.
func appNativeKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(source.WithPinnedKeyStoreForTest(verify.NewStaticStore(map[string]ed25519.PublicKey{appNativeKeyID: pub})))
	return private
}

func appNativeSignedJSON(t *testing.T, key ed25519.PrivateKey, path string, doc any) {
	t.Helper()
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := verify.CanonicalizeJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := verify.MarshalDetachedSignature(verify.Signature{KeyID: appNativeKeyID, Algorithm: verify.AlgorithmEd25519, Bytes: ed25519.Sign(key, canonical)})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, string(canonical))
	writeFile(t, path+".sig", string(sig))
}

// appNativeMarketplace signs the catalog and the App manifest into dir, a fresh git repo.
func appNativeMarketplace(t *testing.T, key ed25519.PrivateKey, dir string, artifacts []map[string]any) {
	t.Helper()
	const summary = "The desktop home for Festival: open a camp, save a plan and follow its progress."
	appNativeSignedJSON(t, key, filepath.Join(dir, "obey-marketplace.json"), map[string]any{
		"id": "fixture/app-native", "name": "App native fixture", "schema_version": "1", "namespace": "obedience-corp",
		"packages": []any{map[string]any{
			"id": "obedience-corp/festival-app", "display_name": "Festival App", "class": "product", "summary": summary,
			"channels": []string{"stable"}, "manifest_path": "packages/obedience-corp/festival-app/obey-package.json",
		}},
	})
	appNativeSignedJSON(t, key, filepath.Join(dir, "packages", "obedience-corp", "festival-app", "obey-package.json"), map[string]any{
		"schema_version": 1, "id": "obedience-corp/festival-app", "class": "product", "display_name": "Festival App", "summary": summary,
		"releases": []any{map[string]any{
			"version": appNativeVersion, "channel": "stable", "published_at": appNativePublished,
			"compatibility": map[string]any{"os": []string{"darwin", "linux", "windows"}, "arch": []string{"amd64", "arm64"}},
			"dependencies":  []any{}, "artifacts": artifacts,
			"install": map[string]any{"entries": []any{map[string]any{
				"kind": "desktop_app", "source": "festival-app", "bundle_id": appNativeBundleID, "product_name": "Festival App",
			}}},
		}},
	})
	git(t, dir, "init", "-b", "main")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-m", "app native fixture")
}

// appNativeHome makes a fresh FESTIVAL_HOME with the fixture registered as
// official-obey. HOME and XDG_DATA_HOME are isolated once per test (not on
// Windows, where NSIS resolves the real profile folders), so a second home in
// the same test sees the first home's files, which is what adoption needs.
func appNativeHome(t *testing.T, marketplace string) string {
	t.Helper()
	if os.Getenv(appNativeHomeMark) == "" {
		t.Setenv(appNativeHomeMark, "1")
		if runtime.GOOS != "windows" {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("XDG_DATA_HOME", "")
		}
	}
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	t.Setenv("OBEY_INSTALLER_HOME", "")
	if out, errOut, err := runInstaller(t, "marketplace", "add", marketplace, "--name", "official-obey"); err != nil {
		t.Fatalf("marketplace add: %v\n%s\n%s", err, out, errOut)
	}
	return home
}

// appNativeJSON runs the command and decodes stdout. A failure envelope is returned, not fatal.
func appNativeJSON(t *testing.T, args ...string) appEnvelope {
	t.Helper()
	out, errOut, err := runInstaller(t, args...)
	var env appEnvelope
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatalf("%v: no JSON envelope on stdout (err=%v)\nstdout: %s\nstderr: %s", args, err, out, errOut)
	}
	return env
}

func appNativeStep(t *testing.T, name string) {
	t.Helper()
	t.Logf("=== STEP %s", name)
}

func appNativeOK(t *testing.T, step string, env appEnvelope) {
	t.Helper()
	if !env.OK {
		t.Fatalf("step %s: expected ok, got %s: %s", step, env.Error.Code, env.Error.Message)
	}
}

func appNativeFail(t *testing.T, step, code string, env appEnvelope) {
	t.Helper()
	if env.OK || env.Error.Code != code {
		t.Fatalf("step %s: expected %s, got ok=%v code=%s message=%s", step, code, env.OK, env.Error.Code, env.Error.Message)
	}
}

type appNativeResult struct {
	Package         string `json:"package"`
	Version         string `json:"version"`
	PreviousVersion string `json:"previous_version"`
	Format          string `json:"format"`
	Location        string `json:"location"`
	Adopted         bool   `json:"adopted"`
}

func appNativeResultOf(t *testing.T, step string, env appEnvelope) appNativeResult {
	t.Helper()
	appNativeOK(t, step, env)
	var r appNativeResult
	if err := json.Unmarshal(env.Data, &r); err != nil {
		t.Fatalf("step %s: %v", step, err)
	}
	return r
}

type appNativeStatus struct {
	Tools []struct {
		Tool string `json:"tool"`
	} `json:"tools"`
	App struct {
		Installed       bool    `json:"installed"`
		Version         *string `json:"version"`
		ReceiptVersion  *string `json:"receipt_version"`
		Format          *string `json:"format"`
		Location        *string `json:"location"`
		UpdatedByApp    *bool   `json:"updated_by_app"`
		Latest          *string `json:"latest"`
		UpdateAvailable *bool   `json:"update_available"`
	} `json:"app"`
}

func appNativeStatusOf(t *testing.T, step string) appNativeStatus {
	t.Helper()
	env := appNativeJSON(t, "status", "--json")
	appNativeOK(t, step, env)
	var s appNativeStatus
	if err := json.Unmarshal(env.Data, &s); err != nil {
		t.Fatalf("step %s: %v", step, err)
	}
	return s
}

func appNativeToolOrder(t *testing.T, step string, s appNativeStatus) {
	t.Helper()
	var names []string
	for _, tool := range s.Tools {
		names = append(names, tool.Tool)
	}
	if strings.Join(names, ",") != "camp,fest,festival,obey,ob" {
		t.Fatalf("step %s: tools = %v, want camp, fest, festival, obey, ob", step, names)
	}
}

func str(p *string) string {
	if p == nil {
		return "<null>"
	}
	return *p
}

func appNativeSHA(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
