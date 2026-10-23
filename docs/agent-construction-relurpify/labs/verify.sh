#!/usr/bin/env bash
# Labs verifier for Agent Construction with Relurpify.
# Usage: ./verify.sh <tier> <chapter>   (for example: ./verify.sh gates ch01)
#
# Tier map (see README.md):
#   build    — compile + unit truth (Chapter 1+)
#   gates    — the framework's law (Chapter 1+; gates accumulate per chapter)
#   behavior — end-to-end behavior (arrives with Chapter 9)
#
# The verifier runs against the checkout that contains this labs folder.

set -euo pipefail

LABS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$LABS_DIR/../../.." && pwd)"
TIER="${1:?usage: verify.sh <build|gates|behavior> <chapter>}"
CHAPTER="${2:?usage: verify.sh <build|gates|behavior> <chapter>}"

# Apply every overlay up to and including CHAPTER into a scratch clone of
# the checkout, so the reader's tree is never touched. Clone via rsync to
# keep uncommitted lab state; exclude caches and runtime state.
apply_overlay() {
    local dst="$1" ch="$2"
    local overlay_src="$LABS_DIR/testdata/overlay"
    local dirs
    dirs=$(ls "$overlay_src" | sort | awk -v upto="$ch" '{split($0,a,"-"); split(upto,b,"-"); if (a[1] <= b[1]) print}')
    for d in $dirs; do
        (cd "$overlay_src/$d" && find . -type f | while read -r f; do
            mkdir -p "$dst/$(dirname "$f")"
            cp "$overlay_src/$d/$f" "$dst/$f"
        done)
    done
}

make_scratch() {
    local scratch
    scratch=$(mktemp -d /tmp/relurpify-verify.XXXXXX)
    rsync -a --exclude='.gomodcache' --exclude='.gocache' \
        --exclude='.relurpify_state' --exclude='.book-scratch' \
        "$REPO_ROOT/" "$scratch/"
    apply_overlay "$scratch" "$CHAPTER"
    echo "$scratch"
}

cd "$REPO_ROOT"

case "$TIER" in
build)
    echo "== tier 1 (build): $CHAPTER =="
    SCRATCH="$(make_scratch)"
    trap 'rm -rf "$SCRATCH"' EXIT
    (cd "$SCRATCH" && go build ./... && go vet ./... \
        && { [ -d cognitionzoo/critic ] && go test ./cognitionzoo/critic/... -count=1 || true; })
    echo "[PASS] build: $CHAPTER compiles clean and its tests pass"
    ;;
gates)
    echo "== tier 2 (gates): $CHAPTER =="
    SCRATCH="$(make_scratch)"
    trap 'rm -rf "$SCRATCH"' EXIT
    (cd "$SCRATCH" && go build ./... && go vet ./... \
        && { [ -d cognitionzoo/critic ] && go test ./cognitionzoo/critic/... -count=1 || true; } \
        && make lint-arch)
    case "$CHAPTER" in
    ch09*|ch10*|ch11*|ch12*|ch13*)
        (cd "$SCRATCH" && make test-euclo-golden)
        ;;
    esac
    case "$CHAPTER" in
    ch11*|ch12*|ch13*)
        (cd "$SCRATCH" && make test-conformance && make test-coverage)
        ;;
    esac
    echo "[PASS] gates: $CHAPTER holds the framework's law"
    ;;
behavior)
    case "$CHAPTER" in
    ch09*|ch10*|ch11*|ch12*|ch13*) ;;
    *) echo "behavior tier starts at Chapter 9" >&2; exit 2 ;;
    esac
    echo "== tier 3 (behavior): $CHAPTER =="
    echo "not yet defined — arrives with Chapter 9" >&2
    exit 2
    ;;
*)
    echo "unknown tier '$TIER' (build|gates|behavior)" >&2
    exit 2
    ;;
esac
