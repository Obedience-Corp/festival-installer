package app

import "testing"

// Every want below is the string a shipped binary actually prints, captured on
// 2026-09-16 from camp v0.10.1-4-gd1fb37c7, fest v0.8.0-4-gdfb49547, a festival
// built with the release ldflags, and obey/ob 0.1.0.
func TestParseToolVersion_EveryShippedShape(t *testing.T) {
	campBlock := "camp v0.10.1-4-gd1fb37c7\n" +
		"commit: d1fb37c7\n" +
		"built: 2026-09-16T07:22:45Z\n" +
		"go: go1.26.7\n" +
		"platform: darwin/arm64\n" +
		"profile: dev\n"

	cases := []struct {
		name   string
		tool   string
		output string
		want   string
	}{
		{"camp version --short", "camp", "v0.10.1-4-gd1fb37c7\n", "0.10.1-4-gd1fb37c7"},
		{"camp version block", "camp", campBlock, "0.10.1-4-gd1fb37c7"},
		{"camp on a clean tag", "camp", "camp v0.10.1\n", "0.10.1"},
		{"camp on a dirty tree", "camp", "v0.10.1-4-gd1fb37c7-dirty\n", "0.10.1-4-gd1fb37c7-dirty"},
		{"fest version --short", "fest", "v0.8.0-4-gdfb49547\n", "0.8.0-4-gdfb49547"},
		{"fest version block", "fest", "fest v0.8.0\ncommit: dfb49547\n", "0.8.0"},
		{"festival release stamp", "festival", "v0.2.2\n", "0.2.2"},
		{"festival prefixed", "festival", "festival v0.2.2\n", "0.2.2"},
		{"obey version --short", "obey", "0.1.0\n", "0.1.0"},
		{"obey version block", "obey", "obey 0.1.0\ncommit: abc1234\n", "0.1.0"},
		{"obey --version cobra template", "obey", "obey version 0.1.0\n", "0.1.0"},
		{"ob --version cobra template", "ob", "ob version 0.1.0\n", "0.1.0"},
		{"a release candidate", "camp", "camp v0.11.0-rc.1\n", "0.11.0-rc.1"},
		{"leading blank lines", "fest", "\n\nfest v0.8.0\n", "0.8.0"},
		{"empty output", "camp", "", ""},
		{"whitespace only", "camp", "   \n\n", ""},
		{"a help banner", "ob", "Usage: ob [command]\nRun ob help for more.\n", ""},
		{"an error message", "obey", "unknown command \"version\" for \"obey\"\n", ""},
		{"a bare commit hash from describe --always", "camp", "d1fb37c7\n", ""},
		{"too few parts", "festival", "0.2\n", ""},
		{"a non-numeric major", "festival", "next.2.2\n", ""},
		{"the v was the whole number", "festival", "v.2.2\n", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseToolVersion(tc.tool, tc.output); got != tc.want {
				t.Fatalf("ParseToolVersion(%q, %q) = %q, want %q", tc.tool, tc.output, got, tc.want)
			}
		})
	}
}

// parseVersionText must agree with ParseToolVersion on the version, which is
// what makes `festival which` and `festival status` report one string for one
// binary, and must still read the bundle and profile rows.
func TestParseVersionText_AgreesWithTheSharedParser(t *testing.T) {
	block := "camp v0.10.1-4-gd1fb37c7\n" +
		"bundle: festival 0.2.2\n" +
		"commit: d1fb37c7\n" +
		"profile: stable\n"

	version, bundle, profile := parseVersionText("camp", block)
	if want := ParseToolVersion("camp", block); version != want {
		t.Fatalf("parseVersionText version = %q, ParseToolVersion = %q", version, want)
	}
	if version != "0.10.1-4-gd1fb37c7" {
		t.Fatalf("version = %q, want 0.10.1-4-gd1fb37c7", version)
	}
	if bundle != "0.2.2" {
		t.Fatalf("bundle = %q, want 0.2.2", bundle)
	}
	if profile != "stable" {
		t.Fatalf("profile = %q, want stable", profile)
	}
}

func TestParseVersionText_RejectsAHelpDump(t *testing.T) {
	version, _, _ := parseVersionText("camp", "Usage: camp [command]\nprofile: stable\n")
	if version != "" {
		t.Fatalf("version = %q, want empty for output that is not a version", version)
	}
}
