#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

matches="$(rg -n '"github.com/lexcodex/relurpify/platform/lang/(go|js|python|rust)"' named/euclo/agent.go || true)"

if [[ -n "${matches}" ]]; then
  echo "agent import violation: named/euclo/agent.go must not import platform/lang packages"
  echo
  echo "${matches}"
  exit 1
fi

echo "agent lang import check passed"
