#!/usr/bin/env bash
# check-single-boot-root.sh
#
# Enforces the single-composition-root invariant. Phase 12 (failing).
#
# Rules:
#   1. BuildBuiltinCapabilityBundle may only be called by BootstrapAgentRuntime
#      (framework/agentenv/workspace.go).
#   2. The legacy symbols agentenv.Open and BuildWorkspaceEnvironment must not
#      reappear (they were renamed/deleted in Phases 5 and 6).
#   3. NewAuthorizedRunner may only be minted by buildSecuredRuntime
#      (framework/agentenv/secured_runtime.go).
#
# CI fails any PR that reintroduces a second root, a legacy boot symbol, or
# an unauthorized runner mint.
set -euo pipefail

module_root="$(cd "$(dirname "$0")/.." && pwd)"
errors=0

# --- Rule 1: BuildBuiltinCapabilityBundle callers ---------------------------
violations=$(grep -rn 'BuildBuiltinCapabilityBundle' \
    --include='*.go' "$module_root" 2>/dev/null \
    | grep -v 'vendor/' \
    | grep -v 'framework/services/capability_bundle.go' \
    || true)

while IFS= read -r line; do
    [ -z "$line" ] && continue
    # The ONLY allowed production caller is BootstrapAgentRuntime in workspace.go.
    if echo "$line" | grep -q 'framework/agentenv/workspace.go'; then
        continue
    fi
    # Allow doc.go and test files (comments, grep commands, test function names).
    if echo "$line" | grep -qE '(doc\.go|_test\.go|fakerunner\.go)'; then
        continue
    fi
    echo "FAIL: unexpected BuildBuiltinCapabilityBundle caller (new composition root?):"
    echo "  $line"
    errors=$((errors + 1))
done <<< "$violations"

# --- Rule 2: Legacy symbols must not reappear --------------------------------
# Only production code matters — test function names can reference the deleted
# symbols in their names (they won't compile if the actual function exists).
for symbol in 'agentenv\.Open(' 'BuildWorkspaceEnvironment('; do
    matches=$(grep -rn "$symbol" --include='*.go' "$module_root" 2>/dev/null \
        | grep -v 'vendor/' \
        | grep -v '_test.go' \
        | grep -v 'doc.go' \
        || true)
    if [ -n "$matches" ]; then
        echo "FAIL: legacy symbol '$symbol' must not reappear:"
        echo "$matches"
        errors=$((errors + 1))
    fi
done

# --- Rule 3: NewAuthorizedRunner may only be minted by buildSecuredRuntime ----
# Check for NewAuthorizedRunner calls outside the authoritative producer.
ar_violations=$(grep -rn 'NewAuthorizedRunner' --include='*.go' "$module_root" 2>/dev/null \
    | grep -v 'vendor/' \
    | grep -v 'framework/sandbox/authorized_runner.go' \
    | grep -v 'framework/sandbox/authorized_runner_test.go' \
    | grep -v 'framework/agentenv/secured_runtime.go' \
    | grep -v 'testsuite/testsupport/fakerunner.go' \
    || true)

if [ -n "$ar_violations" ]; then
    echo "FAIL: NewAuthorizedRunner called outside buildSecuredRuntime:"
    echo "$ar_violations"
    errors=$((errors + 1))
fi

# --- Summary ----------------------------------------------------------------
if [ "$errors" -gt 0 ]; then
    echo ""
    echo "FAIL: $errors boot-root invariant violation(s) detected"
    exit 1
fi

echo "OK: single composition root invariants hold"
