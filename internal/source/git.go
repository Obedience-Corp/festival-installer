package source

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/gitsafe"
)

var (
	ErrGitClone   = errpkg.New("E_GIT_CLONE", "git clone failed")
	ErrGitPull    = errpkg.New("E_GIT_PULL", "git pull failed")
	ErrNotInCache = errpkg.New("E_NOT_IN_CACHE", "path is not within the marketplaces cache")
)

func Clone(ctx context.Context, url, dest string) (string, error) {
	if err := gitsafe.ValidateRemote(url); err != nil {
		return "", err
	}
	if _, err := run(ctx, "", "clone", "--", url, dest); err != nil {
		return "", errpkg.Wrap("E_GIT_CLONE", err, "clone "+url)
	}
	return headCommit(ctx, dest)
}

func Pull(ctx context.Context, dest string) (string, error) {
	if _, err := run(ctx, dest, "pull", "--ff-only"); err != nil {
		return "", errpkg.Wrap("E_GIT_PULL", err, "pull "+dest)
	}
	return headCommit(ctx, dest)
}

func RemoveClone(ctx context.Context, dest string) error {
	md, err := MarketplacesDir(ctx)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(md, dest)
	if err != nil {
		return errpkg.Wrap("E_NOT_IN_CACHE", err, "resolve "+dest)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return ErrNotInCache
	}
	if err := os.RemoveAll(dest); err != nil {
		return errpkg.Wrap("E_GIT_REMOVE", err, "remove "+dest)
	}
	return nil
}

func headCommit(ctx context.Context, dest string) (string, error) {
	out, err := run(ctx, dest, "rev-parse", "HEAD")
	if err != nil {
		return "", errpkg.Wrap("E_GIT_HEAD", err, "rev-parse HEAD in "+dest)
	}
	return strings.TrimSpace(out), nil
}

func run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append(gitsafe.ConfigArgs(), args...)...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = gitsafe.Env()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", errpkg.Wrap("E_GIT_EXEC", err, "git "+strings.Join(args, " ")+": "+strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
