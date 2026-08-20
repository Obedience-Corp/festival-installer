set dotenv-load := false

mod release '.justfiles/build.just'
mod testing '.justfiles/test.just'
mod tools '.justfiles/tools.just'
mod vhs '.justfiles/vhs.just'

binary := "festival"
bin_dir := "bin"

# Show available recipes
[private]
@default:
    just --list

# Build the binary to ./bin/
build:
    mkdir -p {{bin_dir}}
    go build -o {{bin_dir}}/{{binary}} ./cmd/{{binary}}

# Run all tests
test:
    go test ./...

# Run the manager (e.g. `just run version`, `just run install festival`)
run *ARGS:
    go run ./cmd/{{binary}} {{ARGS}}

# Format code
fmt:
    go fmt ./...

# Run go vet
vet:
    go vet ./...

# Run linter (install with: just tools install-golangci-lint)
lint:
    @just tools require-golangci-lint
    golangci-lint run ./...

# Fail if an em dash (U+2014) appears in tracked or untracked .go/.md files (still honors .gitignore)
no-em-dash:
    #!/usr/bin/env bash
    set -euo pipefail
    hits="$(git grep -n --untracked $'\xe2\x80\x94' -- '*.go' '*.md' || true)"
    if [ -n "$hits" ]; then
        echo "em dash (U+2014) found in .go/.md files:" >&2
        echo "$hits" >&2
        echo "" >&2
        echo "This repo does not allow em dashes in source or docs. Use a colon, a comma, parentheses, or two sentences instead." >&2
        exit 1
    fi
    echo "no em dashes found in .go/.md files"

# Combined pre-commit gate: fmt + vet + lint + no-em-dash + test
check: fmt vet lint no-em-dash test

# What CI runs: full check plus cross-platform release build
ci: check
    just release all
    just release verify-linux-static

# Download and tidy dependencies
tidy:
    go mod tidy
    go mod download

# Install binary to GOPATH/bin
install:
    go install ./cmd/{{binary}}

# Clean build artifacts
clean:
    rm -rf {{bin_dir}}
    rm -f coverage.out coverage.html
