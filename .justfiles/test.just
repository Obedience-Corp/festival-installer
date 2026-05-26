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

# Generate HTML coverage report at ./coverage.html
coverage-html:
    go test -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out -o coverage.html
    @echo "Open coverage.html in a browser to view."

# Run tests with verbose output
verbose:
    go test -v ./...

# Run benchmarks (single iteration; use -count=N for more)
bench:
    go test -bench=. -benchtime=1x -run=^$ ./...
