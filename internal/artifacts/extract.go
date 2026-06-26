package artifacts

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
)

func ExtractTarGz(ctx context.Context, srcArchive, destDir string) error {
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

	f, err := os.Open(srcArchive)
	if err != nil {
		return errpkg.Wrap("E_ARTIFACT_OPEN", err, "open "+srcArchive)
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return errpkg.Wrap("E_ARTIFACT_GZIP", err, "gzip reader for "+srcArchive)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		if err := ctx.Err(); err != nil {
			return errpkg.Wrap("E_ARTIFACT_CTX", err, "context cancelled during extract")
		}
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errpkg.Wrap("E_ARTIFACT_TAR", err, "read tar header")
		}

		target, err := SafeJoin(absDest, hdr.Name)
		if err != nil {
			return err
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return errpkg.Wrap("E_ARTIFACT_MKDIR", err, "mkdir "+target)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return errpkg.Wrap("E_ARTIFACT_MKDIR", err, "mkdir parent of "+target)
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode)&0o777)
			if err != nil {
				return errpkg.Wrap("E_ARTIFACT_CREATE", err, "create "+target)
			}
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return errpkg.Wrap("E_ARTIFACT_WRITE", err, "write "+target)
			}
			if err := out.Close(); err != nil {
				return errpkg.Wrap("E_ARTIFACT_CLOSE", err, "close "+target)
			}
		case tar.TypeSymlink, tar.TypeLink:
			if _, err := SafeJoin(absDest, hdr.Linkname); err != nil {
				return errpkg.Wrap("E_ARTIFACT_UNSAFE_PATH", ErrUnsafePath, "link target escapes dest: "+hdr.Linkname)
			}
			return errpkg.Wrap("E_ARTIFACT_UNSAFE_PATH", ErrUnsafePath, "link members not permitted: "+hdr.Name)
		default:
			continue
		}
	}
	return nil
}

func SafeJoin(absDest, name string) (string, error) {
	clean := filepath.Clean(name)
	if filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) || clean == ".." {
		return "", errpkg.Wrap("E_ARTIFACT_UNSAFE_PATH", ErrUnsafePath, name)
	}
	target := filepath.Join(absDest, clean)
	rel, err := filepath.Rel(absDest, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", errpkg.Wrap("E_ARTIFACT_UNSAFE_PATH", ErrUnsafePath, name)
	}
	return target, nil
}
