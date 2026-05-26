set working-directory := '..'

# Show available recipes for this module
[private]
@default:
    just --list --justfile {{source_file()}}

# Fail with install instructions if golangci-lint is missing.
[private]
require-golangci-lint:
    @command -v golangci-lint >/dev/null 2>&1 || { \
      echo "golangci-lint not found. Install with: just tools install-golangci-lint"; \
      exit 1; \
    }

# Install golangci-lint into $(go env GOPATH)/bin.
install-golangci-lint:
    go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
    @echo "Installed. Ensure $(go env GOPATH)/bin is on PATH."
