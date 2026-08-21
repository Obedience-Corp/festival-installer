package source

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/state"
	"github.com/Obedience-Corp/festival-installer/internal/verify"
)

// keyedRepo is a git fixture repo plus the ed25519 key material used to sign
// (or not sign) its obey-marketplace.json.
type keyedRepo struct {
	dir  string
	pub  ed25519.PublicKey
	priv ed25519.PrivateKey
}

func (r keyedRepo) keyStore() verify.KeyStore {
	return verify.NewStaticStore(map[string]ed25519.PublicKey{"k1": r.pub})
}

// newKeyedRepo builds a git fixture repo carrying seedTestManifest. When
// signed is true the document is signed with a fresh key before the first
// commit; when tamper is also true, the file on disk is modified after
// signing (but before committing) so the single commit's bytes no longer
// match the signature, matching decision L8 (sign the bytes as they are on
// disk; verification checks file bytes, not a re-canonicalized form).
func newKeyedRepo(t *testing.T, signed, tamper bool) keyedRepo {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	dir := t.TempDir()
	seedGit(t, dir, "init", "-b", "main")
	manifestPath := filepath.Join(dir, manifestFilename)
	if err := os.WriteFile(manifestPath, []byte(seedTestManifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if signed {
		sig := verify.Signature{KeyID: "k1", Algorithm: verify.AlgorithmEd25519, Bytes: ed25519.Sign(priv, []byte(seedTestManifest))}
		sigRaw, err := verify.MarshalDetachedSignature(sig)
		if err != nil {
			t.Fatalf("MarshalDetachedSignature: %v", err)
		}
		if err := os.WriteFile(manifestPath+".sig", sigRaw, 0o644); err != nil {
			t.Fatalf("write sig: %v", err)
		}
	}
	if tamper {
		tampered := append([]byte(nil), seedTestManifest...)
		tampered = append(tampered, '\n')
		if err := os.WriteFile(manifestPath, tampered, 0o644); err != nil {
			t.Fatalf("tamper manifest: %v", err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "packages", "fest"), 0o755); err != nil {
		t.Fatalf("mkdir packages: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "packages", "fest", "obey-package.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write package: %v", err)
	}
	seedGit(t, dir, "add", ".")
	seedGit(t, dir, "commit", "-m", "init")
	return keyedRepo{dir: dir, pub: pub, priv: priv}
}

func storedCommit(t *testing.T, ctx context.Context, name string) string {
	t.Helper()
	home, err := state.Home(ctx)
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	db, err := state.OpenDB(ctx, home)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer func() { _ = db.Close(ctx) }()
	src, err := Get(ctx, db.Raw(), name)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	return src.Commit
}

func cloneExists(t *testing.T, ctx context.Context, name string) bool {
	t.Helper()
	dest, err := CloneDir(ctx, name)
	if err != nil {
		t.Fatalf("CloneDir: %v", err)
	}
	_, err = os.Stat(dest)
	return err == nil
}

// Case 1: seeding against a tampered official fixture is refused and leaves
// no clone behind.
func TestManagerVerify_SeedTamperedRefusedNoClone(t *testing.T) {
	seedHome(t)
	ctx := seedTestCtx(t)
	repo := newKeyedRepo(t, true, true)

	_, err := seedFromURL(ctx, repo.dir, VerifyOptions{KeyStore: repo.keyStore()})
	if errpkg.Code(err) != "E_SIG_INVALID" {
		t.Fatalf("error code = %q, want E_SIG_INVALID (err: %v)", errpkg.Code(err), err)
	}
	if cloneExists(t, ctx, state.OfficialSeedKey) {
		t.Fatal("expected the clone to be removed after a tampered seed")
	}
}

// Case 2: seeding against an unsigned official fixture, with no override, is
// refused and leaves no clone behind.
func TestManagerVerify_SeedUnsignedRefusedNoClone(t *testing.T) {
	seedHome(t)
	ctx := seedTestCtx(t)
	fixture := seedFixtureRepo(t)

	_, err := seedFromURL(ctx, fixture, VerifyOptions{})
	if errpkg.Code(err) != "E_UNVERIFIED_REFUSED" {
		t.Fatalf("error code = %q, want E_UNVERIFIED_REFUSED (err: %v)", errpkg.Code(err), err)
	}
	if cloneExists(t, ctx, state.OfficialSeedKey) {
		t.Fatal("expected the clone to be removed after an unverified refusal")
	}
}

// Case 3: adding an unsigned third-party marketplace still works, marked
// unverified, with the warning on the caller's writer.
func TestManagerVerify_AddUnsignedThirdPartySucceedsUnverified(t *testing.T) {
	seedHome(t)
	ctx := seedTestCtx(t)
	fixture := seedFixtureRepo(t)

	var warn bytes.Buffer
	if _, err := AddMarketplace(ctx, fixture, "acme", VerifyOptions{WarnWriter: &warn}); err != nil {
		t.Fatalf("AddMarketplace: %v", err)
	}
	views, err := ListMarketplaces(ctx, VerifyOptions{WarnWriter: io.Discard})
	if err != nil {
		t.Fatalf("ListMarketplaces: %v", err)
	}
	if len(views) != 1 || views[0].Verified {
		t.Fatalf("expected one unverified view, got %+v", views)
	}
	if !bytes.Contains(warn.Bytes(), []byte("UNVERIFIED")) {
		t.Fatalf("expected a loud warning on add, got %q", warn.String())
	}
}

// Case 4: --allow-unverified (or PolicyWarnAllow) cannot launder a tampered
// document. Adding a tampered third-party fixture still fails E_SIG_INVALID,
// and the clone is removed.
func TestManagerVerify_AddTamperedThirdPartyRefusedEvenUnderWarnAllow(t *testing.T) {
	seedHome(t)
	ctx := seedTestCtx(t)
	repo := newKeyedRepo(t, true, true)

	_, err := AddMarketplace(ctx, repo.dir, "acme-tampered", VerifyOptions{KeyStore: repo.keyStore(), AllowUnverified: true})
	if errpkg.Code(err) != "E_SIG_INVALID" {
		t.Fatalf("error code = %q, want E_SIG_INVALID (err: %v)", errpkg.Code(err), err)
	}
	if cloneExists(t, ctx, "acme-tampered") {
		t.Fatal("expected the clone to be removed after a tampered add")
	}
}

// Case 5: a refresh whose new commit does not verify surfaces the failure in
// view.Err, does not mark the view changed, and leaves the stored commit
// unchanged, so a bad clone on disk is never reported as the current one.
func TestManagerVerify_RefreshUnverifiableCommitLeavesCommitUnchanged(t *testing.T) {
	seedHome(t)
	ctx := seedTestCtx(t)
	fixture := seedFixtureRepo(t)

	if _, err := AddMarketplace(ctx, fixture, "acme-refresh", VerifyOptions{WarnWriter: io.Discard}); err != nil {
		t.Fatalf("AddMarketplace: %v", err)
	}
	firstCommit := storedCommit(t, ctx, "acme-refresh")

	// Simulate a compromised remote: the new commit carries a signature that
	// does not verify against the caller's key store (unknown key id), which
	// PolicyWarnAllow must not paper over: a present signature is never
	// policy-overridable.
	_, otherPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	manifestPath := filepath.Join(fixture, manifestFilename)
	sig := verify.Signature{KeyID: "unknown-key", Algorithm: verify.AlgorithmEd25519, Bytes: ed25519.Sign(otherPriv, []byte(seedTestManifest))}
	sigRaw, err := verify.MarshalDetachedSignature(sig)
	if err != nil {
		t.Fatalf("MarshalDetachedSignature: %v", err)
	}
	if err := os.WriteFile(manifestPath+".sig", sigRaw, 0o644); err != nil {
		t.Fatalf("write sig: %v", err)
	}
	seedGit(t, fixture, "add", ".")
	seedGit(t, fixture, "commit", "-m", "compromised update")

	views, err := RefreshMarketplaces(ctx, "acme-refresh", VerifyOptions{})
	if err != nil {
		t.Fatalf("RefreshMarketplaces: %v", err)
	}
	if len(views) != 1 {
		t.Fatalf("expected one view, got %+v", views)
	}
	view := views[0]
	if view.Err == "" {
		t.Fatal("expected view.Err to be set for an unverifiable new commit")
	}
	if view.Changed {
		t.Fatal("expected view.Changed to stay false when the new commit does not verify")
	}
	if got := storedCommit(t, ctx, "acme-refresh"); got != firstCommit {
		t.Fatalf("stored commit changed despite an unverifiable refresh: got %q want %q", got, firstCommit)
	}
}

// Case 6: ListMarketplaces reports Verified accurately per source: true for a
// signed source, false for an unsigned one.
func TestManagerVerify_ListMarketplacesReportsPerSourceVerified(t *testing.T) {
	seedHome(t)
	ctx := seedTestCtx(t)
	signedRepo := newKeyedRepo(t, true, false)
	unsignedFixture := seedFixtureRepo(t)

	if _, err := AddMarketplace(ctx, signedRepo.dir, "signed-src", VerifyOptions{KeyStore: signedRepo.keyStore()}); err != nil {
		t.Fatalf("AddMarketplace signed: %v", err)
	}
	if _, err := AddMarketplace(ctx, unsignedFixture, "unsigned-src", VerifyOptions{WarnWriter: io.Discard}); err != nil {
		t.Fatalf("AddMarketplace unsigned: %v", err)
	}

	views, err := ListMarketplaces(ctx, VerifyOptions{KeyStore: signedRepo.keyStore(), WarnWriter: io.Discard})
	if err != nil {
		t.Fatalf("ListMarketplaces: %v", err)
	}
	verified := map[string]bool{}
	for _, v := range views {
		verified[v.Name] = v.Verified
	}
	if !verified["signed-src"] {
		t.Fatalf("expected signed-src to be verified, got %+v", views)
	}
	if verified["unsigned-src"] {
		t.Fatalf("expected unsigned-src to be unverified, got %+v", views)
	}
}

// Case 7: AllPackages carries each package's source Verified flag.
func TestManagerVerify_AllPackagesReportsPerSourceVerified(t *testing.T) {
	seedHome(t)
	ctx := seedTestCtx(t)
	signedRepo := newKeyedRepo(t, true, false)
	unsignedFixture := seedFixtureRepo(t)

	if _, err := AddMarketplace(ctx, signedRepo.dir, "signed-src", VerifyOptions{KeyStore: signedRepo.keyStore()}); err != nil {
		t.Fatalf("AddMarketplace signed: %v", err)
	}
	if _, err := AddMarketplace(ctx, unsignedFixture, "unsigned-src", VerifyOptions{WarnWriter: io.Discard}); err != nil {
		t.Fatalf("AddMarketplace unsigned: %v", err)
	}

	pkgs, err := AllPackages(ctx, VerifyOptions{KeyStore: signedRepo.keyStore(), WarnWriter: io.Discard})
	if err != nil {
		t.Fatalf("AllPackages: %v", err)
	}
	if len(pkgs) == 0 {
		t.Fatal("expected packages from both sources")
	}
	for _, p := range pkgs {
		switch p.Source {
		case "signed-src":
			if !p.Verified {
				t.Fatalf("expected package from signed-src to be verified: %+v", p)
			}
		case "unsigned-src":
			if p.Verified {
				t.Fatalf("expected package from unsigned-src to be unverified: %+v", p)
			}
		default:
			t.Fatalf("unexpected source %q", p.Source)
		}
	}
}
