package artifacts

import (
	"archive/zip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

func ExtractZip(ctx context.Context, srcArchive, destDir string) error {
	return ExtractZipWithLimits(ctx, srcArchive, destDir, DefaultExtractLimits)
}

func ExtractZipWithLimits(ctx context.Context, srcArchive, destDir string, lim ExtractLimits) (err error) {
	if err := ctx.Err(); err != nil {
		return errpkg.Wrap("E_ARTIFACT_CTX", err, "context cancelled before extract")
	}
	absDest, err := filepath.Abs(destDir)
	if err != nil {
		return errpkg.Wrap("E_ARTIFACT_ABS", err, "resolve dest "+destDir)
	}
	if err := os.MkdirAll(absDest, 0o755); err != nil {
		return errpkg.Wrap("E_ARTIFACT_MKDIR", err, "create extract dir")
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(absDest)
		}
	}()

	r, err := zip.OpenReader(srcArchive)
	if err != nil {
		return errpkg.Wrap("E_ARTIFACT_ZIP", err, "open zip "+srcArchive)
	}
	defer func() { _ = r.Close() }()
	if len(r.File) > lim.MaxEntries {
		return errpkg.Wrap("E_ARTIFACT_ARCHIVE_TOO_LARGE", ErrArchiveTooLarge, "entry count exceeds limit")
	}
	var totalBytes int64
	seen := make(map[string]struct{}, len(r.File))
	for _, member := range r.File {
		if err := ctx.Err(); err != nil {
			return errpkg.Wrap("E_ARTIFACT_CTX", err, "context cancelled during extract")
		}
		// ZIP names use forward slashes. Backslashes and colons can alias a
		// different Windows path or address an alternate data stream.
		name := member.Name
		if name == "" || strings.ContainsAny(name, `\:`) || strings.HasPrefix(name, "/") ||
			strings.HasPrefix(name, "//") {
			return errpkg.Wrap("E_ARTIFACT_UNSAFE_PATH", ErrUnsafePath, name)
		}
		target, err := SafeJoin(absDest, filepath.FromSlash(name))
		if err != nil {
			return err
		}
		key := strings.ToLower(filepath.Clean(name))
		if _, duplicate := seen[key]; duplicate {
			return errpkg.Wrap("E_ARTIFACT_UNSAFE_PATH", ErrUnsafePath, "duplicate zip member "+name)
		}
		seen[key] = struct{}{}
		mode := member.Mode()
		if mode&os.ModeSymlink != 0 || (!mode.IsRegular() && !member.FileInfo().IsDir()) {
			return errpkg.Wrap("E_ARTIFACT_UNSAFE_PATH", ErrUnsafePath, "non-regular zip member "+name)
		}
		if member.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return errpkg.Wrap("E_ARTIFACT_MKDIR", err, "mkdir "+target)
			}
			continue
		}
		if member.UncompressedSize64 > uint64(lim.MaxFileBytes) ||
			member.UncompressedSize64 > uint64(lim.MaxTotalBytes-totalBytes) {
			return errpkg.Wrap("E_ARTIFACT_ARCHIVE_TOO_LARGE", ErrArchiveTooLarge, "decompressed size exceeds limit")
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return errpkg.Wrap("E_ARTIFACT_MKDIR", err, "mkdir parent of "+target)
		}
		in, err := member.Open()
		if err != nil {
			return errpkg.Wrap("E_ARTIFACT_ZIP", err, "open zip member "+name)
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode.Perm())
		if err != nil {
			_ = in.Close()
			return errpkg.Wrap("E_ARTIFACT_CREATE", err, "create "+target)
		}
		remaining := lim.MaxTotalBytes - totalBytes
		if remaining > lim.MaxFileBytes {
			remaining = lim.MaxFileBytes
		}
		written, copyErr := io.Copy(out, io.LimitReader(in, remaining+1))
		closeErr := out.Close()
		_ = in.Close()
		if copyErr != nil {
			return errpkg.Wrap("E_ARTIFACT_WRITE", copyErr, "write "+target)
		}
		if written > remaining {
			return errpkg.Wrap("E_ARTIFACT_ARCHIVE_TOO_LARGE", ErrArchiveTooLarge, "decompressed size exceeds limit")
		}
		if closeErr != nil {
			return errpkg.Wrap("E_ARTIFACT_CLOSE", closeErr, "close "+target)
		}
		totalBytes += written
	}
	return nil
}
