#!/usr/bin/env bash
# check-config-tree-drift.sh
#
# Asserts that the live config tree (relurpify_cfg/) and the scaffold
# templates (templates/workspace/) are in sync modulo a documented,
# tested allowlist of intentional differences.
#
# Phase 11 of the Unified Boot Contract.
set -euo pipefail

module_root="$(cd "$(dirname "$0")/.." && pwd)"

# --- Allowlist of intentional differences ------------------------------------
#
# Each entry is a relative path from the config root. Files listed here
# are allowed to exist in only one tree.
#
# Format: <tree>:<path>  where tree is "live" or "template".
declare -a allowlist=(
)
# ---------------------------------------------------------------------------

errors=0

check_tree_pair() {
    local label="$1"       # human-readable name
    local live_dir="$2"    # relurpify_cfg subdirectory
    local tpl_dir="$3"     # templates/workspace subdirectory

    live_files=$(mktemp)
    tpl_files=$(mktemp)
    trap 'rm -f "$live_files" "$tpl_files"' RETURN

    find "$live_dir" -type f -printf '%P\n' 2>/dev/null | sort > "$live_files" || true
    find "$tpl_dir"  -type f -printf '%P\n' 2>/dev/null | sort > "$tpl_files"  || true

    # Files only in live (excluding allowlist).
    while IFS= read -r f; do
        [ -z "$f" ] && continue
        skip=0
        for a in "${allowlist[@]}"; do
            [ "$a" = "live:$f" ] && skip=1 && break
        done
        [ "$skip" -eq 1 ] && continue
        # Check if identical file exists in templates.
        if [ ! -f "$tpl_dir/$f" ]; then
            echo "DRIFT: $f exists in live $label but missing from templates"
            errors=$((errors + 1))
        fi
    done < "$live_files"

    # Files only in templates (excluding allowlist).
    while IFS= read -r f; do
        [ -z "$f" ] && continue
        skip=0
        for a in "${allowlist[@]}"; do
            [ "$a" = "template:$f" ] && skip=1 && break
        done
        [ "$skip" -eq 1 ] && continue
        if [ ! -f "$live_dir/$f" ]; then
            echo "DRIFT: $f exists in templates but missing from live $label"
            errors=$((errors + 1))
        fi
    done < "$tpl_files"
}

check_tree_pair "tools"   "$module_root/relurpify_cfg/tools"   "$module_root/templates/workspace/tools"
check_tree_pair "security" "$module_root/relurpify_cfg/security" "$module_root/templates/workspace/security"

if [ "$errors" -gt 0 ]; then
    echo ""
    echo "FAIL: $errors drift(s) detected between relurpify_cfg and templates/workspace"
    echo "Reconcile the differences or add intentional ones to the allowlist in $0"
    exit 1
fi

echo "OK: relurpify_cfg and templates/workspace are in sync"
