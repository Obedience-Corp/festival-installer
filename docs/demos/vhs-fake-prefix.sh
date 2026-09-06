#!/usr/bin/env bash
# Build a helper-adjacent fake prefix for VHS tapes. Requires just-built
# $PWD/bin/festival. Does not use live /usr/bin/festival as the hub.
#
# Usage: . docs/demos/vhs-fake-prefix.sh [package|shadow]
set -euo pipefail

if [[ ! -x ${PWD}/bin/festival ]]; then
	echo "just build required: missing ${PWD}/bin/festival" >&2
	return 1 2>/dev/null || exit 1
fi

mode=${1:-package}
FAKE=$(mktemp -d /tmp/festival-vhs-fake-XXXXXX)
CORE=${FAKE}/core
mkdir -p "${FAKE}/usr/bin" "${FAKE}/usr/share/festival/shell" "${CORE}" "${FAKE}/home"
cp "${PWD}/bin/festival" "${FAKE}/usr/bin/festival"
chmod +x "${FAKE}/usr/bin/festival"

for t in mkdir clear cp chmod cat ls rm sleep uname; do
	p=$(command -v "$t") || continue
	ln -sf "$p" "${CORE}/$t"
done

printf '%s\n' '#!/bin/sh' 'echo camp v0.5.1' 'echo bundle: festival v0.3.1' 'echo profile: stable' >"${FAKE}/usr/bin/camp"
printf '%s\n' '#!/bin/sh' 'echo fest v0.6.3' 'echo bundle: festival v0.3.1' 'echo profile: stable' >"${FAKE}/usr/bin/fest"
chmod +x "${FAKE}/usr/bin/camp" "${FAKE}/usr/bin/fest"
echo '# festival zsh helper' >"${FAKE}/usr/share/festival/shell/festival.zsh"

PATH="${FAKE}/usr/bin:${CORE}"
if [[ $mode == shadow ]]; then
	LEFT=${FAKE}/leftover
	mkdir -p "${LEFT}"
	printf '%s\n' '#!/bin/sh' 'echo camp v0.5.0' >"${LEFT}/camp"
	printf '%s\n' '#!/bin/sh' 'echo fest dev' >"${LEFT}/fest"
	chmod +x "${LEFT}/camp" "${LEFT}/fest"
	PATH="${FAKE}/usr/bin:${LEFT}:${CORE}"
fi

export PATH
export HOME="${FAKE}/home"
export TERM=xterm-256color
export COLORTERM=truecolor
export COLORFGBG='15;0'
export FESTIVAL_REDUCED_MOTION=1
export FESTIVAL_HOME="${FAKE}/missing-home"
hash -r
export PS1=$'\033[38;5;208m❯\033[0m '
