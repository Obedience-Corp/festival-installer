//go:build container_fs && linux

package main

import (
	"os"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	"github.com/Obedience-Corp/festival-installer/internal/apptest/appfixture"
	"github.com/Obedience-Corp/festival-installer/internal/source"
)

const fakeAppImageForTUI = `#!/bin/sh
if [ "$1" = "--appimage-extract" ]; then
  mkdir -p "squashfs-root/$(dirname "$2")" && printf PNG > "squashfs-root/$2"
  exit 0
fi
exit 0
`

// TestAppTUIHelperProcess runs the real festival command tree and TUI with the
// two trust roots swapped for a fixture catalog and a throwaway updater key.
// It returns at once unless FESTIVAL_APP_TUI_HELPER=1; the pty driver sets it.
func TestAppTUIHelperProcess(t *testing.T) {
	if os.Getenv("FESTIVAL_APP_TUI_HELPER") != "1" {
		return
	}
	c := appfixture.NewCatalogAt(t, os.Getenv("HOME"))
	rel := appfixture.NewReleases(t)
	t.Cleanup(app.WithUpdaterKeyForTest(rel.Key))
	t.Cleanup(source.WithPinnedKeyStoreForTest(c.Verify.KeyStore))
	c.Publish(appfixture.Manifest(appfixture.Release("0.2.0", "stable",
		rel.Artifact("appimage", "0.2.0", []byte(fakeAppImageForTUI)),
		rel.Artifact("deb", "0.2.0", []byte("not a real deb, never installed")),
	)), appfixture.PublishOptions{})
	c.Register("official-obey")
	os.Args = []string{"festival"}
	main()
}
