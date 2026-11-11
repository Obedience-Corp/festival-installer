//go:build windows

package app

import (
	"context"
	"unsafe"

	"golang.org/x/sys/windows"
)

func listProcessesOS(ctx context.Context) ([]processInfo, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = windows.CloseHandle(snap) }()
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	if err := windows.Process32First(snap, &entry); err != nil {
		return nil, err
	}
	var out []processInfo
	for n := 0; ; n++ {
		if n%64 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if exe, ok := processImagePath(entry.ProcessID); ok {
			out = append(out, processInfo{PID: int(entry.ProcessID), Exe: exe})
		}
		if err := windows.Process32Next(snap, &entry); err != nil {
			break
		}
	}
	return out, nil
}

func processImagePath(pid uint32) (string, bool) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", false
	}
	defer func() { _ = windows.CloseHandle(h) }()
	buf := make([]uint16, 32768)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return "", false
	}
	return windows.UTF16ToString(buf[:size]), true
}
