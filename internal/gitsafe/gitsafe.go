package gitsafe

import (
	"os"
	"strings"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
)

var ErrUnsafeRemote = errpkg.New("E_GIT_UNSAFE_REMOTE", "git remote uses a disallowed scheme or form")

func ValidateRemote(remote string) error {
	if remote == "" || strings.HasPrefix(remote, "-") {
		return errpkg.Wrap("E_GIT_UNSAFE_REMOTE", ErrUnsafeRemote, "leading dash or empty: "+remote)
	}
	if strings.Contains(remote, "::") {
		return errpkg.Wrap("E_GIT_UNSAFE_REMOTE", ErrUnsafeRemote, "transport helper form: "+remote)
	}
	if scheme, _, ok := strings.Cut(remote, "://"); ok {
		switch scheme {
		case "https", "ssh":
			return nil
		case "file":
			// Intentional, prod-reachable carve-out: `marketplace add` supports a
			// local repository source and ConfigArgs sets protocol.file.allow=user
			// to permit it; the clone and ls-remote tests also fixture from local
			// repos. http, git, ftp, and every other scheme fall through to reject.
			return nil
		default:
			return errpkg.Wrap("E_GIT_UNSAFE_REMOTE", ErrUnsafeRemote, "disallowed scheme "+scheme+"://: "+remote)
		}
	}
	if isSCPLike(remote) {
		return nil
	}
	if strings.HasPrefix(remote, "/") {
		// Same prod-reachable carve-out as the file:// case above, restricted to
		// absolute paths so a bare relative token cannot be mistaken for a repo.
		return nil
	}
	return errpkg.Wrap("E_GIT_UNSAFE_REMOTE", ErrUnsafeRemote, "unrecognized remote form: "+remote)
}

// isSCPLike reports whether remote uses git's scp-like ssh form, [user@]host:path.
// Following git, the colon must precede any slash and leave a non-empty host, so
// an absolute path that happens to contain a colon stays a local path.
func isSCPLike(remote string) bool {
	colon := strings.IndexByte(remote, ':')
	if colon <= 0 {
		return false
	}
	if slash := strings.IndexByte(remote, '/'); slash >= 0 && slash < colon {
		return false
	}
	return true
}

func ConfigArgs() []string {
	return []string{"-c", "protocol.ext.allow=never", "-c", "protocol.file.allow=user"}
}

func Env() []string {
	return append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
}
