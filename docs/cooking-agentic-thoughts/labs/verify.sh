#!/usr/bin/env bash
# Labs verifier for Cooking Agentic Thoughts with Relurpify.
# Usage: ./verify.sh <chapter>   (for example: ./verify.sh ch01)
#
# Tier map (see README.md): ch01 runs the seeded-failure check. Lint and
# boot tiers arrive with Chapter 4, the behavior tier with Chapter 9.

set -euo pipefail

LABS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHAPTER="${1:?usage: verify.sh <chapter>}"

case "$CHAPTER" in
ch01)
    SCRATCH="$(mktemp -d)"
    trap 'rm -rf "$SCRATCH"' EXIT
    cp "$LABS_DIR/ch01-stringkit/stringkit.py" "$SCRATCH/"
    cp "$LABS_DIR/ch01-stringkit/test_stringkit.py" "$SCRATCH/"

    set +e
    (cd "$SCRATCH" && python3 -m pytest test_stringkit.py -q) >"$SCRATCH/out.txt" 2>&1
    RC=$?
    set -e

    if [ "$RC" -eq 0 ]; then
        echo "FAIL: tests passed — the seeded bug is gone from the lab copy"
        cat "$SCRATCH/out.txt"
        exit 1
    fi
    grep -q "1 failed" "$SCRATCH/out.txt" || {
        echo "FAIL: expected exactly 1 failing test"
        cat "$SCRATCH/out.txt"
        exit 1
    }
    grep -q "test_slugify_strips_punctuation" "$SCRATCH/out.txt" || {
        echo "FAIL: the failing test is not test_slugify_strips_punctuation"
        cat "$SCRATCH/out.txt"
        exit 1
    }
    grep -q "1 passed" "$SCRATCH/out.txt" || {
        echo "FAIL: expected the whitespace test to pass"
        cat "$SCRATCH/out.txt"
        exit 1
    }
    echo "OK: ch01 — seeded failure reproduced (1 failed, 1 passed)"
    ;;
*)
    echo "no verification defined for '$CHAPTER' yet" >&2
    exit 2
    ;;
esac
