package artifacts

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
)

const defaultDownloadTimeout = 5 * time.Minute

type Downloader struct {
	Client   *http.Client
	MaxBytes int64
}

func NewDownloader() *Downloader {
	return &Downloader{
		Client:   &http.Client{Timeout: defaultDownloadTimeout, CheckRedirect: CheckRedirect},
		MaxBytes: DefaultExtractLimits.MaxTotalBytes,
	}
}

func (d *Downloader) maxBytes() int64 {
	if d.MaxBytes > 0 {
		return d.MaxBytes
	}
	return DefaultExtractLimits.MaxTotalBytes
}

func (d *Downloader) Download(ctx context.Context, url, destDir string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", errpkg.Wrap("E_ARTIFACT_CTX", err, "context cancelled before download")
	}
	if err := RequireHTTPS(url); err != nil {
		return "", err
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", errpkg.Wrap("E_ARTIFACT_MKDIR", err, "create download dir")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", errpkg.Wrap("E_ARTIFACT_REQ", err, "build request for "+url)
	}
	resp, err := d.Client.Do(req)
	if err != nil {
		return "", errpkg.Wrap("E_ARTIFACT_DO", err, "GET "+url)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", errpkg.Wrap("E_ARTIFACT_HTTP_STATUS", ErrHTTPStatus, resp.Status+" from "+url)
	}

	limit := d.maxBytes()
	if resp.ContentLength > limit {
		return "", errpkg.Wrap("E_ARTIFACT_DOWNLOAD_TOO_LARGE", ErrDownloadTooLarge, "content-length exceeds size limit for "+url)
	}

	name := filepath.Base(url)
	if name == "" || name == "." || name == "/" || strings.ContainsAny(name, `/\`) {
		name = "artifact.download"
	}
	finalPath := filepath.Join(destDir, name)

	tmp, err := os.CreateTemp(destDir, ".download-*")
	if err != nil {
		return "", errpkg.Wrap("E_ARTIFACT_TMP", err, "create temp file")
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}

	written, err := io.Copy(tmp, io.LimitReader(resp.Body, limit+1))
	if err != nil {
		cleanup()
		return "", errpkg.Wrap("E_ARTIFACT_COPY", err, "stream body to disk")
	}
	if written > limit {
		cleanup()
		return "", errpkg.Wrap("E_ARTIFACT_DOWNLOAD_TOO_LARGE", ErrDownloadTooLarge, "download exceeds size limit for "+url)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return "", errpkg.Wrap("E_ARTIFACT_SYNC", err, "fsync download")
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return "", errpkg.Wrap("E_ARTIFACT_TMP_CLOSE", err, "close temp file")
	}
	if err := os.Rename(tmpName, finalPath); err != nil {
		_ = os.Remove(tmpName)
		return "", errpkg.Wrap("E_ARTIFACT_RENAME", err, "rename temp to "+finalPath)
	}
	return finalPath, nil
}
