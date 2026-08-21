package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/Obedience-Corp/festival-installer/internal/metadata"
	"github.com/Obedience-Corp/festival-installer/internal/source"
)

type validateFlags struct {
	kind string
	path string
}

func knownKind(kind string) error {
	switch kind {
	case "manifest", "marketplace", "index", "source":
		return nil
	default:
		return fmt.Errorf("unknown --kind %q: want manifest, marketplace, index, or source", kind)
	}
}

func parseValidateFlags(args []string, stderr io.Writer) (validateFlags, error) {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	kind := fs.String("kind", "manifest",
		"document kind: manifest, marketplace, index, or source")
	if err := fs.Parse(args); err != nil {
		return validateFlags{}, err
	}
	if err := knownKind(*kind); err != nil {
		return validateFlags{}, err
	}
	if fs.NArg() != 1 {
		return validateFlags{}, errors.New("validate requires one document path")
	}
	return validateFlags{kind: *kind, path: fs.Arg(0)}, nil
}

func validateByKind(ctx context.Context, kind string, raw []byte) error {
	if err := knownKind(kind); err != nil {
		return err
	}
	var err error
	switch kind {
	case "manifest":
		_, err = metadata.ParseManifest(ctx, raw)
	case "marketplace":
		_, err = source.ParseMarketplace(ctx, raw)
	case "index":
		_, err = metadata.ParseIndex(ctx, raw)
	case "source":
		_, err = metadata.ParseSource(ctx, raw)
	}
	return err
}

func runValidate(args []string, stdout, stderr io.Writer) error {
	vf, err := parseValidateFlags(args, stderr)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(vf.path)
	if err != nil {
		return fmt.Errorf("read document: %w", err)
	}
	if err := validateByKind(context.Background(), vf.kind, raw); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "validated %s (%s)\n", vf.path, vf.kind)
	return err
}
