package app

import (
	"context"
	"errors"
	"strings"
	"syscall"
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

func TestAppNSISInstallArgs(t *testing.T) {
	if got := strings.Join(nsisInstallArgs(false), " "); got != "/S" {
		t.Errorf("fresh: %q", got)
	}
	if got := strings.Join(nsisInstallArgs(true), " "); got != "/S /UPDATE" {
		t.Errorf("update: %q", got)
	}
}

func TestAppNSISExitMapping(t *testing.T) {
	if nsisExitError(0) != nil {
		t.Fatal("exit 0 must be nil")
	}
	for _, code := range []int{1, 2, 5} {
		err := nsisExitError(code)
		if errpkg.Code(err) != "E_APP_INSTALLER_FAILED" || !strings.Contains(err.Error(), "exited "+string(rune('0'+code))) {
			t.Errorf("exit %d: %v", code, err)
		}
	}
}

func TestAppNSISPolicyBlockStartError(t *testing.T) {
	for _, n := range []syscall.Errno{4551, 1260} {
		err := nsisStartError(n)
		if errpkg.Code(err) != "E_APP_INSTALLER_FAILED" || !strings.Contains(err.Error(), "Smart App Control") {
			t.Errorf("errno %d: %v", n, err)
		}
	}
	err := nsisStartError(syscall.Errno(2))
	if errpkg.Code(err) != "E_APP_INSTALLER_FAILED" || strings.Contains(err.Error(), "Smart App Control") {
		t.Errorf("errno 2: %v", err)
	}
}

func TestAppNSISFormatRejectsLinuxFormats(t *testing.T) {
	p := nsisPlatform{}
	if f, err := p.format(context.Background(), "", nil); f != "nsis" || err != nil {
		t.Fatalf("%q %v", f, err)
	}
	if _, err := p.format(context.Background(), "deb", nil); errpkg.Code(err) != "E_APP_FORMAT_UNSUPPORTED" {
		t.Errorf("--format: %v", err)
	}
	if _, err := p.format(context.Background(), "", &appReceipt{Format: "app-bundle"}); errpkg.Code(err) != "E_APP_FORMAT_UNSUPPORTED" {
		t.Errorf("other receipt format: %v", err)
	}
}

func TestAppNSISDiscoverRejectsForeignMainBinary(t *testing.T) {
	p := nsisPlatform{readReg: func() (nsisRegistration, bool, error) {
		return nsisRegistration{DisplayName: "Festival App", MainBinaryName: "other.exe", InstallLocation: "/abs/Festival App"}, true, nil
	}}
	if _, err := p.discover(context.Background(), "nsis", nil, appRelease{}); errpkg.Code(err) != "E_APP_FOREIGN" {
		t.Fatalf("%v", err)
	}
}

func TestAppNSISDiscoverRejectsRelativeLocation(t *testing.T) {
	for _, loc := range []string{"", "Festival App", `relative\dir`} {
		p := nsisPlatform{readReg: func() (nsisRegistration, bool, error) {
			return nsisRegistration{DisplayName: "Festival App", InstallLocation: loc}, true, nil
		}}
		if _, err := p.discover(context.Background(), "nsis", nil, appRelease{}); errpkg.Code(err) != "E_APP_FOREIGN" {
			t.Errorf("location %q: %v", loc, err)
		}
	}
	absent := nsisPlatform{readReg: func() (nsisRegistration, bool, error) { return nsisRegistration{}, false, nil }}
	if cp, err := absent.discover(context.Background(), "nsis", &appReceipt{Location: "stale"}, appRelease{}); err != nil || cp.Found {
		t.Errorf("absent registration: %+v %v", cp, err)
	}
	boom := errors.New("registry unreadable")
	bad := nsisPlatform{readReg: func() (nsisRegistration, bool, error) { return nsisRegistration{}, false, boom }}
	if _, err := bad.discover(context.Background(), "nsis", nil, appRelease{}); !errors.Is(err, boom) {
		t.Errorf("read error: %v", err)
	}
}
