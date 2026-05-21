set dotenv-load := false

mod release '.justfiles/build.just'
mod testing '.justfiles/test.just'

binary := "obey-installer"
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

# Format code
fmt:
    go fmt ./...

# Run go vet
vet:
    go vet ./...

# Run linter
lint:
    golangci-lint run ./...

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
