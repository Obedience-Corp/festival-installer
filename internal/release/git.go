// Package release resolves a plugin's downloadable artifact for the current
// platform from its git releases, host-agnostically: the latest tag is found
// with `git ls-remote` (works on any git host), and the binary + checksums are
// fetched from declared URL templates. No forge API, so it works for GitHub,
// GitLab, Gitea, self-hosted git, or a plain web server.
package release

import (
	"bufio"
	"context"
	"net/http"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/Obedience-Corp/festival-installer/internal/artifacts"
	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/gitsafe"
	"github.com/Obedience-Corp/festival-installer/internal/installer"
)

const defaultHTTPTimeout = 30 * time.Second

// Source declares where a plugin's releases live. It is carried on a
// marketplace package entry instead of a static per-platform manifest.
// AssetURL and ChecksumsURL are templates over {tag}, {version}, {os}, {arch}.
type Source struct {
	Type         string            `json:"type"`
	Repo         string            `json:"repo"`
	AssetURL     string            `json:"asset_url"`
	ChecksumsURL string            `json:"checksums_url"`
	Binary       string            `json:"binary,omitempty"`
	OS           map[string]string `json:"os,omitempty"`
	Arch         map[string]string `json:"arch,omitempty"`
}

// Resolved is the concrete artifact for one platform.
type Resolved struct {
	Version   string
	URL       string
	Sha256    string
	IsArchive bool
}

type Resolver struct {
	Client *http.Client
}

func NewResolver() *Resolver {
	return &Resolver{Client: &http.Client{Timeout: defaultHTTPTimeout, CheckRedirect: artifacts.CheckRedirect}}
}

var defaultOS = map[string]string{"darwin": "macOS", "linux": "linux"}
var defaultArch = map[string]string{"amd64": "x86_64", "arm64": "arm64"}

func (r *Resolver) Resolve(ctx context.Context, src Source, channel, goos, goarch string) (Resolved, error) {
	if err := ctx.Err(); err != nil {
		return Resolved{}, errpkg.Wrap("E_RELEASE_CTX", err, "context cancelled")
	}
	tag, version, err := r.latestTag(ctx, src.Repo, channel)
	if err != nil {
		return Resolved{}, err
	}
	repl := strings.NewReplacer(
		"{tag}", tag,
		"{version}", version,
		"{os}", token(src.OS, defaultOS, goos),
		"{arch}", token(src.Arch, defaultArch, goarch),
	)
	assetURL := repl.Replace(src.AssetURL)
	checksumsURL := repl.Replace(src.ChecksumsURL)
	assetName := path.Base(assetURL)

	sum, err := r.checksumFor(ctx, checksumsURL, assetName)
	if err != nil {
		return Resolved{}, err
	}
	return Resolved{
		Version:   version,
		URL:       assetURL,
		Sha256:    sum,
		IsArchive: isArchiveName(assetName),
	}, nil
}

func (r *Resolver) latestTag(ctx context.Context, repo, channel string) (string, string, error) {
	if err := gitsafe.ValidateRemote(repo); err != nil {
		return "", "", err
	}
	args := append(gitsafe.ConfigArgs(), "ls-remote", "--tags", "--refs", "--", repo)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = gitsafe.Env()
	out, err := cmd.Output()
	if err != nil {
		return "", "", errpkg.Wrap("E_RELEASE_LS_REMOTE", err, "git ls-remote "+repo)
	}
	var best, bestVer string
	for line := range strings.SplitSeq(string(out), "\n") {
		_, after, ok := strings.Cut(line, "refs/tags/")
		if !ok {
			continue
		}
		tag := strings.TrimSpace(after)
		ver := strings.TrimPrefix(tag, "v")
		if !channelMatches(ver, channel) {
			continue
		}
		if best == "" || installer.VersionLess(bestVer, ver) {
			best, bestVer = tag, ver
		}
	}
	if best == "" {
		return "", "", errpkg.New("E_RELEASE_NO_TAG", "no "+channelOrStable(channel)+" tag in "+repo)
	}
	return best, bestVer, nil
}

func channelMatches(version, channel string) bool {
	pre := ""
	if _, p, ok := strings.Cut(version, "-"); ok {
		pre = p
	}
	switch channelOrStable(channel) {
	case "stable":
		return pre == ""
	default:
		return strings.HasPrefix(pre, channelOrStable(channel))
	}
}

func channelOrStable(channel string) string {
	if channel == "" {
		return "stable"
	}
	return channel
}

func (r *Resolver) checksumFor(ctx context.Context, checksumsURL, assetName string) (string, error) {
	if err := artifacts.RequireHTTPS(checksumsURL); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checksumsURL, nil)
	if err != nil {
		return "", errpkg.Wrap("E_RELEASE_REQ", err, "build request for "+checksumsURL)
	}
	resp, err := r.Client.Do(req)
	if err != nil {
		return "", errpkg.Wrap("E_RELEASE_DO", err, "GET "+checksumsURL)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", errpkg.New("E_RELEASE_HTTP_STATUS", resp.Status+" from "+checksumsURL)
	}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && path.Base(fields[1]) == assetName {
			return fields[0], nil
		}
	}
	return "", errpkg.New("E_RELEASE_NO_CHECKSUM", "no checksum for "+assetName)
}

func token(override, fallback map[string]string, key string) string {
	if override != nil {
		if v, ok := override[key]; ok {
			return v
		}
	}
	if v, ok := fallback[key]; ok {
		return v
	}
	return key
}

func isArchiveName(name string) bool {
	n := strings.ToLower(name)
	return strings.HasSuffix(n, ".tar.gz") || strings.HasSuffix(n, ".tgz")
}
