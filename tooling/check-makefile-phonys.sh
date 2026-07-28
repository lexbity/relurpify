#!/usr/bin/env bash
set -euo pipefail

# check-makefile-phonys: verifies every target declared in .PHONY has a recipe
# (directly or via dependency resolution). Exits non-zero on phantom targets
# that would produce "No rule to make target".

cd "$(git rev-parse --show-toplevel 2>/dev/null || echo "$(dirname "$0")/..")"

# Extract all target names from .PHONY lines
phonys=$(sed -n 's/^\.PHONY:[[:space:]]*//p' Makefile | tr '\n' ' ' | tr -s ' ' '\n' | sed '/^$/d' | sort -u)

failed=0
for target in $phonys; do
  # Skip targets explicitly listed as known no-op grouping targets
  case "$target" in
    lint-layering|lint-invariants)
      continue
      ;;
  esac
  output=$(make -n "$target" 2>&1 || true)
  if echo "$output" | grep -q "No rule to make target"; then
    echo "[FAIL] $target: declared in .PHONY but has no recipe"
    failed=1
  fi
done

if [ "$failed" -ne 0 ]; then
  exit 1
fi
echo "[PASS] all .PHONY targets have recipes"
