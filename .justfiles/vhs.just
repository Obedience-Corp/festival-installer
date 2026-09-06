# VHS recording for festival manager demos.
#
# just vhs record docs/demos/festival-home.tape
# just vhs all

set working-directory := '..'

[private]
default:
    @just --list --justfile {{source_file()}}

doctor:
    #!/usr/bin/env bash
    set -euo pipefail
    for tool in vhs ttyd ffmpeg; do
        command -v "$tool" >/dev/null 2>&1 || {
            echo "vhs: missing required tool: $tool" >&2
            exit 1
        }
    done
    vhs --version

record tape:
    #!/usr/bin/env bash
    set -euo pipefail
    just build
    vhs "{{tape}}"

# The self-contained tapes. festival-launchpad.tape is excluded on purpose:
# it needs a real camp and fest on PATH, which not every machine has.
all:
    #!/usr/bin/env bash
    set -euo pipefail
    just build
    vhs docs/demos/festival-first-run.tape
    vhs docs/demos/festival-home.tape
    vhs docs/demos/festival-channels.tape
    vhs docs/demos/festival-package.tape
    vhs docs/demos/festival-shadow.tape
