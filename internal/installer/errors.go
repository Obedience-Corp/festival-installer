package installer

import errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"

var (
	ErrAlreadyCommitted = errpkg.New("E_INSTALL_COMMITTED", "transaction already committed")
	ErrNotStaged        = errpkg.New("E_INSTALL_NOT_STAGED", "no files staged for commit")
)
