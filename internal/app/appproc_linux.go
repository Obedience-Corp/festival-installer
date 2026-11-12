//go:build linux

package app

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const maxProcEnviron = 1 << 20

func listProcessesOS(ctx context.Context) ([]processInfo, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	var out []processInfo
	for i, e := range entries {
		if i%64 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		dir := filepath.Join("/proc", e.Name())
		exe, err := os.Readlink(filepath.Join(dir, "exe"))
		if err != nil {
			continue
		}
		out = append(out, processInfo{
			PID:      pid,
			Exe:      strings.TrimSuffix(exe, " (deleted)"),
			AppImage: procAppImage(filepath.Join(dir, "environ")),
		})
	}
	return out, nil
}

func procAppImage(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	raw, err := io.ReadAll(io.LimitReader(f, maxProcEnviron))
	if err != nil {
		return ""
	}
	for _, kv := range bytes.Split(raw, []byte{0}) {
		if v, ok := strings.CutPrefix(string(kv), "APPIMAGE="); ok {
			return v
		}
	}
	return ""
}
