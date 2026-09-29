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

# Run the no-TTY integration lane (builds the binary and drives it headless)
notty:
    go test -tags notty -run 'NoTTY' -count=1 -v ./internal/cli/

# Run starter subprocess tests inside an isolated Linux filesystem.
starter:
    docker run --rm -v "{{invocation_directory()}}:/src:ro" golang:1.26-alpine sh -c 'cp -a /src /work && cd /work && go test -tags container_fs -run "Test(ExecuteStarter|StarterResult)" ./internal/app'

# Pass absolute paths to Linux Camp/Fest binaries for end-to-end onboarding.
starter-real camp_binary fest_binary:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p bin
    CGO_ENABLED=0 GOOS=linux GOARCH=$(go env GOARCH) go test -c -tags container_fs -o bin/starter-integration.test ./internal/cli
    docker run --rm -v "$PWD/bin/starter-integration.test:/starter.test:ro" -v "{{camp_binary}}:/tools/camp:ro" -v "{{fest_binary}}:/tools/fest:ro" -e STARTER_CAMP_BINARY=/tools/camp -e STARTER_FEST_BINARY=/tools/fest golang:1.26-alpine sh -c 'apk add --no-cache git bash >/dev/null && su nobody -s /bin/sh -c "/starter.test -test.v -test.run TestStarterInstallAndCurrentUpdate"'
