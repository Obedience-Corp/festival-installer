//go:build windows

package shellsetup

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsAppExecutionAliasAndRegularZeroFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pwsh.exe")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	alias := windows.Win32finddata{FileAttributes: windows.FILE_ATTRIBUTE_REPARSE_POINT, Reserved0: appExecutionLinkTag}
	if !appExecutionAliasData(alias) || validateWindowsExecutable(path, appExecutionAliasData(alias)) != nil {
		t.Fatal("app execution alias was rejected")
	}
	if err := validateWindowsExecutable(path, appExecutionAliasData(windows.Win32finddata{})); err == nil {
		t.Fatal("regular zero-byte executable was accepted")
	}
	alias.Reserved0 = 0 // another reparse tag must not bypass PE validation
	if appExecutionAliasData(alias) {
		t.Fatal("unrelated reparse point was accepted as an app execution alias")
	}
}
