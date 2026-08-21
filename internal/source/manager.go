package source

import (
	"context"
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

// AddMarketplace clones and registers gitURL as name. The trust policy is
// chosen by policyFor(name), the same rule every read path uses: adding a
// source under the official seed's name is refused when unsigned (matching
// case 1 of the sequence's CLI test table, "marketplace add with no flag,
// official-policy source refuses"), and every other name gets
// PolicyWarnAllow so third-party marketplaces remain addable unsigned. The
// signature still fails closed on a present-but-invalid signature
// (E_SIG_INVALID), which is never overridable by either policy.
func AddMarketplace(ctx context.Context, gitURL, name string, vo VerifyOptions) (Source, error) {
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
		if _, err := LoadMarketplace(ctx, dest, voFor(name, vo)); err != nil {
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
	Verified bool   `json:"verified"`
	Err      string `json:"error,omitempty"`
}

type RefreshView struct {
	Name      string `json:"name"`
	OldCommit string `json:"old_commit"`
	NewCommit string `json:"new_commit"`
	Changed   bool   `json:"changed"`
	Err       string `json:"error,omitempty"`
}

// voFor builds the per-source VerifyOptions for name from vo, applying
// policyFor's official-vs-third-party split and setting the source label so
// refusal and warning messages name the actual source.
func voFor(name string, vo VerifyOptions) VerifyOptions {
	vo.Policy = policyFor(name)
	vo.SourceLabel = name
	return vo
}

func ListMarketplaces(ctx context.Context, vo VerifyOptions) ([]ListView, error) {
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
			m, merr := LoadMarketplace(ctx, dest, voFor(src.Name, vo))
			if merr != nil {
				view.Err = merr.Error()
				views = append(views, view)
				continue
			}
			view.Packages = len(m.Packages)
			view.Verified = m.Verified
			views = append(views, view)
		}
		return nil
	})
	return views, err
}

type BrowsePackage struct {
	Source   string     `json:"source"`
	Package  PackageRef `json:"package"`
	Verified bool       `json:"verified"`
}

func AllPackages(ctx context.Context, vo VerifyOptions) ([]BrowsePackage, error) {
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
			m, merr := LoadMarketplace(ctx, dest, voFor(src.Name, vo))
			if merr != nil {
				return errpkg.Wrap("E_BROWSE_LOAD", merr, "load marketplace "+src.Name)
			}
			for _, p := range m.Packages {
				out = append(out, BrowsePackage{Source: src.Name, Package: p, Verified: m.Verified})
			}
		}
		return nil
	})
	return out, err
}

// refreshTargets resolves name to the sources RefreshMarketplaces should
// refresh: a single source when name is given, every registered source
// otherwise.
func refreshTargets(ctx context.Context, db *state.DB, name string) ([]Source, error) {
	if name != "" {
		src, err := Get(ctx, db.Raw(), name)
		if err != nil {
			return nil, err
		}
		return []Source{src}, nil
	}
	return List(ctx, db.Raw())
}

// refreshOneSource pulls src's clone and verifies the result before
// recording it. A commit whose document does not verify must not become the
// source's recorded current commit, or the next `list` would look clean
// while the clone on disk is bad: the source stays pinned at its
// last-known-good commit until a verifying refresh succeeds.
func refreshOneSource(ctx context.Context, db *state.DB, src Source, vo VerifyOptions) RefreshView {
	view := RefreshView{Name: src.Name, OldCommit: src.Commit, NewCommit: src.Commit}
	dest, derr := CloneDir(ctx, src.Name)
	if derr != nil {
		view.Err = derr.Error()
		return view
	}
	commit, perr := Pull(ctx, dest)
	if perr != nil {
		view.Err = perr.Error()
		return view
	}
	if _, lerr := LoadMarketplace(ctx, dest, voFor(src.Name, vo)); lerr != nil {
		view.Err = lerr.Error()
		return view
	}
	if commit == src.Commit {
		return view
	}
	if uerr := UpdateCommit(ctx, db.Raw(), src.Name, commit); uerr != nil {
		view.Err = uerr.Error()
		return view
	}
	view.NewCommit = commit
	view.Changed = true
	return view
}

func RefreshMarketplaces(ctx context.Context, name string, vo VerifyOptions) ([]RefreshView, error) {
	var views []RefreshView
	err := withManager(ctx, func(ctx context.Context, db *state.DB) error {
		targets, err := refreshTargets(ctx, db, name)
		if err != nil {
			return err
		}
		for _, src := range targets {
			views = append(views, refreshOneSource(ctx, db, src, vo))
		}
		return nil
	})
	return views, err
}
