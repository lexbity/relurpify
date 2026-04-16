#!/usr/bin/env bash
set -euo pipefail

go test ./...

./scripts/check-agent-lang-imports.sh
./scripts/check-framework-boundaries.sh
./scripts/check-deprecated-agent-wrappers.sh

./scripts/browser-ci.sh

if [[ -n "${RELURPIFY_AGENTTEST_SUITE:-}" ]]; then
  go run ./app/dev-agent-cli agenttest run --suite "${RELURPIFY_AGENTTEST_SUITE}"
else
  echo "RELURPIFY_AGENTTEST_SUITE not set; skipping agenttest run."
fi
