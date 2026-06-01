package source

import (
	"context"
	"strings"
	"time"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
	"github.com/Obedience-Corp/obey-installer/internal/state"
	"github.com/Obedience-Corp/obey-installer/internal/state/lock"
)

const lockTimeout = 30 * time.Second

func DeriveName(gitURL string) string {
	trimmed := strings.TrimRight(gitURL, "/")
	if i := strings.LastIndexAny(trimmed, "/:"); i >= 0 {
		trimmed = trimmed[i+1:]
	}
	return strings.TrimSuffix(trimmed, ".git")
}

func withManager(ctx context.Context, fn func(ctx context.Context, db *state.DB) error) error {
	home, err := state.Home(ctx)
	if err != nil {
		return err
	}
	if err := state.EnsureHome(ctx, 0o700); err != nil {
		return err
	}
	fl, err := lock.NewFileLock(home)
	if err != nil {
		return err
	}
	release, err := fl.Acquire(ctx, lockTimeout)
	if err != nil {
		return errpkg.Wrap("E_LOCK_ACQUIRE", err, "acquire installer lock")
	}
	defer func() { _ = release() }()

	db, err := state.OpenDB(ctx, home)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close(ctx) }()

	return fn(ctx, db)
}

func AddMarketplace(ctx context.Context, gitURL, name string) (Source, error) {
	if name == "" {
		name = DeriveName(gitURL)
	}
	if err := validateName(name); err != nil {
		return Source{}, err
	}

	var added Source
	err := withManager(ctx, func(ctx context.Context, db *state.DB) error {
		dest, err := CloneDir(ctx, name)
		if err != nil {
			return err
		}
		commit, err := Clone(ctx, gitURL, dest)
		if err != nil {
			return err
		}
		if _, err := LoadMarketplace(ctx, dest); err != nil {
			_ = RemoveClone(ctx, dest)
			return err
		}
		src := Source{Name: name, URL: gitURL, Commit: commit, AddedAt: time.Now().UTC()}
		if err := Add(ctx, db.Raw(), src); err != nil {
			_ = RemoveClone(ctx, dest)
			return err
		}
		added = src
		return nil
	})
	return added, err
}

func RemoveMarketplace(ctx context.Context, name string) error {
	return withManager(ctx, func(ctx context.Context, db *state.DB) error {
		if _, err := Get(ctx, db.Raw(), name); err != nil {
			return err
		}
		dest, err := CloneDir(ctx, name)
		if err != nil {
			return err
		}
		if err := RemoveClone(ctx, dest); err != nil {
			return err
		}
		return Remove(ctx, db.Raw(), name)
	})
}

func ListMarketplaces(ctx context.Context) ([]Source, error) {
	var out []Source
	err := withManager(ctx, func(ctx context.Context, db *state.DB) error {
		sources, err := List(ctx, db.Raw())
		if err != nil {
			return err
		}
		out = sources
		return nil
	})
	return out, err
}

func RefreshMarketplace(ctx context.Context, name string) (Source, error) {
	var refreshed Source
	err := withManager(ctx, func(ctx context.Context, db *state.DB) error {
		src, err := Get(ctx, db.Raw(), name)
		if err != nil {
			return err
		}
		dest, err := CloneDir(ctx, name)
		if err != nil {
			return err
		}
		commit, err := Pull(ctx, dest)
		if err != nil {
			return err
		}
		if err := UpdateCommit(ctx, db.Raw(), name, commit); err != nil {
			return err
		}
		src.Commit = commit
		refreshed = src
		return nil
	})
	return refreshed, err
}
