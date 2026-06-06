#!/usr/bin/env bash
# glob_check.sh — grep gate for glob pattern usage in non-test Go files
set -o nounset -o pipefail -o errexit

ROOT="${1:-$(pwd)}"
cd "$ROOT"

patterns=("glob\.Glob" "filepath\.Glob" "glob\.Match" "glob\.Compile")
found=0

for pattern in "${patterns[@]}"; do
    matches=$(grep -rn "$pattern" --include='*.go' --exclude='*_test.go' . \
        2>/dev/null | grep -v '\.git/' | grep -v '\.gocache/' | grep -v '\.gomodcache/' \
        | grep -v 'tooling/arch/' || true)
    if [ -n "$matches" ]; then
        echo "[FAIL] glob gate: matches for '$pattern'"
        echo "$matches"
        found=1
    fi
done

if [ "$found" -eq 0 ]; then
    echo "[PASS] glob gate: no glob patterns found"
fi
exit "$found"
