#!/usr/bin/env bash
# Fail a native lane that skipped a selected test or ran none (mirrors windows-install-container-proof.ps1:103-108).
set -euo pipefail
log="${1:?usage: require-native-pass.sh <go test -v log>}"
if grep -qE -- '^(--- FAIL:|FAIL)' "$log"; then echo "app_native lane has a failing test" >&2; exit 1; fi
if grep -q -- '--- SKIP:' "$log"; then echo "app_native lane skipped a test" >&2; exit 1; fi
grep -q -- '--- PASS:' "$log" || { echo "app_native lane ran no tests" >&2; exit 1; }
