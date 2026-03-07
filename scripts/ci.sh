#!/usr/bin/env bash
set -euo pipefail

go test ./...

./scripts/browser-ci.sh

if [[ -n "${RELURPIFY_AGENTTEST_SUITE:-}" ]]; then
  go run ./cmd/dev-agent agenttest run --suite "${RELURPIFY_AGENTTEST_SUITE}"
else
  echo "RELURPIFY_AGENTTEST_SUITE not set; skipping agenttest run."
fi
