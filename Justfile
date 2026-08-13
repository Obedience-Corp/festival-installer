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

# Combined pre-commit gate: fmt + vet + lint + test
check: fmt vet lint test

# What CI runs: full check plus cross-platform release build
ci: check
    just release all

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
