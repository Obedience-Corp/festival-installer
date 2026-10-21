// Package shellsetup manages the installer-owned part of shell startup files.
package shellsetup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const Begin = "# >>> festival shell integration >>>"
const End = "# <<< festival shell integration <<<"

// Merge preserves every byte outside the single owned block. An incomplete or
// duplicate marker is a conflict, not permission to rewrite a user's profile.
// An empty block removes the owned section.
func Merge(original, block string) (string, error) {
	lines := strings.SplitAfter(original, "\n")
	start, end, offset := -1, -1, 0
	for _, line := range lines {
		marker := strings.TrimSpace(line)
		switch marker {
		case Begin:
			if start >= 0 || end >= 0 {
				return "", fmt.Errorf("duplicate or out-of-order Festival markers")
			}
			start = offset
		case End:
			if start < 0 || end >= 0 {
				return "", fmt.Errorf("unmatched Festival end marker")
			}
			end = offset + len(line)
		}
		offset += len(line)
	}
	if start >= 0 && end < 0 {
		return "", fmt.Errorf("unmatched Festival begin marker")
	}
	if start >= 0 {
		return original[:start] + block + original[end:], nil
	}
	if block == "" {
		return original, nil
	}
	if original != "" && !strings.HasSuffix(original, "\n") {
		original += "\n"
	}
	return original + block, nil
}

func Block(snippet string) string {
	return Begin + "\n# Managed by Festival setup.\n" + strings.TrimRight(snippet, "\n") + "\n" + End + "\n"
}

// File is a snapshot used for an optimistic write under the setup lock. RC
// symlinks remain symlinks: replacement targets their resolved regular file.
type File struct {
	Path   string
	Target string
	Data   []byte
	Mode   os.FileMode
	Exists bool
}

func Read(path string) (File, error) {
	f := File{Path: path, Target: path, Mode: 0o600}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return f, nil
	}
	if err != nil {
		return f, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		f.Target, err = filepath.EvalSymlinks(path)
		if err != nil {
			return f, fmt.Errorf("resolve profile symlink: %w", err)
		}
		info, err = os.Stat(f.Target)
		if err != nil {
			return f, err
		}
	}
	if !info.Mode().IsRegular() || info.Size() > 4<<20 {
		return f, fmt.Errorf("profile must be a regular file no larger than 4 MiB: %s", path)
	}
	f.Mode, f.Exists = info.Mode().Perm(), true
	f.Data, err = os.ReadFile(f.Target)
	return f, err
}

// Write makes a content-addressed backup before replacing changed existing
// content. Equal content is never rewritten. Callers serialize setup writers.
func (f File) Write(ctx context.Context, data []byte, backupDir string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if f.Exists && bytes.Equal(f.Data, data) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(f.Target), 0o700); err != nil {
		return false, err
	}
	if f.Exists && backupDir != "" {
		if err := os.MkdirAll(backupDir, 0o700); err != nil {
			return false, err
		}
		name := fmt.Sprintf("%x-%x", sha256.Sum256([]byte(f.Target)), sha256.Sum256(f.Data))
		// Publish the backup atomically, and compare its bytes on every retry.
		// A previous interrupted write must never count as a completed backup.
		backup, err := Read(filepath.Join(backupDir, name))
		if err != nil {
			return false, err
		}
		if _, err := backup.Write(ctx, f.Data, ""); err != nil {
			return false, err
		}

	}
	file, err := os.CreateTemp(filepath.Dir(f.Target), ".festival-setup-*")
	if err != nil {
		return false, err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	if err = file.Chmod(f.Mode); err == nil {
		_, err = file.Write(data)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return false, err
	}
	if closeErr != nil {
		return false, closeErr
	}
	current, err := Read(f.Path)
	if err != nil {
		return false, err
	}
	if current.Target != f.Target || current.Exists != f.Exists || !bytes.Equal(current.Data, f.Data) || current.Mode != f.Mode {
		return false, fmt.Errorf("profile changed during setup; retry: %s", f.Path)
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := os.Rename(file.Name(), f.Target); err != nil {
		return false, err
	}
	return true, nil
}
