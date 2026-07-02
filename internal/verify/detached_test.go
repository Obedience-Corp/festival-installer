package verify

import (
	"encoding/base64"
	"errors"
	"testing"
)

func TestParseDetachedSignature_Valid(t *testing.T) {
	raw := []byte(`{"key_id":"root-1","algorithm":"ed25519","signature":"` + base64.StdEncoding.EncodeToString([]byte("abc")) + `"}`)
	sig, err := ParseDetachedSignature(raw)
	if err != nil {
		t.Fatalf("ParseDetachedSignature: %v", err)
	}
	if sig.KeyID != "root-1" || sig.Algorithm != "ed25519" || string(sig.Bytes) != "abc" {
		t.Fatalf("unexpected signature: %+v", sig)
	}
}

func TestParseDetachedSignature_Invalid(t *testing.T) {
	cases := map[string]string{
		"not json":        `not json`,
		"missing fields":  `{"key_id":"root-1"}`,
		"bad base64":      `{"key_id":"root-1","algorithm":"ed25519","signature":"!!!not-base64!!!"}`,
		"empty signature": `{"key_id":"root-1","algorithm":"ed25519","signature":""}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseDetachedSignature([]byte(raw)); err == nil {
				t.Fatalf("expected an error for %q", name)
			}
		})
	}
}

func TestParseDetachedSignature_MalformedSentinel(t *testing.T) {
	_, err := ParseDetachedSignature([]byte(`{"key_id":"root-1","algorithm":"ed25519"}`))
	if !errors.Is(err, ErrSignatureMalformed) {
		t.Fatalf("expected ErrSignatureMalformed, got %v", err)
	}
}
