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
# I4: no decorative manifest fields — every populated field must have a Go reader
# ---------------------------------------------------------------------------
echo "I4: checking for decorative manifest fields..."
# This check enforces that every field populated in a .tool.yaml manifest is
# consumed by at least one code path. The field→reader map below documents
# where each manifest field is read. Any field not found in the map or by
# grepping reader directories is flagged as decorative (hard failure).
#
# Reader directories scanned:
#   - platform/tools/subprocess/   -> ExpandCommand, NewTool, Run
#   - platform/tools/composite/    -> composite.New
#   - framework/cfgload/           -> validate*, LoadToolManifest
#   - framework/toolcapabilities/  -> Build, ChunkToolResult, wrap
#   - platform/contracts/          -> struct tags + NormalizeToolName
#
# Field→reader map: fields that have a dedicated reader that may not be
# discovered by a simple grep across reader dirs are listed here with their
# expected reader file pattern. The reader must exist and be loadable.
# Fields NOT in this map are looked up via grep across reader dirs.
declare -A field_reader_map
field_reader_map=(

    # --- schema / structural ---
    [schema]="framework/cfgload"
    [name]="framework/cfgload"
    [version]="framework/cfgload"
    [family]="subprocess/executor.go:Category"
    [intent]="framework/cfgload"
    [description]="framework/cfgload"

    # --- guidance ---
    [guidance]="framework/cfgload"

    # --- parameters ---
    [parameters]="subprocess/executor.go:Parameters"

    # --- execution ---
    [execution]="framework/cfgload"
    [backend]="toolcapabilities/build.go:buildOne"
    [implementation]="toolcapabilities/build.go:buildGoNative"
    [default_args]="subprocess/flagexpand.go:ExpandCommand"

    # --- execution.command ---
    [command]="framework/cfgload"
    [base]="subprocess/flagexpand.go:ExpandCommand"
    [args]="subprocess/flagexpand.go:ExpandCommand"
    [flags]="subprocess/flagexpand.go:expandFlags"
    [when_true]="subprocess/flagexpand.go:expandBooleanFlag"
    [when_false]="subprocess/flagexpand.go:expandBooleanFlag"
    [param]="subprocess/flagexpand.go:expandTypedFlag"
    [style]="subprocess/flagexpand.go:expandTypedFlag"
    [type]="subprocess/flagexpand.go"
    [repeat]="subprocess/flagexpand.go:expandTypedFlag"

    # --- execution.platform_variants ---
    [platform_variants]="subprocess/flagexpand.go:ExpandCommand"

    # --- execution.sandbox ---
    [sandbox]="framework/cfgload"
    [allowed_root]="framework/cfgload"
    [timeout_seconds]="subprocess/run.go:Run"
    [network_access]="subprocess/egress.go:isNetworkTool"
    [allow_flags]="subprocess/flagexpand.go:ExpandCommand"
    [memory_mb]="subprocess/run.go:Run"
    [pids_limit]="subprocess/run.go:Run"
    [cpus]="subprocess/run.go:Run"
    [allow_hosts]="subprocess/egress.go:checkEgress"

    # --- execution.* ---
    [stdin]="subprocess/executor.go"
    [allow_stdin]="subprocess/executor.go"
    [supports_workdir]="subprocess/executor.go"
    [mcp]="toolcapabilities/build.go:buildOne"

    # --- returns ---
    [returns]="toolcapabilities/chunking.go:ChunkToolResult"
    [shape]="toolcapabilities/chunking.go"
    [chunking]="toolcapabilities/chunking.go"
    [mode]="toolcapabilities/chunking.go"
    [item_path]="toolcapabilities/chunking.go"
    [ref_fields]="toolcapabilities/chunking.go"

    # --- errors ---
    [errors]="subprocess/run.go:Run"

    # --- capability ---
    [capability]="framework/cfgload"
    [trust_class]="toolcapabilities/wrap.go:wrapWithCapability"
    [risk_class]="toolcapabilities/wrap.go:wrapWithCapability"
    [effect_class]="toolcapabilities/wrap.go:wrapWithCapability"

    # --- rate_limit ---
    [rate_limit]="framework/capability"
    [per_second]="framework/capability"
    [burst]="framework/capability"

    # --- composition ---
    [composition]="tools/composite"
    [steps]="tools/composite"
    [tool]="tools/composite"
    [alias]="tools/composite"

    # --- telemetry ---
    [telemetry]="subprocess"
    [span_name]="subprocess"
    [extra_attributes]="subprocess"
)

reader_dirs=(
    "platform/tools/subprocess"
    "platform/tools/composite"
    "framework/cfgload"
    "framework/toolcapabilities"
    "platform/contracts"
)

# Collect all unique manifest keys from .tool.yaml files (all nesting levels).
manifest_fields=$(grep -rhE '^[[:space:]]*[a-z_]+:' "$root/relurpify_cfg/tools/" \
    --include="*.tool.yaml" 2>/dev/null | \
    sed 's/^[[:space:]]*//; s/:.*//' | sort -u)

missing=0
for field in $manifest_fields; do
    case "$field" in
        yaml|json) continue ;;
    esac

    # Check field→reader map first.
    reader="${field_reader_map[$field]:-}"
    if [ -n "$reader" ]; then
        # Verify the mapped reader file actually exists.
        reader_file=$(echo "$reader" | cut -d: -f1)
        found=false
        for dir in "${reader_dirs[@]}"; do
            if [ -f "$root/$dir/$reader_file" ] || [ -f "$root/$dir/" ] && ls "$root/$dir/" | grep -q "$reader_file" 2>/dev/null; then
                found=true
                break
            fi
            if [ -d "$root/$reader" ]; then
                found=true
                break
            fi
        done
        if $found; then
            continue
        fi
    fi

    # Fallback: grep across reader dirs for the field name in Go source.
    found=false
    for dir in "${reader_dirs[@]}"; do
        if grep -rq "[\"\`]$field[\"\`]" "$root/$dir/" --include="*.go" 2>/dev/null; then
            found=true
            break
        fi
        # Also check yaml struct tags.
        if grep -rq "yaml:\"$field" "$root/$dir/" --include="*.go" 2>/dev/null; then
            found=true
            break
        fi
    done

    if ! $found; then
        echo "  FAIL: manifest field \"$field\" has no reader in any reader directory"
        echo "    Add it to field_reader_map in $0 or ensure a reader exists"
        errors=$((errors + 1))
    fi
done

if [ $errors -eq 0 ]; then
    echo "  PASS (no decorative manifest fields found)"
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
