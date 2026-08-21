package source

import (
	"context"
	"os"
	"strings"
	"time"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/state"
	"github.com/Obedience-Corp/festival-installer/internal/state/lock"
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
		// TODO(task 02): placeholder policy, replaced once AddMarketplace threads vo.
		if _, err := LoadMarketplace(ctx, dest, DefaultVerifyOptions(os.Stderr, false)); err != nil {
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

type ListView struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Commit   string `json:"commit"`
	Packages int    `json:"packages"`
	Err      string `json:"error,omitempty"`
}

type RefreshView struct {
	Name      string `json:"name"`
	OldCommit string `json:"old_commit"`
	NewCommit string `json:"new_commit"`
	Changed   bool   `json:"changed"`
	Err       string `json:"error,omitempty"`
}

func ListMarketplaces(ctx context.Context) ([]ListView, error) {
	var views []ListView
	err := withManager(ctx, func(ctx context.Context, db *state.DB) error {
		sources, err := List(ctx, db.Raw())
		if err != nil {
			return err
		}
		for _, src := range sources {
			view := ListView{Name: src.Name, URL: src.URL, Commit: src.Commit}
			dest, derr := CloneDir(ctx, src.Name)
			if derr != nil {
				view.Err = derr.Error()
				views = append(views, view)
				continue
			}
			// TODO(task 02): placeholder policy, replaced once ListMarketplaces threads vo.
			m, merr := LoadMarketplace(ctx, dest, DefaultVerifyOptions(os.Stderr, false))
			if merr != nil {
				view.Err = merr.Error()
				views = append(views, view)
				continue
			}
			view.Packages = len(m.Packages)
			views = append(views, view)
		}
		return nil
	})
	return views, err
}

type BrowsePackage struct {
	Source  string     `json:"source"`
	Package PackageRef `json:"package"`
}

func AllPackages(ctx context.Context) ([]BrowsePackage, error) {
	var out []BrowsePackage
	err := withManager(ctx, func(ctx context.Context, db *state.DB) error {
		sources, err := List(ctx, db.Raw())
		if err != nil {
			return err
		}
		for _, src := range sources {
			dest, derr := CloneDir(ctx, src.Name)
			if derr != nil {
				return derr
			}
			// TODO(task 02): placeholder policy, replaced once AllPackages threads vo.
			m, merr := LoadMarketplace(ctx, dest, DefaultVerifyOptions(os.Stderr, false))
			if merr != nil {
				return errpkg.Wrap("E_BROWSE_LOAD", merr, "load marketplace "+src.Name)
			}
			for _, p := range m.Packages {
				out = append(out, BrowsePackage{Source: src.Name, Package: p})
			}
		}
		return nil
	})
	return out, err
}

func RefreshMarketplaces(ctx context.Context, name string) ([]RefreshView, error) {
	var views []RefreshView
	err := withManager(ctx, func(ctx context.Context, db *state.DB) error {
		var targets []Source
		if name != "" {
			src, err := Get(ctx, db.Raw(), name)
			if err != nil {
				return err
			}
			targets = []Source{src}
		} else {
			all, err := List(ctx, db.Raw())
			if err != nil {
				return err
			}
			targets = all
		}

		for _, src := range targets {
			view := RefreshView{Name: src.Name, OldCommit: src.Commit, NewCommit: src.Commit}
			dest, derr := CloneDir(ctx, src.Name)
			if derr != nil {
				view.Err = derr.Error()
				views = append(views, view)
				continue
			}
			commit, perr := Pull(ctx, dest)
			if perr != nil {
				view.Err = perr.Error()
				views = append(views, view)
				continue
			}
			if commit != src.Commit {
				if uerr := UpdateCommit(ctx, db.Raw(), src.Name, commit); uerr != nil {
					view.Err = uerr.Error()
					views = append(views, view)
					continue
				}
				view.NewCommit = commit
				view.Changed = true
			}
			views = append(views, view)
		}
		return nil
	})
	return views, err
}
