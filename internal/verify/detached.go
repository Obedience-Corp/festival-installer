package verify

import (
	"encoding/base64"
	"encoding/json"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

var ErrSignatureMalformed = errpkg.New("E_SIG_MALFORMED", "detached signature is malformed")

type detachedSignature struct {
	KeyID     string `json:"key_id"`
	Algorithm string `json:"algorithm"`
	Signature string `json:"signature"`
}

func ParseDetachedSignature(raw []byte) (Signature, error) {
	var d detachedSignature
	if err := json.Unmarshal(raw, &d); err != nil {
		return Signature{}, errpkg.Wrap("E_SIG_MALFORMED", err, "decode detached signature")
	}
	if d.KeyID == "" || d.Algorithm == "" || d.Signature == "" {
		return Signature{}, errpkg.Wrap("E_SIG_MALFORMED", ErrSignatureMalformed, "missing key_id, algorithm, or signature")
	}
	bytes, err := base64.StdEncoding.DecodeString(d.Signature)
	if err != nil {
		return Signature{}, errpkg.Wrap("E_SIG_MALFORMED", err, "decode signature base64")
	}
	return Signature{KeyID: d.KeyID, Algorithm: d.Algorithm, Bytes: bytes}, nil
}
