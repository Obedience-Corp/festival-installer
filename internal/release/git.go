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
	"os"
	"os/exec"
	"path"
	"regexp"
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
	Type         string `json:"type"`
	TagPrefix    string `json:"tag_prefix,omitempty"`
	Repo         string `json:"repo"`
	AssetURL     string `json:"asset_url"`
	ChecksumsURL string `json:"checksums_url"`
	Binary       string `json:"binary,omitempty"`
	// Binaries lists every executable a product archive places. Plugins place
	// one binary and use Binary. A product that ships more than one, such as
	// obey shipping obey and ob, declares them all here.
	Binaries []string          `json:"binaries,omitempty"`
	OS       map[string]string `json:"os,omitempty"`
	Arch     map[string]string `json:"arch,omitempty"`
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

var defaultOS = map[string]string{"darwin": "macOS", "linux": "linux", "windows": "windows"}
var defaultArch = map[string]string{"amd64": "x86_64", "arm64": "arm64"}

func (r *Resolver) Resolve(ctx context.Context, src Source, channel, goos, goarch string) (Resolved, error) {
	if err := ctx.Err(); err != nil {
		return Resolved{}, errpkg.Wrap("E_RELEASE_CTX", err, "context cancelled")
	}
	tag, version, err := r.latestTagForSource(ctx, src, channel)
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

// LatestVersion is the highest channel-matching tag in repo, without fetching artifacts.
func (r *Resolver) LatestVersion(ctx context.Context, repo, channel string) (string, error) {
	if r == nil {
		r = NewResolver()
	}
	_, ver, err := r.latestTag(ctx, repo, channel)
	return ver, err
}

// LatestSourceVersion applies the same signed producer namespace as artifact resolution.
func (r *Resolver) LatestSourceVersion(ctx context.Context, src Source, channel string) (string, error) {
	if r == nil {
		r = NewResolver()
	}
	_, version, err := r.latestTagForSource(ctx, src, channel)
	return version, err
}

func (r *Resolver) latestTag(ctx context.Context, repo, channel string) (string, string, error) {
	return r.latestTagForSource(ctx, Source{Repo: repo}, channel)
}

var releaseVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)
var tagNamespace = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func (r *Resolver) latestTagForSource(ctx context.Context, src Source, channel string) (string, string, error) {
	if err := ctx.Err(); err != nil {
		return "", "", errpkg.Wrap("E_RELEASE_CTX", err, "resolve producer tag")
	}
	if src.TagPrefix != "" && !tagNamespace.MatchString(src.TagPrefix) {
		return "", "", errpkg.New("E_RELEASE_TAG_PREFIX", "tag_prefix must be a literal tag namespace")
	}
	if err := gitsafe.ValidateRemote(src.Repo); err != nil {
		return "", "", err
	}
	args := append(gitsafe.ConfigArgs(), "ls-remote", "--tags", "--refs", "--", src.Repo)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = gitsafe.Env()
	cmd.Dir = os.TempDir()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", errpkg.Wrap("E_RELEASE_LS_REMOTE", err, "git ls-remote "+src.Repo+": "+strings.TrimSpace(string(out)))
	}
	return selectProducerTag(string(out), src.TagPrefix, channel)
}

func selectProducerTag(output, prefix, channel string) (string, string, error) {
	var best, bestVersion string
	for line := range strings.SplitSeq(output, "\n") {
		_, after, ok := strings.Cut(line, "refs/tags/")
		if !ok {
			continue
		}
		tag := strings.TrimSpace(after)
		version, matches := strings.CutPrefix(tag, prefix)
		if !matches {
			continue
		}
		version = strings.TrimPrefix(version, "v")
		if !releaseVersion.MatchString(version) || !channelMatches(version, channel) {
			continue
		}
		if best == "" || installer.VersionLess(bestVersion, version) {
			best, bestVersion = tag, version
		}
	}
	if best == "" {
		return "", "", errpkg.New("E_RELEASE_NO_TAG", "no "+channelOrStable(channel)+" release tag for producer namespace "+prefix)
	}
	return best, bestVersion, nil
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
