//go:build windows

package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

const windowsUserPathTarget = `HKEY_CURRENT_USER\Environment\Path`

func readWindowsUserPath() (string, uint32, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return "", registry.EXPAND_SZ, nil
	}
	if err != nil {
		return "", 0, errpkg.Wrap("E_SHELL_PATH_READ", err, "open "+windowsUserPathTarget)
	}
	defer key.Close()
	value, kind, err := key.GetStringValue("Path")
	if errors.Is(err, registry.ErrNotExist) {
		return "", registry.EXPAND_SZ, nil
	}
	if err != nil {
		return "", 0, errpkg.Wrap("E_SHELL_PATH_READ", err, "read "+windowsUserPathTarget)
	}
	return value, kind, nil
}

func userPathContains(value, bin string) bool {
	for _, entry := range filepath.SplitList(value) {
		entry = strings.Trim(entry, `"`)
		if entry != "" && strings.EqualFold(filepath.Clean(entry), filepath.Clean(bin)) {
			return true
		}
	}
	return false
}

func planWindowsUserPath(ctx context.Context, shell, bin string) (ShellRCPlan, error) {
	if err := ctx.Err(); err != nil {
		return ShellRCPlan{}, errpkg.Wrap("E_SHELL_RC_CTX", err, "context cancelled before planning user Path")
	}
	if bin == "" {
		return ShellRCPlan{}, errpkg.New("E_SHELL_PATH_BIN", "managed bin directory is empty")
	}
	value, _, err := readWindowsUserPath()
	if err != nil {
		return ShellRCPlan{}, err
	}
	return ShellRCPlan{
		Shell:      shell,
		File:       windowsUserPathTarget,
		Block:      fmt.Sprintf("Add %s to your user Path. New sessions will find camp, fest, and festival without loading a PowerShell profile.\n", bin),
		Present:    userPathContains(value, bin),
		Appendable: true,
		UserPath:   true,
		BinDir:     bin,
	}, nil
}

func applyWindowsUserPath(ctx context.Context, plan ShellRCPlan) error {
	if plan.File != windowsUserPathTarget || plan.BinDir == "" || !plan.Appendable {
		return errpkg.New("E_SHELL_RC_PLAN", "invalid Windows user Path plan")
	}
	if err := ctx.Err(); err != nil {
		return errpkg.Wrap("E_SHELL_RC_CTX", err, "context cancelled before writing user Path")
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return errpkg.Wrap("E_SHELL_PATH_WRITE", err, "open "+windowsUserPathTarget)
	}
	defer key.Close()
	value, kind, err := key.GetStringValue("Path")
	if errors.Is(err, registry.ErrNotExist) {
		value, kind, err = "", registry.EXPAND_SZ, nil
	}
	if err != nil {
		return errpkg.Wrap("E_SHELL_PATH_READ", err, "read "+windowsUserPathTarget)
	}
	if userPathContains(value, plan.BinDir) {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return errpkg.Wrap("E_SHELL_RC_CTX", err, "context cancelled before writing user Path")
	}
	next := plan.BinDir
	if value != "" {
		next += string(os.PathListSeparator) + value
	}
	if kind == registry.SZ {
		err = key.SetStringValue("Path", next)
	} else {
		err = key.SetExpandStringValue("Path", next)
	}
	if err != nil {
		return errpkg.Wrap("E_SHELL_PATH_WRITE", err, "write "+windowsUserPathTarget)
	}
	// Explorer refreshes its environment on WM_SETTINGCHANGE. A detached or
	// Server Core process may have no listener; the registry value is durable.
	notifyWindowsUserPathChange()
	return nil
}

func notifyWindowsUserPathChange() {
	name, err := windows.UTF16PtrFromString("Environment")
	if err != nil {
		return
	}
	proc := windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW")
	var result uintptr
	_, _, _ = proc.Call(
		0xffff, // HWND_BROADCAST
		0x001a, // WM_SETTINGCHANGE
		0,
		uintptr(unsafe.Pointer(name)),
		0x0002, // SMTO_ABORTIFHUNG
		5000,
		uintptr(unsafe.Pointer(&result)),
	)
}
