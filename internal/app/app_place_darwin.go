//go:build darwin

package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

func hostAppPlatform(context.Context) (appPlatform, error) {
	home, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(home) {
		return nil, errpkg.New("E_APP_PLATFORM", "cannot find the home directory for ~/Applications")
	}
	return newBundlePlatform("/Applications", filepath.Join(home, "Applications"), os.Getuid(), darwinOwner, darwinWritable, codesignVerify), nil
}

func darwinOwner(path string) (int, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, errpkg.New("E_APP_PROBE", "no owner for "+path)
	}
	return int(st.Uid), nil
}

func darwinWritable(path string) bool { return unix.Access(path, unix.W_OK) == nil }

// codesignVerify runs codesign --verify; it never signs and never touches
// extended attributes.
func codesignVerify(ctx context.Context, bundle string) error {
	res, err := runAppCommand(ctx, 2*time.Minute, "", nil, "/usr/bin/codesign", "--verify", "--deep", "--strict", bundle)
	if err != nil {
		return errpkg.Wrap("E_APP_BUNDLE_INVALID", err, "run codesign")
	}
	if res.ExitCode != 0 {
		tail := res.Stderr
		if len(tail) > 1024 {
			tail = tail[len(tail)-1024:]
		}
		return errpkg.New("E_APP_BUNDLE_INVALID", "codesign --verify failed: "+strings.TrimSpace(tail))
	}
	return nil
}
