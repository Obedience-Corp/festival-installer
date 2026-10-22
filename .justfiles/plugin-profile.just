# Selected-plugin tests and filesystem state remain inside an isolated container.
[private]
default:
    @just --list --justfile {{source_file()}}

# Full tests with required shells and profile/archive negative paths.
container:
    #!/usr/bin/env bash
    set -euo pipefail
    root="$(git rev-parse --show-toplevel)"
    docker run --rm --tmpfs /tmp:rw,exec,size=512m,mode=1777 \
      -v "$root:/src:ro" -v fa0027-installer-gomod:/go/pkg/mod \
      -v fa0027-installer-build:/cache/build -w /work golang:1.26-bookworm bash -euc '
        cp -a /src/. /work/
        rm -rf /work/.git
        apt-get update -qq
        apt-get install -y -qq zsh fish >/dev/null
        useradd --create-home --uid 1000 fixture
        chown -R fixture:fixture /work /go/pkg/mod /cache/build
        su fixture -s /bin/bash -c "cd /work && export GOCACHE=/cache/build && go vet ./... && go vet -tags notty ./internal/cli/... && go test -race ./... && go test -race -tags notty -run NoTTY -count=1 ./internal/cli && go test -race -tags container_fs -run \"Test(SelectedCampApp|PluginOwnership|PluginProfile)\" ./internal/app"
      '
