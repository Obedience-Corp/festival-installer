//go:build container_fs || app_native

// The helpers import nothing from this module: internal/installer's in-package
// tests import apptest, and internal/source imports internal/release, which
// imports internal/installer, so a helper here that imported source would be an
// import cycle. The signed catalog builder lives in internal/apptest/appfixture
// (04_lifecycle/02) for that reason.
package apptest

import (
	"os"
	"testing"
)

// RequireHostedRunner fails the test at once unless it runs on a fresh
// GitHub-hosted runner. app_native tests replace /Applications bundles, run NSIS
// and apt-get, and write under the real HOME, so they must never run on a
// developer machine (D014). It fails rather than skips because every native
// lane treats a skip as a failure. Same guard as obey-releases
// verify-app-fresh-setup.yml:120-126.
func RequireHostedRunner(t testing.TB) {
	t.Helper()
	if os.Getenv("GITHUB_ACTIONS") != "true" || os.Getenv("RUNNER_ENVIRONMENT") != "github-hosted" {
		t.Fatalf("app_native tests run only on fresh GitHub-hosted runners (GITHUB_ACTIONS=%q RUNNER_ENVIRONMENT=%q)",
			os.Getenv("GITHUB_ACTIONS"), os.Getenv("RUNNER_ENVIRONMENT"))
	}
}
