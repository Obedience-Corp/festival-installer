// Package minisigntest produces minisign/Tauri signatures for tests. Only
// _test.go files import it, so it never links into the binary.
package minisigntest

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"

	"golang.org/x/crypto/blake2b"
)

type Parts struct {
	Algorithm        string // "ED" normally; tests set "Ed" to model a legacy signature
	KeyID            [8]byte
	Signature        []byte
	UntrustedComment string
	TrustedComment   string
	Global           []byte
}

type Signer struct {
	KeyID   [8]byte
	Public  ed25519.PublicKey
	private ed25519.PrivateKey
}

func NewSigner() (*Signer, error) {
	var id [8]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, err
	}
	return NewSignerWithKeyID(id)
}

func NewSignerWithKeyID(id [8]byte) (*Signer, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &Signer{KeyID: id, Public: pub, private: priv}, nil
}

func (s *Signer) keyIDHex() string {
	return fmt.Sprintf("%016X", binary.LittleEndian.Uint64(s.KeyID[:]))
}

// TauriPublicKey is the base64 of the two-line minisign public key file.
func (s *Signer) TauriPublicKey() string {
	line := append(append([]byte("Ed"), s.KeyID[:]...), s.Public...)
	file := "untrusted comment: minisign public key: " + s.keyIDHex() + "\n" +
		base64.StdEncoding.EncodeToString(line) + "\n"
	return base64.StdEncoding.EncodeToString([]byte(file))
}

// Parts signs payload and returns the unencoded pieces so tests can tamper.
func (s *Signer) Parts(payload []byte, trustedComment string) Parts {
	digest := blake2b.Sum512(payload)
	sig := ed25519.Sign(s.private, digest[:])
	global := ed25519.Sign(s.private, append(append([]byte{}, sig...), []byte(trustedComment)...))
	return Parts{
		Algorithm:        "ED",
		KeyID:            s.KeyID,
		Signature:        sig,
		UntrustedComment: "signature from tauri secret key",
		TrustedComment:   trustedComment,
		Global:           global,
	}
}

func (s *Signer) SignTauri(payload []byte, trustedComment string) string {
	return Encode(s.Parts(payload, trustedComment))
}

// Encode renders Parts as the base64 of the four-line signature file.
func Encode(p Parts) string {
	line := append(append([]byte(p.Algorithm), p.KeyID[:]...), p.Signature...)
	file := "untrusted comment: " + p.UntrustedComment + "\n" +
		base64.StdEncoding.EncodeToString(line) + "\n" +
		"trusted comment: " + p.TrustedComment + "\n" +
		base64.StdEncoding.EncodeToString(p.Global) + "\n"
	return base64.StdEncoding.EncodeToString([]byte(file))
}
