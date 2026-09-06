package app

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

// fakeFest puts a fest on PATH that prints body on stdout and records the
// directory it was run in, so a test can check both the answer the hub read and
// the camp it asked about.
//
// The script uses only shell builtins. PATH here holds nothing but this fake, so
// a script calling cat or printf would find neither and print nothing at all,
// which reads as an empty listing instead of a broken fixture.
func fakeFest(t *testing.T, body string, exit int) (cwdFile string) {
	t.Helper()
	dir := t.TempDir()
	cwdFile = filepath.Join(dir, "cwd")
	script := "#!/bin/sh\npwd > " + cwdFile + "\necho '" + body + "'\nexit " + strconv.Itoa(exit) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "fest"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake fest: %v", err)
	}
	t.Setenv("PATH", dir)
	return cwdFile
}

// festEnv points the installer home at an empty dir so ResolveTool finds the
// fake on PATH rather than a managed binary or the developer's own install.
func festEnv(t *testing.T) {
	t.Helper()
	t.Setenv("FESTIVAL_HOME", t.TempDir())
	t.Setenv("OBEY_INSTALLER_HOME", "")
	t.Setenv("PATH", t.TempDir())
}

func TestResolveFestivalDir_PrefersActiveThenReadyThenPlanning(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "active wins over everything",
			body: `{"active":[{"name":"a","path":"/camp/festivals/active/a","status":"active"}],` +
				`"ready":[{"name":"r","path":"/camp/festivals/ready/r","status":"ready"}],` +
				`"planning":[{"name":"p","path":"/camp/festivals/planning/p","status":"planning"}],"total":3}`,
			want: "/camp/festivals/active/a",
		},
		{
			name: "ready wins when nothing is active",
			body: `{"ready":[{"name":"r","path":"/camp/festivals/ready/r","status":"ready"}],` +
				`"planning":[{"name":"p","path":"/camp/festivals/planning/p","status":"planning"}],"total":2}`,
			want: "/camp/festivals/ready/r",
		},
		{
			name: "planning is used as a last resort",
			body: `{"planning":[{"name":"p","path":"/camp/festivals/planning/p","status":"planning"}],"total":1}`,
			want: "/camp/festivals/planning/p",
		},
		{
			name: "the first entry in a bucket wins",
			body: `{"active":[{"name":"one","path":"/camp/one","status":"active"},` +
				`{"name":"two","path":"/camp/two","status":"active"}],"total":2}`,
			want: "/camp/one",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			festEnv(t)
			fakeFest(t, tc.body, 0)
			got, err := ResolveFestivalDir(context.Background(), t.TempDir())
			if err != nil {
				t.Fatalf("ResolveFestivalDir: %v", err)
			}
			if got != tc.want {
				t.Fatalf("dir = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestResolveFestivalDir_IgnoresParkedAndRitual pins the deliberate exclusion. A
// parked festival was set aside and a ritual is recurring machinery, so neither
// is the festival a new user came here to work in.
func TestResolveFestivalDir_IgnoresParkedAndRitual(t *testing.T) {
	festEnv(t)
	fakeFest(t, `{"parked":[{"name":"pk","path":"/camp/parked/pk","status":"parked"}],`+
		`"ritual":[{"name":"daily","path":"/camp/ritual/daily","status":"ritual"}],"total":2}`, 0)

	_, err := ResolveFestivalDir(context.Background(), t.TempDir())
	if got := errpkg.Code(err); got != CodeNoFestival {
		t.Fatalf("code = %q, want %q (err=%v)", got, CodeNoFestival, err)
	}
}

// TestResolveFestivalDir_TotalDoesNotBreakTheParse is the regression test for
// decoding fest's listing as a map of arrays. fest puts an integer total beside
// the status buckets, which makes that parse fail on every real camp.
func TestResolveFestivalDir_TotalDoesNotBreakTheParse(t *testing.T) {
	festEnv(t)
	fakeFest(t, `{"active":[{"name":"a","path":"/camp/a","status":"active"}],`+
		`"residents":{"active":[{"name":"someone"}]},"total":13}`, 0)

	got, err := ResolveFestivalDir(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("ResolveFestivalDir: %v", err)
	}
	if got != "/camp/a" {
		t.Fatalf("dir = %q, want /camp/a", got)
	}
}

func TestResolveFestivalDir_EmptyCampHasNoFestival(t *testing.T) {
	bodies := []struct {
		name string
		body string
	}{
		{"an empty document", `{"total":0}`},
		{"no output at all", ``},
		{"a json null", `null`},
		{"buckets present but empty", `{"active":[],"ready":[],"planning":[],"total":0}`},
	}
	for _, tc := range bodies {
		t.Run(tc.name, func(t *testing.T) {
			festEnv(t)
			fakeFest(t, tc.body, 0)
			_, err := ResolveFestivalDir(context.Background(), t.TempDir())
			if got := errpkg.Code(err); got != CodeNoFestival {
				t.Fatalf("code = %q, want %q (err=%v)", got, CodeNoFestival, err)
			}
		})
	}
}

func TestResolveFestivalDir_AsksInsideTheCamp(t *testing.T) {
	festEnv(t)
	cwdFile := fakeFest(t, `{"active":[{"name":"a","path":"/camp/a","status":"active"}],"total":1}`, 0)
	camp := t.TempDir()

	if _, err := ResolveFestivalDir(context.Background(), camp); err != nil {
		t.Fatalf("ResolveFestivalDir: %v", err)
	}
	raw, err := os.ReadFile(cwdFile)
	if err != nil {
		t.Fatalf("read recorded cwd: %v", err)
	}
	got, err := filepath.EvalSymlinks(trimTrailingNewlines(string(raw)))
	if err != nil {
		t.Fatalf("resolve recorded cwd: %v", err)
	}
	want, err := filepath.EvalSymlinks(camp)
	if err != nil {
		t.Fatalf("resolve camp: %v", err)
	}
	if got != want {
		t.Fatalf("fest ran in %q, want the camp root %q", got, want)
	}
}

func TestResolveFestivalDir_NoCampRootIsItsOwnAnswer(t *testing.T) {
	festEnv(t)
	fakeFest(t, `{"total":0}`, 0)

	_, err := ResolveFestivalDir(context.Background(), "  ")
	if got := errpkg.Code(err); got != CodeNoCampRoot {
		t.Fatalf("code = %q, want %q (err=%v)", got, CodeNoCampRoot, err)
	}
}

func TestResolveFestivalDir_UnreadableOutputIsNotNoFestival(t *testing.T) {
	festEnv(t)
	fakeFest(t, `this is not json`, 0)

	_, err := ResolveFestivalDir(context.Background(), t.TempDir())
	if got := errpkg.Code(err); got != CodeFestivalList {
		t.Fatalf("code = %q, want %q (err=%v)", got, CodeFestivalList, err)
	}
}

func TestResolveFestivalDir_AFailingFestIsReported(t *testing.T) {
	festEnv(t)
	fakeFest(t, ``, 1)

	_, err := ResolveFestivalDir(context.Background(), t.TempDir())
	if got := errpkg.Code(err); got != CodeFestivalList {
		t.Fatalf("code = %q, want %q (err=%v)", got, CodeFestivalList, err)
	}
}

func TestResolveFestivalDir_MissingFestIsANotFound(t *testing.T) {
	festEnv(t)

	_, err := ResolveFestivalDir(context.Background(), t.TempDir())
	if got := errpkg.Code(err); got != "E_LAUNCH_NOT_FOUND" {
		t.Fatalf("code = %q, want E_LAUNCH_NOT_FOUND (err=%v)", got, err)
	}
}

func TestResolveFestivalDir_CancelledContext(t *testing.T) {
	festEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := ResolveFestivalDir(ctx, t.TempDir()); err == nil {
		t.Fatal("a cancelled context must not run fest")
	}
}

func trimTrailingNewlines(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
