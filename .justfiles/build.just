set working-directory := '..'

binary := "festival"
bin_dir := "bin"
version := `git describe --tags --always --dirty 2>/dev/null || echo "dev"`
ldflags := "-s -w -X main.version=" + version

# Show available recipes for this module
[private]
@default:
    just --list --justfile {{source_file()}}

# Build for Linux amd64
linux:
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags '{{ldflags}}' -o {{bin_dir}}/{{binary}}-linux-amd64 ./cmd/{{binary}}

# Build for Linux arm64
linux-arm64:
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags '{{ldflags}}' -o {{bin_dir}}/{{binary}}-linux-arm64 ./cmd/{{binary}}

# Build for macOS amd64
darwin:
    CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags '{{ldflags}}' -o {{bin_dir}}/{{binary}}-darwin-amd64 ./cmd/{{binary}}

# Build for macOS arm64
darwin-arm64:
    CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags '{{ldflags}}' -o {{bin_dir}}/{{binary}}-darwin-arm64 ./cmd/{{binary}}

# Build all platforms
all: linux linux-arm64 darwin darwin-arm64

# Verify the Linux release artifacts do not require glibc or another dynamic loader.
verify-linux-static:
    #!/usr/bin/env bash
    set -euo pipefail
    for artifact in {{bin_dir}}/{{binary}}-linux-amd64 {{bin_dir}}/{{binary}}-linux-arm64; do
        file "$artifact" | grep -q "statically linked" || {
            echo "release artifact is not statically linked: $artifact" >&2
            exit 1
        }
    done

# --- Release tagging -------------------------------------------------------
#
# Division of labor with CI: .github/workflows/release.yml fires on every `v*`
# tag, rebuilds the four portable binaries, writes checksums.txt, and runs
# `gh release create --generate-notes --verify-tag`. So the recipes below stop
# at the tag push. Do not add local release-note generation or a `gh release
# create` call here; it would race the workflow and publish a duplicate release.
#
# Channels: this repo ships stable tags only. camp and fest also cut `rc` and
# `dev` channel tags, and the installer deliberately does not, because
# release.yml matches every `v*` tag and publishes without `--prerelease`. gh
# does not infer prerelease status from a tag name, so a `v0.3.0-rc.1` tag would
# publish a full release that GitHub can mark as Latest. Adding `just release
# rc` or `just release dev` means teaching release.yml to pass `--prerelease`
# for tags carrying a `-rc.N` or `-dev.N` suffix first.

# Show the newest tag on each channel
current:
    #!/usr/bin/env bash
    set -euo pipefail
    stable=$(git tag -l --sort=-version:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | head -1 || true)
    rc=$(git tag -l "v*-rc.*" --sort=-version:refname | head -1)
    dev=$(git tag -l "v*-dev.*" --sort=-version:refname | head -1)
    echo "stable: ${stable:-none}"
    echo "rc:     ${rc:-none} (not cut by this repo, see .justfiles/build.just)"
    echo "dev:    ${dev:-none} (not cut by this repo, see .justfiles/build.just)"

# List every release tag, newest first
tags:
    @git tag -l "v*" --sort=-version:refname

# Guard (main, clean, level with origin/main), run `just ci`, create the annotated
# tag, push it. release.yml takes over from there. Pass `dry-run` as the second
# argument to print the computed tag and stop before any of that happens.
[doc("Cut a stable release: guard, gate, tag, push. Pass 'dry-run' to preview the tag")]
stable level="patch" dry_run="":
    #!/usr/bin/env bash
    set -euo pipefail
    case "{{dry_run}}" in
        "") ;;
        dry-run|--dry-run)
            next=$(just --justfile {{source_file()}} _next-stable "{{level}}")
            echo "dry run: next {{level}} release would be $next"
            echo "dry run: stopping before guard, gate, tag, and push"
            exit 0
            ;;
        *)
            echo "release stable: unknown argument '{{dry_run}}' (pass 'dry-run' or nothing)" >&2
            exit 1
            ;;
    esac

    just --justfile {{source_file()}} _release-guard

    new=$(just --justfile {{source_file()}} _next-stable "{{level}}")
    if git rev-parse -q --verify "refs/tags/${new}" >/dev/null; then
        echo "release stable: tag ${new} already exists" >&2
        exit 1
    fi

    echo "Running gate before tagging..."
    just ci

    echo "Creating stable release: ${new}"
    git tag -a "${new}" -m "festival installer ${new}"
    git push origin "${new}"
    echo "Tagged ${new}. release.yml builds the artifacts and publishes the GitHub release."
    echo "Watch it here:"
    gh run list --workflow release.yml --limit 1 || true

# Refuse to tag unless main is checked out, clean, and level with origin/main
[private]
_release-guard:
    #!/usr/bin/env bash
    set -euo pipefail
    branch=$(git rev-parse --abbrev-ref HEAD)
    if [ "${branch}" != "main" ]; then
        echo "release: refusing to tag from branch '${branch}'. Releases are cut from main." >&2
        exit 1
    fi
    if [ -n "$(git status --porcelain)" ]; then
        echo "release: refusing to tag with a dirty working tree. Commit or stash first:" >&2
        git status --short >&2
        exit 1
    fi
    git fetch --quiet --tags origin main
    local_head=$(git rev-parse HEAD)
    remote_head=$(git rev-parse FETCH_HEAD)
    if [ "${local_head}" != "${remote_head}" ]; then
        echo "release: refusing to tag, main is not level with origin/main." >&2
        echo "  local:  ${local_head}" >&2
        echo "  origin: ${remote_head}" >&2
        exit 1
    fi

# Print the next stable version derived from the newest vX.Y.Z tag
[private]
_next-stable level:
    #!/usr/bin/env bash
    set -euo pipefail
    latest=$(git tag -l --sort=-version:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | head -1 || true)
    if [ -z "${latest}" ]; then
        echo "v0.1.0"
        exit 0
    fi
    IFS='.' read -r major minor patch <<< "${latest#v}"
    case "{{level}}" in
        major) major=$((major + 1)); minor=0; patch=0 ;;
        minor) minor=$((minor + 1)); patch=0 ;;
        patch) patch=$((patch + 1)) ;;
        *)
            echo "release: unknown level '{{level}}' (use patch, minor, or major)" >&2
            exit 1
            ;;
    esac
    echo "v${major}.${minor}.${patch}"
