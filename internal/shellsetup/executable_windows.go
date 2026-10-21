//go:build windows

package shellsetup

import "golang.org/x/sys/windows"

const appExecutionLinkTag = 0x8000001b // IO_REPARSE_TAG_APPEXECLINK

// FindFirstFile reports the reparse tag without opening the target. This also
// works when CreateProcess can resolve an alias that os.Open cannot read.
func appExecutionAlias(path string) bool {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	var data windows.Win32finddata
	handle, err := windows.FindFirstFile(name, &data)
	if err != nil {
		return false
	}
	defer func() { _ = windows.FindClose(handle) }()
	return appExecutionAliasData(data)
}

func appExecutionAliasData(data windows.Win32finddata) bool {
	return data.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 &&
		data.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 &&
		data.Reserved0 == appExecutionLinkTag
}
