package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

func TestParseProcArgs2(t *testing.T) {
	good := append([]byte{2, 0, 0, 0}, []byte("/Applications/Festival App.app/Contents/MacOS/festival-app\x00arg\x00")...)
	if got, err := parseProcArgs2(good); err != nil || got != "/Applications/Festival App.app/Contents/MacOS/festival-app" {
		t.Fatalf("got %q %v", got, err)
	}
	for name, buf := range map[string][]byte{
		"too short":  {1, 0},
		"no NUL":     append([]byte{1, 0, 0, 0}, []byte("/bin/x")...),
		"empty path": {1, 0, 0, 0, 0, 'a'},
	} {
		if _, err := parseProcArgs2(buf); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestMatchAppProcesses(t *testing.T) {
	bundle := appRunScope{Dirs: []string{"/Applications/Festival App.app"}}
	deb := appRunScope{Exes: []string{"/usr/bin/festival-app", "/usr/bin/festival-app-installer", "/usr/bin/festival-activity-engine"}}
	img := appRunScope{Exes: []string{"/home/u/.local/bin/festival-app"}}
	win := appRunScope{Dirs: []string{`C:\Users\u\AppData\Local\Festival App`}}
	cases := []struct {
		name  string
		goos  string
		scope appRunScope
		proc  processInfo
		want  bool
	}{
		{"bundle main", "darwin", bundle, processInfo{PID: 5, Exe: "/Applications/Festival App.app/Contents/MacOS/festival-app"}, true},
		{"bundle installer sidecar", "darwin", bundle, processInfo{PID: 5, Exe: "/Applications/Festival App.app/Contents/MacOS/festival-app-installer"}, true},
		{"bundle engine sidecar", "darwin", bundle, processInfo{PID: 5, Exe: "/Applications/Festival App.app/Contents/MacOS/festival-activity-engine"}, true},
		{"bundle case-insensitive on darwin", "darwin", bundle, processInfo{PID: 5, Exe: "/applications/festival app.app/contents/macos/Festival-App"}, true},
		{"other bundle", "darwin", bundle, processInfo{PID: 5, Exe: "/Applications/Other.app/Contents/MacOS/festival-app"}, false},
		{"name-prefix look-alike", "darwin", bundle, processInfo{PID: 5, Exe: "/Applications/Festival App.app.bak/Contents/MacOS/festival-app"}, false},
		{"other tool in bundle", "darwin", bundle, processInfo{PID: 5, Exe: "/Applications/Festival App.app/Contents/MacOS/other-tool"}, false},
		{"self excluded", "darwin", bundle, processInfo{PID: 99, Exe: "/Applications/Festival App.app/Contents/MacOS/festival-app"}, false},
		{"deb exact", "linux", deb, processInfo{PID: 5, Exe: "/usr/bin/festival-app-installer"}, true},
		{"deb festival is not the app", "linux", deb, processInfo{PID: 5, Exe: "/usr/bin/festival"}, false},
		{"linux is case-sensitive", "linux", deb, processInfo{PID: 5, Exe: "/usr/bin/Festival-App"}, false},
		{"appimage by exe", "linux", img, processInfo{PID: 5, Exe: "/home/u/.local/bin/festival-app"}, true},
		{"appimage by APPIMAGE env", "linux", img, processInfo{PID: 5, Exe: "/tmp/.mount_abc/usr/bin/festival-app", AppImage: "/home/u/.local/bin/festival-app"}, true},
		{"other appimage env", "linux", img, processInfo{PID: 5, Exe: "/tmp/.mount_abc/usr/bin/x", AppImage: "/home/u/other"}, false},
		{"windows main", "windows", win, processInfo{PID: 5, Exe: `C:\Users\u\AppData\Local\Festival App\festival-app.exe`}, true},
		{"windows ignores case", "windows", win, processInfo{PID: 5, Exe: `c:\users\U\appdata\local\festival app\FESTIVAL-ACTIVITY-ENGINE.EXE`}, true},
		{"windows look-alike dir", "windows", win, processInfo{PID: 5, Exe: `C:\Users\u\AppData\Local\Festival App2\festival-app.exe`}, false},
		{"empty exe", "linux", deb, processInfo{PID: 5}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			self := 1
			if tc.name == "self excluded" {
				self = tc.proc.PID
			}
			got := matchAppProcesses(tc.goos, tc.scope, []processInfo{tc.proc}, self)
			if (len(got) == 1) != tc.want {
				t.Fatalf("matched %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAppRunScopeFor(t *testing.T) {
	cases := []struct {
		format, location string
		dirs, exes       int
	}{
		{"app-bundle", "/Applications/Festival App.app", 1, 0},
		{"nsis", `C:\Users\u\AppData\Local\Festival App`, 1, 0},
		{"deb", "dpkg:festival-app", 0, 3},
		{"appimage", "/home/u/.local/bin/festival-app", 0, 1},
	}
	for _, tc := range cases {
		s, err := appRunScopeFor(tc.format, tc.location)
		if err != nil || len(s.Dirs) != tc.dirs || len(s.Exes) != tc.exes {
			t.Errorf("%s: %+v %v", tc.format, s, err)
		}
	}
	if s, _ := appRunScopeFor("appimage", "/x/festival-app"); s.Exes[0] != "/x/festival-app" {
		t.Errorf("appimage scope = %+v", s)
	}
	if _, err := appRunScopeFor("rpm", "/x"); errpkg.Code(err) != "E_APP_FORMAT_UNSUPPORTED" {
		t.Errorf("unknown format: %v", err)
	}
}

func TestRefuseRunningApp(t *testing.T) {
	prev := listAppProcesses
	t.Cleanup(func() { listAppProcesses = prev })
	scope := appRunScope{Exes: []string{"/usr/bin/festival-app"}}
	ctx := context.Background()

	listAppProcesses = func(context.Context) ([]processInfo, error) {
		return []processInfo{{PID: 4242, Exe: "/usr/bin/festival-app"}}, nil
	}
	err := refuseRunningApp(ctx, scope)
	if errpkg.Code(err) != "E_APP_RUNNING" || !strings.Contains(err.Error(), "4242") ||
		!strings.Contains(err.Error(), "/usr/bin/festival-app") || !strings.Contains(err.Error(), "Quit Festival App and run the command again") {
		t.Fatalf("running: %v", err)
	}

	listAppProcesses = func(context.Context) ([]processInfo, error) { return nil, errors.New("sysctl boom") }
	err = refuseRunningApp(ctx, scope)
	if errpkg.Code(err) != "E_APP_RUNNING" || !strings.Contains(err.Error(), "could not check") {
		t.Fatalf("scan failure must fail closed: %v", err)
	}

	listAppProcesses = func(context.Context) ([]processInfo, error) {
		return nil, errpkg.New("E_APP_PLATFORM", "unsupported")
	}
	if err := refuseRunningApp(ctx, scope); errpkg.Code(err) != "E_APP_PLATFORM" {
		t.Fatalf("platform error was remapped: %v", err)
	}

	listAppProcesses = func(context.Context) ([]processInfo, error) {
		return []processInfo{{PID: 1, Exe: "/usr/bin/other"}}, nil
	}
	if err := refuseRunningApp(ctx, scope); err != nil {
		t.Fatalf("no match: %v", err)
	}
}
