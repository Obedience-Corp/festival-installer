package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/metadata"
)

type appRunScope struct {
	Dirs []string // install directories (bundle, NSIS dir)
	Exes []string // exact executables (deb paths, the AppImage file)
}

type processInfo struct {
	PID      int
	Exe      string
	AppImage string // value of APPIMAGE in the process environment (linux only)
}

var appExecutableNames = []string{"festival-app", "festival-app-installer", "festival-activity-engine"}

// listAppProcesses is the OS process scan; tests replace it.
var listAppProcesses = listProcessesOS

// appRunScopeFor builds the scope from a receipt's app.format and app.location.
func appRunScopeFor(format, location string) (appRunScope, error) {
	switch format {
	case metadata.ArtifactKindAppBundle, metadata.ArtifactKindNSIS:
		return appRunScope{Dirs: []string{location}}, nil
	case metadata.ArtifactKindDeb:
		return appRunScope{Exes: []string{"/usr/bin/festival-app", "/usr/bin/festival-app-installer", "/usr/bin/festival-activity-engine"}}, nil
	case metadata.ArtifactKindAppImage:
		return appRunScope{Exes: []string{location}}, nil
	}
	return appRunScope{}, errpkg.New("E_APP_FORMAT_UNSUPPORTED", "unknown Festival App format "+fmt.Sprintf("%q", format))
}

func foldsCase(goos string) bool { return goos == "darwin" || goos == "windows" }

func pathSep(goos string) string {
	if goos == "windows" {
		return `\`
	}
	return "/"
}

func normalizeForCompare(goos, p string) string {
	if foldsCase(goos) {
		return strings.ToLower(p)
	}
	return p
}

// inDir reports whether exe lies in a real subdirectory of dir, not a name prefix.
func inDir(goos, exe, dir string) bool {
	sep := pathSep(goos)
	d := strings.TrimRight(normalizeForCompare(goos, dir), sep)
	return d != "" && strings.HasPrefix(normalizeForCompare(goos, exe), d+sep)
}

func appExeName(goos, exe string) string {
	name := exe
	if i := strings.LastIndex(exe, pathSep(goos)); i >= 0 {
		name = exe[i+1:]
	}
	name = normalizeForCompare(goos, name)
	if goos == "windows" {
		name = strings.TrimSuffix(name, ".exe")
	}
	return name
}

func isAppExecutableName(goos, exe string) bool {
	name := appExeName(goos, exe)
	for _, n := range appExecutableNames {
		if name == n {
			return true
		}
	}
	return false
}

// matchAppProcesses returns the processes that belong to the App in scope. self
// is never counted.
func matchAppProcesses(goos string, scope appRunScope, procs []processInfo, self int) []processInfo {
	var out []processInfo
	for _, p := range procs {
		if p.PID == self || p.Exe == "" {
			continue
		}
		if processMatches(goos, scope, p) {
			out = append(out, p)
		}
	}
	return out
}

func processMatches(goos string, scope appRunScope, p processInfo) bool {
	exe := normalizeForCompare(goos, p.Exe)
	for _, want := range scope.Exes {
		if exe == normalizeForCompare(goos, want) {
			return true
		}
		if goos == "linux" && p.AppImage != "" && p.AppImage == want {
			return true
		}
	}
	for _, dir := range scope.Dirs {
		if inDir(goos, p.Exe, dir) && isAppExecutableName(goos, p.Exe) {
			return true
		}
	}
	return false
}

// refuseRunningApp refuses while any App executable in scope is running. The
// installer never kills anything. A scan that fails as a whole also refuses:
// it fails closed.
func refuseRunningApp(ctx context.Context, scope appRunScope) error {
	procs, err := listAppProcesses(ctx)
	if err != nil {
		if errpkg.Code(err) == "E_APP_PLATFORM" {
			return err
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return errpkg.Wrap("E_APP_RUNNING", err, "could not check whether Festival App is running ("+err.Error()+"); quit Festival App and run the command again")
	}
	if m := matchAppProcesses(runtime.GOOS, scope, procs, os.Getpid()); len(m) > 0 {
		return errpkg.New("E_APP_RUNNING", fmt.Sprintf("Festival App is running (pid %d, %s). Quit Festival App and run the command again.", m[0].PID, m[0].Exe))
	}
	return nil
}

// parseProcArgs2 reads the executable path out of darwin's kern.procargs2
// buffer: an int32 argc, then the NUL-terminated exec path.
func parseProcArgs2(buf []byte) (string, error) {
	if len(buf) < 4 {
		return "", errors.New("procargs2 buffer is too short")
	}
	i := bytes.IndexByte(buf[4:], 0)
	if i <= 0 {
		return "", errors.New("procargs2 has no executable path")
	}
	return string(buf[4 : 4+i]), nil
}
