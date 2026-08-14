package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Obedience-Corp/festival-installer/internal/metadata"
	"github.com/Obedience-Corp/festival-installer/internal/verify"
)

const usage = `festival-metadata manages canonical Ed25519 metadata signatures.

Usage:
  festival-metadata generate-key --private-key PATH
  festival-metadata sign --private-key PATH --key-id ID [--signature PATH] MANIFEST
  festival-metadata verify (--public-key BASE64 | --public-key-file PATH) [--signature PATH] MANIFEST
`

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		_, _ = io.WriteString(stderr, usage)
		return errors.New("command required")
	}
	switch args[0] {
	case "generate-key":
		return runGenerateKey(args[1:], stdout, stderr)
	case "sign":
		return runSign(args[1:], stdout, stderr)
	case "verify":
		return runVerify(args[1:], stdout, stderr)
	default:
		_, _ = io.WriteString(stderr, usage)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runGenerateKey(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("generate-key", flag.ContinueOnError)
	fs.SetOutput(stderr)
	privatePath := fs.String("private-key", "", "path for the base64 private key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *privatePath == "" || fs.NArg() != 0 {
		return errors.New("generate-key requires --private-key PATH")
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generate Ed25519 key: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString(priv) + "\n"
	if err := writeExclusive(*privatePath, []byte(encoded), 0o600); err != nil {
		return fmt.Errorf("write private key: %w", err)
	}
	_, err = fmt.Fprintln(stdout, base64.StdEncoding.EncodeToString(pub))
	return err
}

func runSign(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("sign", flag.ContinueOnError)
	fs.SetOutput(stderr)
	privatePath := fs.String("private-key", "", "path to a base64 Ed25519 private key")
	keyID := fs.String("key-id", "", "stable key identifier")
	signaturePath := fs.String("signature", "", "detached signature output path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *privatePath == "" || !validKeyID(*keyID) || fs.NArg() != 1 {
		return errors.New("sign requires --private-key PATH, --key-id ID, and one manifest")
	}
	manifestPath := fs.Arg(0)
	if *signaturePath == "" {
		*signaturePath = manifestPath + ".sig"
	}
	priv, err := readPrivateKey(*privatePath)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	canonical, err := verify.CanonicalizeJSON(raw)
	if err != nil {
		return err
	}
	sigRaw, err := verify.MarshalDetachedSignature(verify.Signature{
		KeyID:     *keyID,
		Algorithm: verify.AlgorithmEd25519,
		Bytes:     ed25519.Sign(priv, canonical),
	})
	if err != nil {
		return err
	}
	info, err := os.Stat(manifestPath)
	if err != nil {
		return fmt.Errorf("stat manifest: %w", err)
	}
	if err := writeAtomic(manifestPath, canonical, info.Mode().Perm()); err != nil {
		return fmt.Errorf("write canonical manifest: %w", err)
	}
	if err := writeAtomic(*signaturePath, append(sigRaw, '\n'), 0o644); err != nil {
		return fmt.Errorf("write detached signature: %w", err)
	}
	_, err = fmt.Fprintf(stdout, "signed %s with %s\n", manifestPath, *keyID)
	return err
}

func runVerify(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	publicValue := fs.String("public-key", "", "base64 Ed25519 public key")
	publicPath := fs.String("public-key-file", "", "file containing a base64 Ed25519 public key")
	signaturePath := fs.String("signature", "", "detached signature path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 || (*publicValue == "") == (*publicPath == "") {
		return errors.New("verify requires exactly one public key source and one manifest")
	}
	if *publicPath != "" {
		raw, err := os.ReadFile(*publicPath)
		if err != nil {
			return fmt.Errorf("read public key: %w", err)
		}
		*publicValue = strings.TrimSpace(string(raw))
	}
	pub, err := decodePublicKey(*publicValue)
	if err != nil {
		return err
	}
	manifestPath := fs.Arg(0)
	if *signaturePath == "" {
		*signaturePath = manifestPath + ".sig"
	}
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	canonical, err := verify.CanonicalizeJSON(raw)
	if err != nil {
		return err
	}
	if !bytes.Equal(raw, canonical) {
		return errors.New("manifest is not canonical JSON")
	}
	sigFile, err := os.ReadFile(*signaturePath)
	if err != nil {
		return fmt.Errorf("read detached signature: %w", err)
	}
	sig, err := verify.ParseDetachedSignature(sigFile)
	if err != nil {
		return err
	}
	ks := verify.NewStaticStore(map[string]ed25519.PublicKey{sig.KeyID: pub})
	if _, err := metadata.ParseVerifiedManifest(context.Background(), ks, raw, sig); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "verified %s with %s\n", manifestPath, sig.KeyID)
	return err
}

func readPrivateKey(path string) (ed25519.PrivateKey, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat private key: %w", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("private key permissions must not allow group or other access")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, errors.New("private key is not valid base64")
	}
	switch len(decoded) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(decoded), nil
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(decoded), nil
	default:
		return nil, fmt.Errorf("private key has %d bytes, want %d or %d", len(decoded), ed25519.SeedSize, ed25519.PrivateKeySize)
	}
}

func decodePublicKey(encoded string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return nil, errors.New("public key is not valid base64")
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("public key has %d bytes, want %d", len(raw), ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(raw), nil
}

func validKeyID(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func writeExclusive(path string, data []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".festival-metadata-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()
	if err := f.Chmod(mode); err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
