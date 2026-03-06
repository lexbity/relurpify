#!/usr/bin/env bash
set -euo pipefail

if [[ "${RELURPIFY_BROWSER_CI:-1}" == "0" ]]; then
  echo "RELURPIFY_BROWSER_CI=0; skipping browser CI."
  exit 0
fi

echo "[browser-ci] running browser package tests"
go test ./framework/browser/... ./app/relurpish/runtime

if [[ "${RELURPIFY_BROWSER_STRESS:-}" != "" ]]; then
  echo "[browser-ci] running browser stress tests"
  RELURPIFY_BROWSER_STRESS="${RELURPIFY_BROWSER_STRESS}" go test ./framework/browser/... -run 'RepeatedLocalhostFlow'
else
  echo "[browser-ci] RELURPIFY_BROWSER_STRESS not set; skipping browser stress tests."
fi
