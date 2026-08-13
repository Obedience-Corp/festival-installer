package shared

import (
	"strings"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

var (
	ErrEmptyName  = errpkg.New("E_HOST_EMPTY_NAME", "binary name must not be empty")
	ErrUnsafeName = errpkg.New("E_HOST_UNSAFE_NAME", "name must be a single path segment without separators or ..")
)

func ValidateSegment(name string) error {
	if strings.TrimSpace(name) == "" {
		return ErrEmptyName
	}
	if name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return ErrUnsafeName
	}
	return nil
}
