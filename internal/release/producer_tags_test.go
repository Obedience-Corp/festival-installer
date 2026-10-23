package release

import (
	"context"
	"testing"
)

func TestProducerNamespaceSeparatesSharedPublicReleases(t *testing.T) {
	const tags = "a refs/tags/v99.0.0\na refs/tags/app-v20.0.0\na refs/tags/camp-app-v0.1.0\na refs/tags/camp-app-v0.2.0-rc.1\na refs/tags/fest-gif-v8.0.0\na refs/tags/camp-app-vgarbage\na refs/tags/notes\n"
	for _, tc := range []struct {
		name, prefix, channel, tag, version string
		fail                                bool
	}{
		{"legacy plain tags", "", "stable", "v99.0.0", "99.0.0", false},
		{"plugin stable", "camp-app-v", "stable", "camp-app-v0.1.0", "0.1.0", false},
		{"plugin channel", "camp-app-v", "rc", "camp-app-v0.2.0-rc.1", "0.2.0-rc.1", false},
		{"other producer", "fest-gif-v", "stable", "fest-gif-v8.0.0", "8.0.0", false},
		{"absent producer", "camp-gh-v", "stable", "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tag, version, err := selectProducerTag(tags, tc.prefix, tc.channel)
			if (err != nil) != tc.fail || tag != tc.tag || version != tc.version {
				t.Fatalf("got %q %q %v", tag, version, err)
			}
		})
	}
}

func TestProducerResolutionCancelsBeforeGit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := NewResolver().latestTagForSource(ctx, Source{Repo: "https://example.invalid/release.git", TagPrefix: "camp-app-v"}, "stable")
	if err == nil {
		t.Fatal("cancelled resolution succeeded")
	}
}
