#!/usr/bin/env bash
# Enforces: framework/* must not import agents/*, named/*, or ayenitd/*
set -euo pipefail

# Check 1: framework/ -> agents/ or named/ violations
violations=$(grep -rn \
    '"codeburg.org/lexbit/relurpify/agents/\|"codeburg.org/lexbit/relurpify/named/' \
    framework/ --include="*.go" 2>/dev/null | grep -v "_test.go" || true)
if [ -n "$violations" ]; then
  echo "FAIL: framework/ -> agents/ or named/ boundary violations:"
  echo "$violations"
  exit 1
fi
echo "OK: no framework/ -> agents/ or named/ boundary violations"

# Check 2: framework/services -> ayenitd violations
violations=$(grep -rn \
    '"codeburg.org/lexbit/relurpify/ayenitd' \
    framework/services/ --include="*.go" 2>/dev/null | grep -v "_test.go" || true)
if [ -n "$violations" ]; then
  echo "FAIL: framework/services -> ayenitd boundary violations:"
  echo "$violations"
  exit 1
fi
echo "OK: no framework/services -> ayenitd boundary violations"

# Check 3: framework/agentenv -> ayenitd violations
violations=$(grep -rn \
    '"codeburg.org/lexbit/relurpify/ayenitd' \
    framework/agentenv/ --include="*.go" 2>/dev/null | grep -v "_test.go" || true)
if [ -n "$violations" ]; then
  echo "FAIL: framework/agentenv -> ayenitd boundary violations:"
  echo "$violations"
  exit 1
fi
echo "OK: no framework/agentenv -> ayenitd boundary violations"

echo "All framework boundary checks passed"

