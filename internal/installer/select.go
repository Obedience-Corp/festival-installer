package installer

import (
	"sort"
	"strconv"
	"strings"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/metadata"
)

// dirtyIdentifier is the marker `git describe --dirty` appends for a modified
// working tree.
const dirtyIdentifier = "dirty"

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
	aheadA, describeA := describeAhead(preA)
	aheadB, describeB := describeAhead(preB)
	switch {
	case describeA && describeB:
		return aheadA < aheadB
	case describeA:
		return false
	case describeB:
		return true
	}
	if preA == "" {
		return false
	}
	if preB == "" {
		return true
	}
	return lessPrerelease(preA, preB)
}

// describeAhead reads the commit count out of a `git describe --tags` suffix
// such as "4-gd1fb37c7", with any dirty marker already stripped by
// stripDirty. camp, fest, and festival stamp
// their version from git describe, so a build cut after a tag carries one.
// Semver would sort it below the tag as a pre-release; git describe means the
// opposite, and treating it as older is what makes an up-to-date machine look
// like it is below the floor.
func describeAhead(pre string) (int, bool) {
	fields := strings.Split(pre, "-")
	if len(fields) < 2 || !isHexAfterG(fields[1]) {
		return 0, false
	}
	commits, err := strconv.Atoi(fields[0])
	if err != nil || commits < 0 {
		return 0, false
	}
	return commits, true
}

// stripDirty drops the identifier `git describe --dirty` appends when the tree
// has uncommitted changes. It says nothing about which build is newer, and at a
// tag exactly it is the only field, so a bare "0.10.1-dirty" would otherwise
// reach lessPrerelease and rank below the very tag it was built from. Dropping
// it leaves "0.10.1-dirty" at its tag and "0.10.1-4-gd1fb37c7-dirty" ahead of
// it on the commit count, which is the rule festival-app applies in
// src-tauri/src/setup/versions.rs.
func stripDirty(pre string) string {
	trimmed := strings.TrimSuffix(pre, "-"+dirtyIdentifier)
	if trimmed == dirtyIdentifier {
		return ""
	}
	return trimmed
}

func isHexAfterG(field string) bool {
	rest, ok := strings.CutPrefix(field, "g")
	if !ok || rest == "" {
		return false
	}
	for _, r := range rest {
		isHex := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
		if !isHex {
			return false
		}
	}
	return true
}

func splitVersion(v string) ([3]int, string) {
	var core [3]int
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	pre := ""
	if i := strings.IndexByte(v, '-'); i >= 0 {
		pre = stripDirty(v[i+1:])
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
