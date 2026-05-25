set working-directory := '..'

# Show available recipes for this module
[private]
@default:
    just --list --justfile {{source_file()}}

# Run tests with race detector
race:
    go test -race ./...

# Run tests with coverage
coverage:
    go test -cover ./...

# Run tests with verbose output
verbose:
    go test -v ./...
