package source_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/metadata"
	"github.com/Obedience-Corp/festival-installer/internal/source"
	"github.com/Obedience-Corp/festival-installer/internal/verify"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "marketplace", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return raw
}

func TestParseMarketplace_Valid(t *testing.T) {
	ctx := context.Background()
	m, err := source.ParseMarketplace(ctx, readFixture(t, "valid.json"))
	if err != nil {
		t.Fatalf("ParseMarketplace: %v", err)
	}
	if m.ID != "obedience-corp/official" {
		t.Fatalf("id: got %q", m.ID)
	}
	if len(m.Packages) != 2 {
		t.Fatalf("packages: got %d want 2", len(m.Packages))
	}
	if m.Packages[0].Class != "tool" || m.Packages[1].Class != "plugin" {
		t.Fatalf("classes: %q %q", m.Packages[0].Class, m.Packages[1].Class)
	}
}

func TestParseMarketplace_PluginTargetsRoundTrip(t *testing.T) {
	ctx := context.Background()
	m, err := source.ParseMarketplace(ctx, readFixture(t, "valid.json"))
	if err != nil {
		t.Fatalf("ParseMarketplace: %v", err)
	}
	plugin := m.Packages[1]
	if plugin.Class != "plugin" {
		t.Fatalf("expected plugin entry, got %+v", plugin)
	}
	if len(plugin.Targets) != 1 || plugin.Targets[0].Runtime != "fest-cli" || plugin.Targets[0].VersionConstraint != ">=0.4.0" {
		t.Fatalf("targets did not round-trip: %+v", plugin.Targets)
	}
}

func TestParseMarketplace_InvalidVariants(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		file  string
		field string
	}{
		{"missing_required.json", ""},
		{"wrong_type.json", "/schema_version"},
		{"unknown_field.json", ""},
		{"bad_class.json", "/packages/0/class"},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			_, err := source.ParseMarketplace(ctx, readFixture(t, tc.file))
			if !errors.Is(err, source.ErrManifestInvalid) {
				t.Fatalf("expected ErrManifestInvalid, got %v", err)
			}
			if tc.field != "" {
				var merr *source.ManifestError
				if !errors.As(err, &merr) {
					t.Fatalf("expected *ManifestError, got %T", err)
				}
				if merr.Field() != tc.field {
					t.Fatalf("field: got %q want %q", merr.Field(), tc.field)
				}
			}
		})
	}
}

func TestLoadMarketplace_Valid(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "obey-marketplace.json"), readFixture(t, "valid.json"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	var warn bytes.Buffer
	// An unsigned document under PolicyWarnAllow still loads; this test is
	// about the parsed content, not the verification policy, so the warn
	// writer is a buffer rather than the default stderr.
	m, err := source.LoadMarketplace(ctx, dir, source.VerifyOptions{Policy: metadata.PolicyWarnAllow, WarnWriter: &warn})
	if err != nil {
		t.Fatalf("LoadMarketplace: %v", err)
	}
	if len(m.Packages) != 2 {
		t.Fatalf("packages: got %d want 2", len(m.Packages))
	}
}

func TestLoadMarketplace_MissingFile(t *testing.T) {
	ctx := context.Background()
	if _, err := source.LoadMarketplace(ctx, t.TempDir(), source.VerifyOptions{}); !errors.Is(err, source.ErrNotAMarketplace) {
		t.Fatalf("expected ErrNotAMarketplace, got %v", err)
	}
}

// signer builds detached ed25519 signatures over exact file bytes, matching
// decision L8: LoadMarketplace verifies the bytes on disk, not a
// re-canonicalized form, so tests sign the fixture as it actually reads.
type signer struct {
	keyID string
	priv  ed25519.PrivateKey
	pub   ed25519.PublicKey
}

func newSigner(t *testing.T, keyID string) signer {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return signer{keyID: keyID, priv: priv, pub: pub}
}

func (s signer) sign(raw []byte) verify.Signature {
	return verify.Signature{KeyID: s.keyID, Algorithm: verify.AlgorithmEd25519, Bytes: ed25519.Sign(s.priv, raw)}
}

func (s signer) keyStore() verify.KeyStore {
	return verify.NewStaticStore(map[string]ed25519.PublicKey{s.keyID: s.pub})
}

func writeSigFile(t *testing.T, path string, sig verify.Signature) {
	t.Helper()
	raw, err := verify.MarshalDetachedSignature(sig)
	if err != nil {
		t.Fatalf("MarshalDetachedSignature: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write .sig: %v", err)
	}
}

// TestLoadMarketplace_VerificationCases is the sequence 05 task 01 table: nine
// cases, error cases first (1 through 8), the happy path last (9).
func TestLoadMarketplace_VerificationCases(t *testing.T) {
	ctx := context.Background()
	valid := readFixture(t, "valid.json")
	k1 := newSigner(t, "k1")

	t.Run("1_tampered_document_is_refused", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "obey-marketplace.json")
		if err := os.WriteFile(path, valid, 0o644); err != nil {
			t.Fatalf("write marketplace: %v", err)
		}
		// Sign the original bytes, then tamper the file on disk so the
		// signature no longer covers what LoadMarketplace actually reads.
		writeSigFile(t, path+".sig", k1.sign(valid))
		tampered := append(append([]byte(nil), valid...), '\n')
		if err := os.WriteFile(path, tampered, 0o644); err != nil {
			t.Fatalf("tamper marketplace: %v", err)
		}
		_, err := source.LoadMarketplace(ctx, dir, source.VerifyOptions{KeyStore: k1.keyStore()})
		if !errors.Is(err, verify.ErrSignatureInvalid) {
			t.Fatalf("expected ErrSignatureInvalid, got %v", err)
		}
	})

	t.Run("2_unknown_key_id_is_refused", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "obey-marketplace.json")
		if err := os.WriteFile(path, valid, 0o644); err != nil {
			t.Fatalf("write marketplace: %v", err)
		}
		writeSigFile(t, path+".sig", k1.sign(valid))
		k2 := newSigner(t, "k2")
		_, err := source.LoadMarketplace(ctx, dir, source.VerifyOptions{KeyStore: k2.keyStore()})
		if !errors.Is(err, verify.ErrKeyNotFound) {
			t.Fatalf("expected ErrKeyNotFound, got %v", err)
		}
	})

	t.Run("3_malformed_sig_is_refused", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "obey-marketplace.json")
		if err := os.WriteFile(path, valid, 0o644); err != nil {
			t.Fatalf("write marketplace: %v", err)
		}
		if err := os.WriteFile(path+".sig", []byte("not json"), 0o644); err != nil {
			t.Fatalf("write malformed sig: %v", err)
		}
		_, err := source.LoadMarketplace(ctx, dir, source.VerifyOptions{KeyStore: k1.keyStore()})
		// ParseDetachedSignature wraps the raw json.Unmarshal error for
		// syntactically invalid JSON rather than verify.ErrSignatureMalformed
		// (that sentinel is only used for well-formed JSON missing required
		// fields), so this checks the error code rather than errors.Is.
		if code := errpkg.Code(err); code != "E_SIG_MALFORMED" {
			t.Fatalf("error code = %q, want E_SIG_MALFORMED (err: %v)", code, err)
		}
	})

	t.Run("4_unreadable_sig_is_refused", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("running as root: file permissions are not enforced")
		}
		dir := t.TempDir()
		path := filepath.Join(dir, "obey-marketplace.json")
		if err := os.WriteFile(path, valid, 0o644); err != nil {
			t.Fatalf("write marketplace: %v", err)
		}
		writeSigFile(t, path+".sig", k1.sign(valid))
		if err := os.Chmod(path+".sig", 0o000); err != nil {
			t.Fatalf("chmod .sig: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(path+".sig", 0o644) })
		_, err := source.LoadMarketplace(ctx, dir, source.VerifyOptions{KeyStore: k1.keyStore()})
		if err == nil {
			t.Fatal("expected an error for an unreadable .sig file")
		}
		if code := errpkg.Code(err); code != "E_MARKETPLACE_SIG_READ" {
			t.Fatalf("error code = %q, want E_MARKETPLACE_SIG_READ", code)
		}
	})

	t.Run("5_no_sig_refuse_by_default", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "obey-marketplace.json"), valid, 0o644); err != nil {
			t.Fatalf("write marketplace: %v", err)
		}
		_, err := source.LoadMarketplace(ctx, dir, source.VerifyOptions{
			KeyStore: k1.keyStore(),
			Policy:   metadata.PolicyRefuseByDefault,
		})
		if !errors.Is(err, metadata.ErrUnverifiedRefused) {
			t.Fatalf("expected ErrUnverifiedRefused, got %v", err)
		}
	})

	t.Run("6_no_sig_allow_unverified", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "obey-marketplace.json"), valid, 0o644); err != nil {
			t.Fatalf("write marketplace: %v", err)
		}
		var warn bytes.Buffer
		m, err := source.LoadMarketplace(ctx, dir, source.VerifyOptions{
			KeyStore:        k1.keyStore(),
			Policy:          metadata.PolicyRefuseByDefault,
			AllowUnverified: true,
			WarnWriter:      &warn,
		})
		if err != nil {
			t.Fatalf("LoadMarketplace: %v", err)
		}
		if m.Verified {
			t.Fatal("expected Verified == false for an unsigned document accepted via --allow-unverified")
		}
		if !strings.Contains(warn.String(), "UNVERIFIED") {
			t.Fatalf("expected a loud warning, got %q", warn.String())
		}
	})

	// This is the third-party marketplace path and the most likely regression
	// in this festival, so it gets its own case rather than folding into 6.
	t.Run("7_no_sig_warn_allow_policy", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "obey-marketplace.json"), valid, 0o644); err != nil {
			t.Fatalf("write marketplace: %v", err)
		}
		var warn bytes.Buffer
		m, err := source.LoadMarketplace(ctx, dir, source.VerifyOptions{
			KeyStore:   k1.keyStore(),
			Policy:     metadata.PolicyWarnAllow,
			WarnWriter: &warn,
		})
		if err != nil {
			t.Fatalf("LoadMarketplace: %v", err)
		}
		if m.Verified {
			t.Fatal("expected Verified == false for an unsigned third-party document")
		}
		if !strings.Contains(warn.String(), "UNVERIFIED") {
			t.Fatalf("expected a loud warning, got %q", warn.String())
		}
	})

	t.Run("8_missing_document", func(t *testing.T) {
		dir := t.TempDir()
		_, err := source.LoadMarketplace(ctx, dir, source.VerifyOptions{KeyStore: k1.keyStore()})
		if !errors.Is(err, source.ErrNotAMarketplace) {
			t.Fatalf("expected ErrNotAMarketplace, got %v", err)
		}
	})

	t.Run("9_valid_signed", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "obey-marketplace.json")
		if err := os.WriteFile(path, valid, 0o644); err != nil {
			t.Fatalf("write marketplace: %v", err)
		}
		writeSigFile(t, path+".sig", k1.sign(valid))
		m, err := source.LoadMarketplace(ctx, dir, source.VerifyOptions{KeyStore: k1.keyStore()})
		if err != nil {
			t.Fatalf("LoadMarketplace: %v", err)
		}
		if !m.Verified {
			t.Fatal("expected Verified == true for a validly signed document")
		}
		if len(m.Packages) != 2 {
			t.Fatalf("packages: got %d want 2", len(m.Packages))
		}
	})
}
