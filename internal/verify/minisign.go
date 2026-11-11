package verify

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/crypto/blake2b"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

// FestivalAppUpdaterKeyID is the Festival App Tauri updater key (D004).
const FestivalAppUpdaterKeyID = "A0880EE086F129E5"

// festivalAppUpdaterPublicKey is festival-app src-tauri/tauri.conf.json
// plugins.updater.pubkey at 3464e98. Rotating it needs an installer release.
const festivalAppUpdaterPublicKey = "dW50cnVzdGVkIGNvbW1lbnQ6IG1pbmlzaWduIHB1YmxpYyBrZXk6IEEwODgwRUUwODZGMTI5RTUKUldUbEtmR0c0QTZJb0NSc0ZwYmNwZjJ5Sjd2MEQ1Q0ZSYjE3ZjhnSy9uM0xXZGQ5eDFOS0ErbXMK"

const (
	minisignChunk       = 1 << 20
	minisignTrustedLead = "trusted comment: "
	minisignSigAlg      = "ED"
	minisignKeyAlg      = "Ed"
)

var (
	ErrMinisignMalformed = errpkg.New("E_MINISIGN_MALFORMED", "minisign data is malformed")
	ErrMinisignAlgorithm = errpkg.New("E_MINISIGN_ALG", "minisign algorithm not supported")
	ErrMinisignKeyID     = errpkg.New("E_MINISIGN_KEY_ID", "minisign signature was made with a different key")
	ErrMinisignInvalid   = errpkg.New("E_MINISIGN_INVALID", "minisign signature does not match the file")
	ErrMinisignGlobal    = errpkg.New("E_MINISIGN_GLOBAL", "minisign trusted comment signature does not verify")
)

type MinisignPublicKey struct {
	KeyID [8]byte
	Key   ed25519.PublicKey
}

// KeyIDString renders the key id the way minisign and Tauri print it.
func (k MinisignPublicKey) KeyIDString() string {
	return fmt.Sprintf("%016X", binary.LittleEndian.Uint64(k.KeyID[:]))
}

type MinisignSignature struct {
	KeyID           [8]byte
	Signature       []byte // 64 bytes over BLAKE2b-512(file)
	TrustedComment  string // text after "trusted comment: "
	GlobalSignature []byte // 64 bytes over Signature || TrustedComment
}

func malformed(detail string) error {
	return errpkg.Wrap("E_MINISIGN_MALFORMED", ErrMinisignMalformed, detail)
}

// decodeTauriLines base64-decodes a Tauri-wrapped minisign file and returns
// its lines, requiring exactly want of them.
func decodeTauriLines(encoded string, want int) ([]string, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return nil, malformed("outer base64: " + err.Error())
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if len(lines) != want {
		return nil, malformed(fmt.Sprintf("expected %d lines, got %d", want, len(lines)))
	}
	for i := range lines {
		lines[i] = strings.TrimSuffix(lines[i], "\r")
	}
	return lines, nil
}

func decodeLine(line string, size int, what string) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(line)
	if err != nil {
		return nil, malformed(what + " base64: " + err.Error())
	}
	if len(b) != size {
		return nil, malformed(fmt.Sprintf("%s is %d bytes, want %d", what, len(b), size))
	}
	return b, nil
}

// ParseTauriPublicKey parses the base64 minisign public key Tauri stores in
// tauri.conf.json.
func ParseTauriPublicKey(encoded string) (MinisignPublicKey, error) {
	lines, err := decodeTauriLines(encoded, 2)
	if err != nil {
		return MinisignPublicKey{}, err
	}
	b, err := decodeLine(lines[1], 2+8+ed25519.PublicKeySize, "public key")
	if err != nil {
		return MinisignPublicKey{}, err
	}
	if string(b[:2]) != minisignKeyAlg {
		return MinisignPublicKey{}, errpkg.Wrap("E_MINISIGN_ALG", ErrMinisignAlgorithm, "public key algorithm "+fmt.Sprintf("%q", b[:2]))
	}
	var pub MinisignPublicKey
	copy(pub.KeyID[:], b[2:10])
	pub.Key = ed25519.PublicKey(append([]byte{}, b[10:]...))
	return pub, nil
}

// FestivalAppUpdaterKey returns the pinned Festival App updater key. It panics
// on a bad embedded constant, like mustPublicKey.
func FestivalAppUpdaterKey() MinisignPublicKey {
	pub, err := ParseTauriPublicKey(festivalAppUpdaterPublicKey)
	if err != nil {
		panic("invalid embedded Festival App updater public key: " + err.Error())
	}
	return pub
}

// ParseTauriSignature parses the base64 .sig text Tauri publishes beside an
// update artifact.
func ParseTauriSignature(encoded string) (MinisignSignature, error) {
	lines, err := decodeTauriLines(encoded, 4)
	if err != nil {
		return MinisignSignature{}, err
	}
	b, err := decodeLine(lines[1], 2+8+ed25519.SignatureSize, "signature")
	if err != nil {
		return MinisignSignature{}, err
	}
	comment, ok := strings.CutPrefix(lines[2], minisignTrustedLead)
	if !ok {
		return MinisignSignature{}, malformed("missing trusted comment line")
	}
	global, err := decodeLine(lines[3], ed25519.SignatureSize, "global signature")
	if err != nil {
		return MinisignSignature{}, err
	}
	if string(b[:2]) != minisignSigAlg {
		return MinisignSignature{}, errpkg.Wrap("E_MINISIGN_ALG", ErrMinisignAlgorithm, "signature algorithm "+fmt.Sprintf("%q", b[:2])+" (only prehashed ED is supported)")
	}
	sig := MinisignSignature{TrustedComment: comment, GlobalSignature: global, Signature: append([]byte{}, b[10:]...)}
	copy(sig.KeyID[:], b[2:10])
	return sig, nil
}

// VerifyMinisign checks sig against file with pub. The cheap global signature
// is checked before the file is hashed.
func VerifyMinisign(ctx context.Context, pub MinisignPublicKey, sig MinisignSignature, file io.Reader) error {
	if err := ctx.Err(); err != nil {
		return errpkg.Wrap("E_MINISIGN_CTX", err, "minisign verification cancelled")
	}
	if sig.KeyID != pub.KeyID {
		other := MinisignPublicKey{KeyID: sig.KeyID}
		return errpkg.Wrap("E_MINISIGN_KEY_ID", ErrMinisignKeyID, "signature key "+other.KeyIDString()+", expected "+pub.KeyIDString())
	}
	if len(sig.Signature) != ed25519.SignatureSize || len(sig.GlobalSignature) != ed25519.SignatureSize || len(pub.Key) != ed25519.PublicKeySize {
		return malformed("signature or key has the wrong length")
	}
	signed := append(append([]byte{}, sig.Signature...), sig.TrustedComment...)
	if !ed25519.Verify(pub.Key, signed, sig.GlobalSignature) {
		return errpkg.Wrap("E_MINISIGN_GLOBAL", ErrMinisignGlobal, "trusted comment is not signed by "+pub.KeyIDString())
	}
	h, err := blake2b.New512(nil)
	if err != nil {
		return errpkg.Wrap("E_MINISIGN_MALFORMED", err, "create blake2b hash")
	}
	buf := make([]byte, minisignChunk)
	for {
		if err := ctx.Err(); err != nil {
			return errpkg.Wrap("E_MINISIGN_CTX", err, "minisign verification cancelled")
		}
		n, rerr := file.Read(buf)
		h.Write(buf[:n])
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return errpkg.Wrap("E_MINISIGN_READ", rerr, "read file for minisign verification")
		}
	}
	if !ed25519.Verify(pub.Key, h.Sum(nil), sig.Signature) {
		return errpkg.Wrap("E_MINISIGN_INVALID", ErrMinisignInvalid, "file digest does not match the signature")
	}
	return nil
}

// VerifyTauriSignatureFile verifies the file at path against the Tauri .sig text.
func VerifyTauriSignatureFile(ctx context.Context, pub MinisignPublicKey, encodedSig, path string) error {
	sig, err := ParseTauriSignature(encodedSig)
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return errpkg.Wrap("E_MINISIGN_READ", err, "open "+path)
	}
	defer func() { _ = f.Close() }()
	return VerifyMinisign(ctx, pub, sig, f)
}
