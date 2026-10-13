package app

import "testing"

func TestProductExecutableNames(t *testing.T) {
	for _, tc := range []struct {
		name, goos, want string
	}{
		{"obey", "windows", "obey.exe"},
		{"ob", "windows", "ob.exe"},
		{"obey.exe", "windows", "obey.exe"},
		{"obey", "darwin", "obey"},
		{"obey", "linux", "obey"},
	} {
		if got := executableNameForOS(tc.name, tc.goos); got != tc.want {
			t.Errorf("executableNameForOS(%q, %q) = %q, want %q", tc.name, tc.goos, got, tc.want)
		}
	}
}
