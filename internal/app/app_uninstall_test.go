package app

import (
	"context"
	"io"
	"os/exec"
	"strings"
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

func TestAppResultUninstallResultMapping(t *testing.T) {
	removed := AppResult{Location: "/h/festival-app", Changed: true}.uninstallResult()
	if len(removed.Removed) != 1 || removed.Removed[0] != "/h/festival-app" || removed.Note != "" || removed.Package != AppPackageID {
		t.Fatalf("%+v", removed)
	}
	gone := AppResult{Location: "/h/festival-app", Notes: []string{"already gone"}}.uninstallResult()
	if len(gone.Removed) != 0 || gone.Note != "already gone" {
		t.Fatalf("%+v", gone)
	}
	left := AppResult{Location: "/h/x", Changed: true, Notes: []string{"Left /h"}}.uninstallResult()
	if len(left.Removed) != 1 || !strings.Contains(left.Note, "removed /h/x") || !strings.Contains(left.Note, "Left /h") {
		t.Fatalf("%+v", left)
	}
}

func TestAppDebRemoveArgvIsFixed(t *testing.T) {
	if got := strings.Join(debRemoveArgv(), " "); got != "/usr/bin/pkexec /usr/bin/apt-get remove --yes festival-app" {
		t.Fatalf("argv = %q", got)
	}
}

func TestAppDebRemoveElevationErrors(t *testing.T) {
	cases := []struct {
		code   int
		start  error
		start2 string
	}{
		{-1, exec.ErrNotFound, "pkexec is not installed."},
		{126, nil, "The password prompt was dismissed."},
		{127, nil, "No password prompt is available in this session."},
	}
	for _, tc := range cases {
		err := debRemoveElevationError(tc.code, tc.start)
		if errpkg.Code(err) != "E_APP_ELEVATION" || !strings.HasPrefix(err.Error(), "E_APP_ELEVATION: "+tc.start2) {
			t.Errorf("%d: %v", tc.code, err)
			continue
		}
		if !strings.Contains(err.Error(), "Or remove it yourself with: sudo apt-get remove --yes festival-app, then run festival uninstall app to clear the record.") ||
			strings.Contains(err.Error(), "--format") {
			t.Errorf("%d: wrong hint: %v", tc.code, err)
		}
	}
	if debRemoveElevationError(0, nil) != nil || debRemoveElevationError(100, nil) != nil {
		t.Error("only elevation failures map here")
	}
}

func TestAppNSISUninstallCmdLine(t *testing.T) {
	got := nsisUninstallCmdLine(`C:\Users\a b\AppData\Local\Festival App\uninstall.exe`, `C:\Users\a b\AppData\Local\Festival App`)
	want := `"C:\Users\a b\AppData\Local\Festival App\uninstall.exe" /S _?=C:\Users\a b\AppData\Local\Festival App`
	if got != want {
		t.Fatalf("got %s\nwant %s", got, want)
	}
}

func TestAppNSISRemoveRefusesForeignUninstallString(t *testing.T) {
	called := false
	p := nsisPlatform{
		readReg: func() (nsisRegistration, bool, error) {
			return nsisRegistration{DisplayName: "Festival App", InstallLocation: `C:\Apps\Festival App`, UninstallString: `C:\Evil\uninstall.exe`}, true, nil
		},
		uninstall: func(context.Context, string, string, io.Writer) (appCommandResult, error) {
			called = true
			return appCommandResult{}, nil
		},
	}
	_, _, err := p.remove(context.Background(), appReceipt{Format: "nsis", Location: `C:\Apps\Festival App`}, nil)
	if errpkg.Code(err) != "E_APP_FOREIGN" || called {
		t.Fatalf("err=%v called=%v", err, called)
	}
	p.readReg = func() (nsisRegistration, bool, error) {
		return nsisRegistration{DisplayName: "Festival App", InstallLocation: `C:\Elsewhere`, UninstallString: `C:\Apps\Festival App\uninstall.exe`}, true, nil
	}
	if _, _, err := p.remove(context.Background(), appReceipt{Format: "nsis", Location: `C:\Apps\Festival App`}, nil); errpkg.Code(err) != "E_APP_FOREIGN" || called {
		t.Fatalf("location mismatch: err=%v called=%v", err, called)
	}
}
