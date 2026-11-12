//go:build darwin

package app

import (
	"context"

	"golang.org/x/sys/unix"
)

func listProcessesOS(ctx context.Context) ([]processInfo, error) {
	kps, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, err
	}
	var out []processInfo
	for i, kp := range kps {
		if i%64 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		pid := int(kp.Proc.P_pid)
		if pid == 0 {
			continue
		}
		buf, err := unix.SysctlRaw("kern.procargs2", pid)
		if err != nil {
			continue
		}
		exe, err := parseProcArgs2(buf)
		if err != nil {
			continue
		}
		out = append(out, processInfo{PID: pid, Exe: exe})
	}
	return out, nil
}
