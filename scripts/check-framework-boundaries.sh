#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

matches="$(rg -n '"github.com/lexcodex/relurpify/agents(/[^"]*)?"' framework --glob '*.go' || true)"

if [[ -n "${matches}" ]]; then
  echo "framework boundary violation: framework packages must not import agents packages"
  echo
  echo "${matches}"
  exit 1
fi

echo "framework boundary check passed"
