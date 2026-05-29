#!/usr/bin/env bash
# Enforces that the host-exec command runner path (LocalCommandRunner) has been
# removed and is not reintroduced. LocalCommandRunner was deleted in Phase 5 of
# the sandbox-centrality rework (2026-05-28). All command execution must go
# through a verified sandbox backend (gVisor or Docker).
set -euo pipefail

# Check: The NewLocalCommandRunner / LocalCommandRunner symbol must not appear
# anywhere in Go source files outside the guard test itself and doc.go
# historical notes.
violations=$(grep -rn 'NewLocalCommandRunner\|LocalCommandRunner' \
    --include='*.go' . 2>/dev/null \
    | grep -v 'vendor/' \
    | grep -v 'no_local_runner_guard_test.go' \
    | grep -v 'doc.go' \
    || true)
if [ -n "$violations" ]; then
  echo "FAIL: NewLocalCommandRunner/LocalCommandRunner symbol found — host-exec path must not be reintroduced:"
  echo "$violations"
  exit 1
fi

echo "OK: no host-exec path violations"
