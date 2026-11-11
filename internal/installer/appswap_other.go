//go:build !darwin

package installer

// exchange is unsupported off darwin; D005 keeps Linux on plain rename(2).
func exchange(a, b string) error { return errExchangeUnsupported }
