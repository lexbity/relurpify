#!/usr/bin/env bash
# Enforces: platform/* must not import framework/*
# Exclusions:
# - platform/contracts/ (allowed to reference framework in comments)
# - _test.go files (test files may import framework for testing)
# - platform/sandbox/dockersandbox/ (Phase 2: sandbox type consolidation)
set -euo pipefail
violations=$(grep -rn '"codeburg.org/lexbit/relurpify/framework/' \
    platform/ --include="*.go" 2>/dev/null | grep -v "^platform/contracts/" | grep -v "_test.go" | grep -v "platform/sandbox/dockersandbox/" || true)
if [ -n "$violations" ]; then
  echo "FAIL: platform/ → framework/ boundary violations:"
  echo "$violations"
  exit 1
fi
echo "OK: no platform/ → framework/ boundary violations"
