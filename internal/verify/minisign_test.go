package verify_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/verify"
	"github.com/Obedience-Corp/festival-installer/internal/verify/minisigntest"
)

// Real Tauri signature of Festival.App_0.1.7_aarch64.app.tar.gz from obey-releases
// release app-test-25628d122be1ab6795737af192171d227ec3f2f2.
const festivalApp017Sig = "dW50cnVzdGVkIGNvbW1lbnQ6IHNpZ25hdHVyZSBmcm9tIHRhdXJpIHNlY3JldCBrZXkKUlVUbEtmR0c0QTZJb0ZtaVcxZVBaOUt1NWw1V2t2bE40L01ITXc2US9FRXA2QVFuL05oSnUxQWM5cVAxZXc2b0JGOXVRK2UzV29NTFhKV1dLN2VHTGh3aVRMbHpERnhqMmc0PQp0cnVzdGVkIGNvbW1lbnQ6IHRpbWVzdGFtcDoxNzk0MjcxMTk5CWZpbGU6RmVzdGl2YWwgQXBwLmFwcC50YXIuZ3oKYklYSlhFT0ZOQmNuSTZvRml4QVpJNVRCNlExTXdDMTcrK2ZhY2JsbjRCU1FIcURYYXZtaE8yYUJ4RTBDTTRSaUNWcTdYSXNka3M4TzNhbWFRdlZ3Q3c9PQo="

var minisignPayload = []byte("festival app fixture payload")

const minisignComment = "timestamp:1\tfile:fixture"

func mustSigner(t *testing.T) *minisigntest.Signer {
	t.Helper()
	s, err := minisigntest.NewSigner()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestVerifyMinisign(t *testing.T) {
	tests := []struct {
		name    string
		build   func(s *minisigntest.Signer) (string, []byte)
		want    error
		wantAlg bool
	}{
		{name: "valid", build: func(s *minisigntest.Signer) (string, []byte) {
			return s.SignTauri(minisignPayload, minisignComment), minisignPayload
		}},
		{name: "tampered payload", want: verify.ErrMinisignInvalid, build: func(s *minisigntest.Signer) (string, []byte) {
			bad := append([]byte{}, minisignPayload...)
			bad[0] ^= 1
			return s.SignTauri(minisignPayload, minisignComment), bad
		}},
		{name: "foreign key same id", want: verify.ErrMinisignGlobal, build: func(s *minisigntest.Signer) (string, []byte) {
			other, _ := minisigntest.NewSignerWithKeyID(s.KeyID)
			return other.SignTauri(minisignPayload, minisignComment), minisignPayload
		}},
		{name: "different key id", want: verify.ErrMinisignKeyID, build: func(s *minisigntest.Signer) (string, []byte) {
			p := s.Parts(minisignPayload, minisignComment)
			p.KeyID[0] ^= 0xff
			return minisigntest.Encode(p), minisignPayload
		}},
		{name: "edited trusted comment", want: verify.ErrMinisignGlobal, build: func(s *minisigntest.Signer) (string, []byte) {
			p := s.Parts(minisignPayload, minisignComment)
			p.TrustedComment = "timestamp:2\tfile:evil"
			return minisigntest.Encode(p), minisignPayload
		}},
		{name: "zeroed global signature", want: verify.ErrMinisignGlobal, build: func(s *minisigntest.Signer) (string, []byte) {
			p := s.Parts(minisignPayload, minisignComment)
			p.Global = make([]byte, 64)
			return minisigntest.Encode(p), minisignPayload
		}},
		{name: "legacy Ed algorithm", want: verify.ErrMinisignAlgorithm, wantAlg: true, build: func(s *minisigntest.Signer) (string, []byte) {
			p := s.Parts(minisignPayload, minisignComment)
			p.Algorithm = "Ed"
			return minisigntest.Encode(p), minisignPayload
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := mustSigner(t)
			pub, err := verify.ParseTauriPublicKey(s.TauriPublicKey())
			if err != nil {
				t.Fatalf("public key: %v", err)
			}
			encoded, file := tc.build(s)
			sig, err := verify.ParseTauriSignature(encoded)
			if tc.wantAlg {
				if !errors.Is(err, tc.want) {
					t.Fatalf("parse error = %v, want %v", err, tc.want)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse signature: %v", err)
			}
			err = verify.VerifyMinisign(context.Background(), pub, sig, bytes.NewReader(file))
			if tc.want == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func TestParseTauriSignatureMalformed(t *testing.T) {
	s := mustSigner(t)
	good := s.Parts(minisignPayload, minisignComment)
	line := func(n int) string { return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{'E'}, n)) }
	sigLine := base64.StdEncoding.EncodeToString(append(append([]byte("ED"), good.KeyID[:]...), good.Signature...))
	globalLine := base64.StdEncoding.EncodeToString(good.Global)
	cases := map[string]string{
		"not base64":        "%%%not base64%%%",
		"empty":             "",
		"3 lines":           b64("untrusted comment: x\n" + sigLine + "\ntrusted comment: t\n"),
		"5 lines":           b64("untrusted comment: x\n" + sigLine + "\ntrusted comment: t\n" + globalLine + "\nextra\n"),
		"no trusted prefix": b64("untrusted comment: x\n" + sigLine + "\nwrong: t\n" + globalLine + "\n"),
		"73 byte signature": b64("untrusted comment: x\n" + line(73) + "\ntrusted comment: t\n" + globalLine + "\n"),
		"63 byte global":    b64("untrusted comment: x\n" + sigLine + "\ntrusted comment: t\n" + line(63) + "\n"),
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := verify.ParseTauriSignature(in)
			if !errors.Is(err, verify.ErrMinisignMalformed) {
				t.Fatalf("error = %v, want ErrMinisignMalformed", err)
			}
		})
	}
}

func TestParseTauriPublicKeyMalformed(t *testing.T) {
	keyLine := func(alg string, n int) string {
		return base64.StdEncoding.EncodeToString(append([]byte(alg), bytes.Repeat([]byte{1}, n)...))
	}
	cases := map[string]struct {
		in   string
		want error
	}{
		"not base64": {"%%%", verify.ErrMinisignMalformed},
		"empty":      {"", verify.ErrMinisignMalformed},
		"3 lines":    {b64("c: x\n" + keyLine("Ed", 40) + "\nextra\n"), verify.ErrMinisignMalformed},
		"41 bytes":   {b64("c: x\n" + keyLine("Ed", 39) + "\n"), verify.ErrMinisignMalformed},
		"alg ED":     {b64("c: x\n" + keyLine("ED", 40) + "\n"), verify.ErrMinisignAlgorithm},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := verify.ParseTauriPublicKey(tc.in)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestFestivalAppUpdaterKeyPinned(t *testing.T) {
	k := verify.FestivalAppUpdaterKey()
	if k.KeyIDString() != verify.FestivalAppUpdaterKeyID {
		t.Fatalf("key id %s, want %s", k.KeyIDString(), verify.FestivalAppUpdaterKeyID)
	}
	if len(k.Key) != 32 {
		t.Fatalf("key length %d", len(k.Key))
	}
}

func TestParseTauriSignatureFromFestivalApp017(t *testing.T) {
	sig, err := verify.ParseTauriSignature(festivalApp017Sig)
	if err != nil {
		t.Fatalf("real signature rejected: %v", err)
	}
	if sig.KeyID != verify.FestivalAppUpdaterKey().KeyID {
		t.Fatal("real signature key id differs from the pinned key")
	}
	if want := "timestamp:1794271199\tfile:Festival App.app.tar.gz"; sig.TrustedComment != want {
		t.Fatalf("trusted comment %q, want %q", sig.TrustedComment, want)
	}
	raw, _ := base64.StdEncoding.DecodeString(festivalApp017Sig)
	if !strings.HasPrefix(string(raw), "untrusted comment: ") {
		t.Fatal("unexpected signature layout")
	}
	// The global signature needs no file, so the real signature is checked
	// against the pinned key here.
	signed := append(append([]byte{}, sig.Signature...), sig.TrustedComment...)
	if !ed25519.Verify(verify.FestivalAppUpdaterKey().Key, signed, sig.GlobalSignature) {
		t.Fatal("real global signature does not verify with the pinned key")
	}
}

type failReader struct{ t *testing.T }

func (r failReader) Read([]byte) (int, error) {
	r.t.Fatal("reader was read after cancellation")
	return 0, nil
}

func TestVerifyMinisignHonorsContext(t *testing.T) {
	s := mustSigner(t)
	pub, _ := verify.ParseTauriPublicKey(s.TauriPublicKey())
	sig, _ := verify.ParseTauriSignature(s.SignTauri(minisignPayload, minisignComment))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := verify.VerifyMinisign(ctx, pub, sig, failReader{t})
	if !errors.Is(err, context.Canceled) || errpkg.Code(err) != "E_MINISIGN_CTX" {
		t.Fatalf("error = %v (code %s)", err, errpkg.Code(err))
	}
}
