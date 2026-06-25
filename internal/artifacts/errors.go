package artifacts

import errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"

var (
	ErrChecksumMismatch = errpkg.New("E_ARTIFACT_SHA256", "sha256 does not match expected digest")
	ErrUnsafePath       = errpkg.New("E_ARTIFACT_UNSAFE_PATH", "archive member escapes destination directory")
	ErrCrossDevice      = errpkg.New("E_ARTIFACT_EXDEV", "atomic move requires source and destination on the same filesystem")
	ErrHTTPStatus       = errpkg.New("E_ARTIFACT_HTTP_STATUS", "download returned non-success HTTP status")
)
