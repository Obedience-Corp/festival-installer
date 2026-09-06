#!/usr/bin/env bash
# Isolate a VHS recording from the machine it runs on. Requires just-built
# $PWD/bin/festival. Never records against the operator's real HOME, and never
# puts a path from the operator's disk on screen.
#
# Usage: . docs/demos/vhs-hub-env.sh [fresh|tools]
#
#   fresh  the hub sits in its own managed bin dir and is not on PATH, which is
#          what a machine looks like after install.sh and before the shell rc is
#          wired. A shell function keeps the recorded command line short without
#          putting that dir on PATH, so the hub still grades itself as unset-up.
#   tools  fresh, plus the real camp and fest copied next to each other on PATH,
#          for tapes that launch a child tool.
set -euo pipefail

if [[ ! -x ${PWD}/bin/festival ]]; then
	echo "just build required: missing ${PWD}/bin/festival" >&2
	return 1 2>/dev/null || exit 1
fi

mode=${1:-fresh}
case ${mode} in
fresh | tools) ;;
*)
	echo "vhs: unknown mode: ${mode} (want fresh or tools)" >&2
	return 1 2>/dev/null || exit 1
	;;
esac

camp_src=""
fest_src=""
if [[ ${mode} == tools ]]; then
	camp_src=$(command -v camp) || {
		echo "vhs: tools mode needs camp on PATH" >&2
		return 1 2>/dev/null || exit 1
	}
	fest_src=$(command -v fest) || {
		echo "vhs: tools mode needs fest on PATH" >&2
		return 1 2>/dev/null || exit 1
	}
fi

ROOT=$(mktemp -d /tmp/festival-vhs-XXXXXX)
mkdir -p "${ROOT}/home" "${ROOT}/installer/bin" "${ROOT}/core" "${ROOT}/tools"
cp "${PWD}/bin/festival" "${ROOT}/installer/bin/festival"

# Coreutils only. The hub must not find a camp, fest, or festival on this PATH,
# or it grades its own recording directory as a leftover install.
for t in mkdir clear cat ls rm sleep uname grep; do
	p=$(command -v "$t") || continue
	ln -sf "$p" "${ROOT}/core/$t"
done

hub_path="${ROOT}/core"
if [[ ${mode} == tools ]]; then
	cp "${camp_src}" "${ROOT}/tools/camp"
	cp "${fest_src}" "${ROOT}/tools/fest"
	hub_path="${ROOT}/tools:${ROOT}/core"
fi

export HOME="${ROOT}/home"
export FESTIVAL_HOME="${ROOT}/installer"
export PATH="${hub_path}"
export TERM=xterm-256color
export COLORTERM=truecolor
export COLORFGBG='15;0'
export PS1=$'\033[38;5;208m❯\033[0m '

festival() { "${FESTIVAL_HOME}/bin/festival" "$@"; }

hash -r

# Hand the recorded shell back its ordinary behavior; a strict shell would exit
# the recording on the first command that returns non-zero.
set +e +u
set +o pipefail
