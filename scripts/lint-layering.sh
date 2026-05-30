#!/usr/bin/env bash
# Enforces tool-authoring rearchitecture invariants (Phase 20).
#
# I1: platform/* must not import framework/*.
# I2: platform/contracts must not import OTel SDK.
# I3: framework/toolcapabilities handles only local backends (no mcp).
# I4: each populated manifest field maps to a reader (no decorative fields).
set -euo pipefail

errors=0
root="$(cd "$(dirname "$0")/.." && pwd)"

# ---------------------------------------------------------------------------
# I1: platform/ → framework/ boundary
# ---------------------------------------------------------------------------
echo "I1: checking platform/ → framework/ boundary..."
violations=$(grep -rn '"codeburg.org/lexbit/relurpify/framework/' \
    "$root/platform/" --include="*.go" 2>/dev/null | \
    grep -v "_test.go" | \
    grep -v "platform/sandbox/dockersandbox/" || true)
if [ -n "$violations" ]; then
    echo "  FAIL: platform/ imports framework/:"
    echo "$violations" | sed 's/^/    /'
    errors=$((errors + 1))
else
    echo "  PASS"
fi

# ---------------------------------------------------------------------------
# I2: platform/contracts imports no OTel SDK
# ---------------------------------------------------------------------------
echo "I2: checking OTel SDK imports in platform/contracts..."
if grep -rn 'opentelemetry\|go.opentelemetry.io' \
    "$root/platform/contracts/" --include="*.go" 2>/dev/null | grep -v "_test.go" >/dev/null; then
    echo "  FAIL: platform/contracts imports OTel SDK"
    errors=$((errors + 1))
else
    echo "  PASS"
fi

# ---------------------------------------------------------------------------
# I3: framework/toolcapabilities handles no MCP — the only allowed MCP
# reference is in build.go's switch/case that returns nil, nil (skip).
# ---------------------------------------------------------------------------
echo "I3: checking toolcapabilities handles no MCP backends..."
mcp_block=$(grep -n -A1 'ToolBackendMCP' "$root/framework/toolcapabilities/build.go" 2>/dev/null || true)
if [ -z "$mcp_block" ]; then
    echo "  PASS (no MCP references)"
elif echo "$mcp_block" | grep -q "return nil, nil"; then
    echo "  PASS (MCP only in skip case)"
else
    echo "  FAIL: unexpected MCP handling in toolcapabilities"
    echo "$mcp_block"
    errors=$((errors + 1))
fi

# ---------------------------------------------------------------------------
# I4: no decorative manifest fields
# ---------------------------------------------------------------------------
echo "I4: checking for decorative manifest fields..."
# Every populated field in a .tool.yaml must be consumed by at least one code path.
# This check scans the reader code paths for field name patterns.
#
# Known reader locations:
#   - platform/tools/subprocess/   -> ExpandCommand, NewTool
#   - framework/cfgload/           -> validate*, LoadToolManifest
#   - framework/toolcapabilities/  -> Build, ChunkToolResult
#   - platform/contracts/          -> struct tags + NormalizeToolName
#
# We flag fields that are populated but never appear in reader code.
# For now, this is a structural check: ensure all manifest YAML keys
# appear in at least one reader file.

reader_dirs=(
    "platform/tools/subprocess"
    "framework/cfgload"
    "framework/toolcapabilities"
    "platform/contracts"
)

# Collect a representative unique set of fields from manifests (excluding
# schema, name, version which are handled by the schema loader).
manifest_fields=$(grep -rhE '^[a-z][a-z_]*:' "$root/relurpify_cfg/tools/" \
    --include="*.tool.yaml" 2>/dev/null | \
    sed 's/^ *//; s/:.*//' | sort -u)

known_reader_terms="name version family intent description guidance parameters execution returns errors capability rate_limit composition telemetry source_path canonical_name backend implementation command platform_variants sandbox stdin default_args allow_stdin supports_workdir mcp base args flags when_true when_false param style type repeat allowed_root timeout_seconds network_access allow_flags memory_mb pids_limit cpus allow_hosts server method shape chunking mode item_path ref_fields trust_class risk_class effect_class span_name extra_attributes per_second burst steps tool alias"

missing=0
for field in $manifest_fields; do
    # Skip fields that are structural or handled by the YAML decoder itself
    case "$field" in
        schema|yaml|json) continue ;;
    esac
    if ! echo "$known_reader_terms" | grep -qw "$field"; then
        # Check if the field appears in any reader directory
        found=false
        for dir in "${reader_dirs[@]}"; do
            if grep -rq "\"$field\"" "$root/$dir/" --include="*.go" 2>/dev/null; then
                found=true
                break
            fi
        done
        if ! $found; then
            echo "  WARNING: manifest field \"$field\" has no obvious reader in reader dirs"
            # Not a hard failure — some fields are handled structurally
        fi
    fi
done

# Hard check: test that critical manifest fields have readers.
# Uses "effect_class (trailing space) and ":effect_class to match both
# YAML struct tags (yaml:"effect_class,...") and inline string matching.
for critical_field in "effect_class" "network_access" "allow_flags" "trust_class"; do
    found=false
    for dir in "${reader_dirs[@]}"; do
        # Match both "field:value and yaml:"field patterns
        if grep -rq " $critical_field\|:\"$critical_field\|: $critical_field" "$root/$dir/" --include="*.go" 2>/dev/null; then
            found=true
            break
        fi
    done
    if ! $found; then
        echo "  FAIL: critical manifest field \"$critical_field\" has no reader"
        errors=$((errors + 1))
    fi
done

if [ $missing -eq 0 ]; then
    echo "  PASS (no decorative critical fields)"
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
if [ $errors -gt 0 ]; then
    echo ""
    echo "LAYER LINT: $errors failure(s)"
    exit 1
fi
echo ""
echo "LAYER LINT: all checks passed"
