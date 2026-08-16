package installer

import (
	"sort"
	"strconv"
	"strings"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/metadata"
)

var (
	ErrNoReleaseInChannel    = errpkg.New("E_NO_RELEASE_IN_CHANNEL", "no release found for the requested channel")
	ErrNoArtifactForPlatform = errpkg.New("E_NO_ARTIFACT_FOR_PLATFORM", "no artifact found for the requested os/arch")
)

func SelectRelease(manifest metadata.PackageManifest, channel string) (metadata.Release, error) {
	var candidates []metadata.Release
	for _, r := range manifest.Releases {
		if r.Channel == channel {
			candidates = append(candidates, r)
		}
	}
	if len(candidates) == 0 {
		return metadata.Release{}, errpkg.Wrap("E_NO_RELEASE_IN_CHANNEL", ErrNoReleaseInChannel, "channel "+channel+" in package "+manifest.ID)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return lessSemver(candidates[i].Version, candidates[j].Version)
	})
	return candidates[len(candidates)-1], nil
}

func SelectArtifact(rel metadata.Release, goos, goarch string) (metadata.Artifact, error) {
	for _, a := range rel.Artifacts {
		if a.OS == goos && a.Arch == goarch {
			return a, nil
		}
	}
	for _, a := range rel.Artifacts {
		if a.OS == goos && a.Arch == "all" {
			return a, nil
		}
	}
	return metadata.Artifact{}, errpkg.Wrap("E_NO_ARTIFACT_FOR_PLATFORM", ErrNoArtifactForPlatform, "os="+goos+" arch="+goarch+" in release "+rel.Version)
}

func VersionLess(a, b string) bool {
	return lessSemver(a, b)
}

func lessSemver(a, b string) bool {
	coreA, preA := splitVersion(a)
	coreB, preB := splitVersion(b)
	for i := range 3 {
		if coreA[i] != coreB[i] {
			return coreA[i] < coreB[i]
		}
	}
	if preA == "" && preB == "" {
		return false
	}
	if preA == "" {
		return false
	}
	if preB == "" {
		return true
	}
	return lessPrerelease(preA, preB)
}

func splitVersion(v string) ([3]int, string) {
	var core [3]int
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	pre := ""
	if i := strings.IndexByte(v, '-'); i >= 0 {
		pre = v[i+1:]
		v = v[:i]
	}
	for i, part := range strings.SplitN(v, ".", 3) {
		if i > 2 {
			break
		}
		core[i], _ = strconv.Atoi(part)
	}
	return core, pre
}

func lessPrerelease(a, b string) bool {
	idsA := strings.Split(a, ".")
	idsB := strings.Split(b, ".")
	for i := 0; i < len(idsA) && i < len(idsB); i++ {
		na, errA := strconv.Atoi(idsA[i])
		nb, errB := strconv.Atoi(idsB[i])
		switch {
		case errA == nil && errB == nil:
			if na != nb {
				return na < nb
			}
		case errA == nil:
			return true
		case errB == nil:
			return false
		default:
			if idsA[i] != idsB[i] {
				return idsA[i] < idsB[i]
			}
		}
	}
	return len(idsA) < len(idsB)
}
