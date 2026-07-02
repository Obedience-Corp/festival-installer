package metadata

import (
	"context"

	"github.com/Obedience-Corp/obey-installer/internal/verify"
)

// ParseVerifiedSource verifies that signed is a valid ed25519 signature over
// the input bytes (which must already be canonical JSON as produced by
// verify.Marshal), then schema-validates and decodes into a Source.
//
// This is the only entry point downstream code should use to ingest source
// metadata. Bypassing it (calling ParseSource directly on untrusted bytes)
// loses the signature guarantee.
func ParseVerifiedSource(ctx context.Context, ks verify.KeyStore, signed []byte, sig verify.Signature) (Source, error) {
	if err := verify.Verify(ctx, ks, signed, sig); err != nil {
		return Source{}, err
	}
	return parseSource(ctx, signed)
}

func ParseVerifiedIndex(ctx context.Context, ks verify.KeyStore, signed []byte, sig verify.Signature) (Index, error) {
	if err := verify.Verify(ctx, ks, signed, sig); err != nil {
		return Index{}, err
	}
	return parseIndex(ctx, signed)
}

func ParseVerifiedManifest(ctx context.Context, ks verify.KeyStore, signed []byte, sig verify.Signature) (PackageManifest, error) {
	if err := verify.Verify(ctx, ks, signed, sig); err != nil {
		return PackageManifest{}, err
	}
	return parseManifest(ctx, signed)
}
