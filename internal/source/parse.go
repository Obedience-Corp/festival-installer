package source

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v5"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/metadata"
	"github.com/Obedience-Corp/festival-installer/internal/verify"
)

const manifestFilename = "obey-marketplace.json"

var (
	ErrManifestInvalid = errpkg.New("E_MANIFEST_INVALID", "marketplace manifest failed schema validation")
	ErrNotAMarketplace = errpkg.New("E_NOT_A_MARKETPLACE", "no obey-marketplace.json at repo root")
)

type ManifestError struct {
	field   string
	message string
}

func (e *ManifestError) Error() string {
	if e.field == "" {
		return "manifest invalid: " + e.message
	}
	return "manifest invalid at " + e.field + ": " + e.message
}

func (e *ManifestError) Field() string { return e.field }

func (e *ManifestError) Is(target error) bool {
	return target == ErrManifestInvalid
}

var (
	compileOnce sync.Once
	compiled    *jsonschema.Schema
	compileErr  error
)

func ensureSchema() error {
	compileOnce.Do(func() {
		data, err := schemaFS.ReadFile("schemas/marketplace.schema.json")
		if err != nil {
			compileErr = errpkg.Wrap("E_SCHEMA_LOAD", err, "read marketplace schema")
			return
		}
		compiler := jsonschema.NewCompiler()
		if err := compiler.AddResource("marketplace.json", strings.NewReader(string(data))); err != nil {
			compileErr = errpkg.Wrap("E_SCHEMA_LOAD", err, "add marketplace schema")
			return
		}
		sch, err := compiler.Compile("marketplace.json")
		if err != nil {
			compileErr = errpkg.Wrap("E_SCHEMA_COMPILE", err, "compile marketplace schema")
			return
		}
		compiled = sch
	})
	return compileErr
}

func ParseMarketplace(ctx context.Context, raw []byte) (Marketplace, error) {
	if err := ctx.Err(); err != nil {
		return Marketplace{}, errpkg.Wrap("E_PARSE_CTX", err, "context cancelled")
	}
	if err := ensureSchema(); err != nil {
		return Marketplace{}, err
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Marketplace{}, &ManifestError{message: "invalid JSON: " + err.Error()}
	}
	if err := compiled.Validate(doc); err != nil {
		var verr *jsonschema.ValidationError
		if errors.As(err, &verr) {
			return Marketplace{}, &ManifestError{field: deepestPointer(verr), message: verr.Message}
		}
		return Marketplace{}, &ManifestError{message: err.Error()}
	}
	var m Marketplace
	if err := json.Unmarshal(raw, &m); err != nil {
		return Marketplace{}, errpkg.Wrap("E_PARSE_MANIFEST", err, "decode marketplace")
	}
	return m, nil
}

// ParseVerifiedMarketplace verifies that sig is a valid signature over signed,
// which must already be the canonical bytes stored on disk, then schema-validates
// and decodes into a Marketplace.
//
// This is the only entry point downstream code should use to ingest marketplace
// metadata from an untrusted source. Calling ParseMarketplace directly on
// untrusted bytes loses the signature guarantee. Nothing here canonicalizes:
// per decision L8 the stored bytes are the signed bytes.
func ParseVerifiedMarketplace(ctx context.Context, ks verify.KeyStore, signed []byte, sig verify.Signature) (Marketplace, error) {
	if err := verify.Verify(ctx, ks, signed, sig); err != nil {
		return Marketplace{}, err
	}
	return ParseMarketplace(ctx, signed)
}

// IngestMarketplace decodes marketplace metadata under the caller's trust
// policy. When sig is non-nil the signature must verify: a present-but-invalid
// signature is E_SIG_INVALID and is never overridable. When sig is nil the
// document is unsigned, and opts decides whether that is refused or warned
// about.
//
// This mirrors metadata.IngestManifest deliberately, and reuses its options
// and its policy function, so the codebase has exactly one implementation of
// "what do we do about unsigned content." The asymmetry with IngestManifest is
// intentional: a present signature bypasses opts entirely. That is what makes
// E_SIG_INVALID non-overridable, and it is the behavior that turned a stale
// signature into a hard install failure in August 2026. Do not "fix" it by
// falling back to the consent path when a present signature fails to verify.
func IngestMarketplace(ctx context.Context, ks verify.KeyStore, raw []byte, sig *verify.Signature, opts metadata.IngestOptions) (Marketplace, error) {
	if sig != nil {
		return ParseVerifiedMarketplace(ctx, ks, raw, *sig)
	}
	if err := metadata.EnforceUnverifiedPolicy(opts); err != nil {
		return Marketplace{}, err
	}
	return ParseMarketplace(ctx, raw)
}

// LoadedMarketplace is a parsed marketplace document plus whether its
// signature was verified against the caller's key store. Verified is false
// for a document that was accepted under an allow-unverified policy.
type LoadedMarketplace struct {
	Marketplace
	Verified bool
}

// LoadMarketplace reads and verifies obey-marketplace.json from repoDir. A
// present detached signature must verify against vo.KeyStore (defaulting to
// the pinned trust store); an absent signature is subject to vo.Policy and
// vo.AllowUnverified.
func LoadMarketplace(ctx context.Context, repoDir string, vo VerifyOptions) (LoadedMarketplace, error) {
	path := filepath.Join(repoDir, manifestFilename)
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return LoadedMarketplace{}, ErrNotAMarketplace
		}
		return LoadedMarketplace{}, errpkg.Wrap("E_MANIFEST_READ", err, "read "+manifestFilename)
	}
	sig, err := loadDetachedSignature(path, manifestFilename, "E_MARKETPLACE_SIG_READ")
	if err != nil {
		return LoadedMarketplace{}, err
	}
	if vo.KeyStore == nil {
		vo.KeyStore = verify.PinnedKeyStore()
	}
	m, err := IngestMarketplace(ctx, vo.KeyStore, raw, sig, metadata.IngestOptions{
		Policy:          vo.Policy,
		AllowUnverified: vo.AllowUnverified,
		WarnWriter:      vo.WarnWriter,
		SourceLabel:     vo.SourceLabel,
	})
	if err != nil {
		return LoadedMarketplace{}, err
	}
	return LoadedMarketplace{Marketplace: m, Verified: sig != nil}, nil
}

func deepestPointer(verr *jsonschema.ValidationError) string {
	for len(verr.Causes) > 0 {
		verr = verr.Causes[0]
	}
	return verr.InstanceLocation
}
