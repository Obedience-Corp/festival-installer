package metadata

import (
	"context"
	"fmt"
	"io"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
	"github.com/Obedience-Corp/obey-installer/internal/verify"
)

type Policy int

const (
	PolicyWarnAllow Policy = iota
	PolicyRefuseByDefault
)

type IngestOptions struct {
	Policy          Policy
	AllowUnverified bool
	WarnWriter      io.Writer
	SourceLabel     string
}

var ErrUnverifiedRefused = errpkg.New("E_UNVERIFIED_REFUSED", "refusing to install unverified content")

func IngestManifest(ctx context.Context, ks verify.KeyStore, raw []byte, sig *verify.Signature, opts IngestOptions) (PackageManifest, error) {
	if sig != nil {
		return ParseVerifiedManifest(ctx, ks, raw, *sig)
	}
	if opts.Policy == PolicyRefuseByDefault && !opts.AllowUnverified {
		return PackageManifest{}, errpkg.Wrap("E_UNVERIFIED_REFUSED", ErrUnverifiedRefused, label(opts.SourceLabel)+" is unsigned; pass --allow-unverified to override")
	}
	if opts.WarnWriter != nil {
		_, _ = fmt.Fprintf(opts.WarnWriter, "WARNING: installing UNVERIFIED content from %s (no signature)\n", label(opts.SourceLabel))
	}
	return parseManifest(ctx, raw)
}

func label(s string) string {
	if s == "" {
		return "source"
	}
	return s
}
