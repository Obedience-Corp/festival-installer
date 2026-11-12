package installer

import (
	"strconv"
	"strings"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/metadata"
)

var (
	ErrNoAppArtifactForPlatform = errpkg.New("E_APP_PLATFORM", "no Festival App build for this operating system and architecture")
	ErrAppFormatUnavailable     = errpkg.New("E_APP_FORMAT_UNSUPPORTED", "this Festival App release has no build in the requested format")
)

// SelectAppArtifact returns the artifact of kind format for goos/goarch. It
// matches the exact arch only: App builds are never "all". The first match in
// manifest order wins, like SelectArtifact.
func SelectAppArtifact(rel metadata.Release, goos, goarch, format string) (metadata.Artifact, error) {
	if !metadata.IsAppArtifactKind(format) {
		return metadata.Artifact{}, errpkg.New("E_APP_FORMAT_UNSUPPORTED",
			"unknown Festival App format "+strconv.Quote(format)+" (expected app-bundle, nsis, deb or appimage)")
	}
	var available []string
	for _, a := range rel.Artifacts {
		if !metadata.IsAppArtifactKind(a.Kind) || a.OS != goos || a.Arch != goarch {
			continue
		}
		if a.Kind == format {
			return a, nil
		}
		available = append(available, a.Kind)
	}
	if len(available) > 0 {
		return metadata.Artifact{}, errpkg.Wrap("E_APP_FORMAT_UNSUPPORTED", ErrAppFormatUnavailable,
			"format "+format+" for "+goos+"/"+goarch+" in release "+rel.Version+"; available: "+strings.Join(available, ", "))
	}
	return metadata.Artifact{}, errpkg.Wrap("E_APP_PLATFORM", ErrNoAppArtifactForPlatform, goos+"/"+goarch+" in release "+rel.Version)
}
