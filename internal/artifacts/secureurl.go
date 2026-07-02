package artifacts

import (
	"net"
	"net/url"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
)

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

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
