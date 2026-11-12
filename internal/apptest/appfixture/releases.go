//go:build container_fs || app_native

package appfixture

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"testing"

	"crypto/sha256"
	"encoding/hex"

	"github.com/Obedience-Corp/festival-installer/internal/metadata"
	"github.com/Obedience-Corp/festival-installer/internal/verify"
	"github.com/Obedience-Corp/festival-installer/internal/verify/minisigntest"
)

// Releases serves App artifacts from a loopback server and signs them with a
// throwaway updater key. Callers install Key with app.WithUpdaterKeyForTest
// (this package cannot import internal/app).
type Releases struct {
	t      testing.TB
	srv    *httptest.Server
	signer *minisigntest.Signer
	Key    verify.MinisignPublicKey

	mu     sync.Mutex
	bodies map[string][]byte
	gets   map[string]int
}

func NewReleases(t testing.TB) *Releases {
	t.Helper()
	signer, err := minisigntest.NewSigner()
	if err != nil {
		t.Fatal(err)
	}
	key, err := verify.ParseTauriPublicKey(signer.TauriPublicKey())
	if err != nil {
		t.Fatal(err)
	}
	r := &Releases{t: t, signer: signer, Key: key, bodies: map[string][]byte{}, gets: map[string]int{}}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		body, ok := r.bodies[req.URL.Path]
		if ok && req.Method == http.MethodGet {
			r.gets[req.URL.Path]++
		}
		r.mu.Unlock()
		if !ok {
			http.NotFound(w, req)
			return
		}
		if req.Method == http.MethodHead {
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(r.srv.Close)
	return r
}

// Artifact serves body for this host's os and arch and returns the manifest
// artifact, with a real sha256 and updater signature.
func (r *Releases) Artifact(kind, version string, body []byte) metadata.Artifact {
	r.t.Helper()
	path := "/" + kind + "/" + version
	r.mu.Lock()
	r.bodies[path] = body
	r.mu.Unlock()
	sum := sha256.Sum256(body)
	a := Artifact(kind, runtime.GOOS, runtime.GOARCH)
	a.URL = r.srv.URL + path
	a.Sha256 = hex.EncodeToString(sum[:])
	a.Filename = "Festival.App_" + version + "_" + kind
	a.UpdaterSignature = r.signer.SignTauri(body, "timestamp:1\tfile:"+a.Filename)
	return a
}

// Downloads is how many GET requests the artifact for kind and version has seen.
func (r *Releases) Downloads(kind, version string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.gets["/"+kind+"/"+version]
}
