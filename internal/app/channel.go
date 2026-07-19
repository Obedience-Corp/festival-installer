package app

import errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"

// ValidateChannel accepts stable|rc|dev.
func ValidateChannel(c string) error {
	switch c {
	case "stable", "rc", "dev":
		return nil
	default:
		return errpkg.New("E_INSTALL_CHANNEL", "invalid channel "+c+" (expected stable, rc, or dev)")
	}
}
