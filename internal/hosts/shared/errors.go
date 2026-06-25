package shared

import errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"

var ErrEmptyName = errpkg.New("E_HOST_EMPTY_NAME", "binary name must not be empty")
