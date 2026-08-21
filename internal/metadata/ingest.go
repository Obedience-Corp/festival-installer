package metadata

import (
	"context"
	"fmt"
	"io"
	"os"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/verify"
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
	if err := EnforceUnverifiedPolicy(opts); err != nil {
		return PackageManifest{}, err
	}
	return ParseManifest(ctx, raw)
}

func EnforceUnverifiedPolicy(opts IngestOptions) error {
	if opts.Policy == PolicyRefuseByDefault && !opts.AllowUnverified {
		return errpkg.Wrap("E_UNVERIFIED_REFUSED", ErrUnverifiedRefused, label(opts.SourceLabel)+" is unsigned; pass --allow-unverified to override")
	}
	w := opts.WarnWriter
	if w == nil {
		w = os.Stderr
	}
	_, _ = fmt.Fprintf(w, "WARNING: installing UNVERIFIED content from %s (no signature)\n", label(opts.SourceLabel))
	return nil
}

func label(s string) string {
	if s == "" {
		return "source"
	}
	return s
}
