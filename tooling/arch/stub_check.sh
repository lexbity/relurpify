#!/usr/bin/env bash
# stub_check.sh — grep gate for stub/placeholder markers in non-test Go files
set -o nounset -o pipefail -o errexit

ROOT="${1:-$(pwd)}"
cd "$ROOT"

patterns=("for now" "placeholder" "would search" "TODO" "FIXME" "HACK" "XXX")
found=0

for pattern in "${patterns[@]}"; do
    matches=$(grep -rn "$pattern" --include='*.go' --exclude='*_test.go' . \
        2>/dev/null | grep -v '\.git/' | grep -v '\.gocache/' | grep -v '\.gomodcache/' \
        | grep -v 'tooling/arch/' || true)
    if [ -n "$matches" ]; then
        echo "[FAIL] stub gate: matches for '$pattern'"
        echo "$matches"
        found=1
    fi
done

if [ "$found" -eq 0 ]; then
    echo "[PASS] stub gate: no stub markers found"
fi
exit "$found"
