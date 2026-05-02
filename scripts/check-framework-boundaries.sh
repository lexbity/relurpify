#!/usr/bin/env bash
# Enforces: framework/* must not import agents/* or named/*
set -euo pipefail
violations=$(grep -rn \
    '"codeburg.org/lexbit/relurpify/agents/\|"codeburg.org/lexbit/relurpify/named/' \
    framework/ --include="*.go" 2>/dev/null | grep -v "_test.go" || true)
if [ -n "$violations" ]; then
  echo "FAIL: framework/ -> agents/ or named/ boundary violations:"
  echo "$violations"
  exit 1
fi
echo "OK: no framework/ -> agents/ or named/ boundary violations"
