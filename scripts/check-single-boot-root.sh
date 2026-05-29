#!/usr/bin/env bash
# Warn-only: BuildBuiltinCapabilityBundle callers must be consolidated into
# buildSecuredRuntime/BootstrapAgentRuntime — the single composition root.
#
# Currently warn-only (Phase 0 baseline); flipped to failing in Phase 12 when
# BuildWorkspaceEnvironment is deleted and all callers go through a single root.
set -euo pipefail

module_root="$(cd "$(dirname "$0")/.." && pwd)"

# Find all Go source files that call BuildBuiltinCapabilityBundle, excluding
# the definition itself and vendor.
violations=$(grep -rn 'BuildBuiltinCapabilityBundle' \
    --include='*.go' "$module_root" 2>/dev/null \
    | grep -v 'vendor/' \
    | grep -v 'framework/services/capability_bundle.go' \
    || true)

known_callers=(
    "framework/agentenv/workspace.go"   # BootstrapAgentRuntime (kept — will call buildSecuredRuntime)
    "framework/agentenv/composition.go" # BuildWorkspaceEnvironment (TO BE DELETED in Phase 12)
)

unexpected=()
while IFS= read -r line; do
    [ -z "$line" ] && continue
    found=0
    for known in "${known_callers[@]}"; do
        if echo "$line" | grep -q "$known"; then
            found=1
            break
        fi
    done
    if [ "$found" -eq 0 ]; then
        unexpected+=("$line")
    fi
done <<< "$violations"

if [ ${#unexpected[@]} -gt 0 ]; then
    echo "WARN — unexpected BuildBuiltinCapabilityBundle caller(s) detected:"
    for v in "${unexpected[@]}"; do
        echo "  $v"
    done
    echo ""
    echo "Allowed callers:"
    for k in "${known_callers[@]}"; do
        echo "  $k"
    done
fi

echo "OK (warn-only): no unexpected callers of BuildBuiltinCapabilityBundle"
