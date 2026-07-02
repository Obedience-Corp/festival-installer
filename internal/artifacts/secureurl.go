package artifacts

import (
	"net"
	"net/http"
	"net/url"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
)

const maxRedirects = 10

func RequireHTTPS(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return errpkg.Wrap("E_ARTIFACT_INSECURE_URL", ErrInsecureURL, "parse "+rawURL)
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" && isLoopbackHost(u.Hostname()) {
		return nil
	}
	return errpkg.Wrap("E_ARTIFACT_INSECURE_URL", ErrInsecureURL, "scheme "+u.Scheme+" for "+rawURL)
}

func CheckRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return errpkg.Wrap("E_ARTIFACT_TOO_MANY_REDIRECTS", ErrTooManyRedirects, "redirect chain to "+req.URL.String())
	}
	return RequireHTTPS(req.URL.String())
}

func isLoopbackHost(host string) bool {
	// Intentional, prod-reachable carve-out: not gated to test builds
	// because the whole test suite relies on plain-http httptest servers.
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
