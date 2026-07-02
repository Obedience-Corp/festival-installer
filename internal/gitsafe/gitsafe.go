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
	return nil
}

func ConfigArgs() []string {
	return []string{"-c", "protocol.ext.allow=never", "-c", "protocol.file.allow=user"}
}

func Env() []string {
	return append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
}
