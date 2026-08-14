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
