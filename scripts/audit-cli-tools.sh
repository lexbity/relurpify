#!/usr/bin/env bash
# audit-cli-tools.sh — Phase 0: inventory cli_*.go vs manifests
#
# Scans all cli_*.go constructors and tool manifests, then reports:
#   1. Manifest-less Go tools (Go impl, no manifest)
#   2. Go-less manifests (manifest, no Go impl)
#   3. Config drift (descriptions, tags, commands)
#
# Usage: ./scripts/audit-cli-tools.sh [--json] [--diff]
#   --json   output as JSON
#   --diff   show detailed per-field drift

set -euo pipefail
cd "$(git rev-parse --show-toplevel 2>/dev/null || echo "${0%/*}/..")"

MODE="${1:--}"
MANIFEST_DIR="relurpify_cfg/tools"

echo "=== Tool Authoring Audit — Phase 0 ==="
echo ""

# ---------------------------------------------------------------------------
# 1. Extract Go tool names from cli_*.go constructors
# ---------------------------------------------------------------------------
echo "--- Go cli_* tools ---"
declare -A GO_TOOLS  # tool_name -> "path|command|description|tags|allowflags|defaultargs|hitlrequired"
declare -A GO_FAMILIES
total_go=0

for f in platform/shell/*/cli_*.go platform/shell/*/*/cli_*.go; do
  [ -f "$f" ] || continue
  # Extract the package (family)
  pkg=$(awk '/^package /{print $2}' "$f")
  # Find New*Tool function — extract Name/Command/Description/Tags/AllowFlags/DefaultArgs/HITLRequired
  # Use awk to parse the CommandToolConfig literal
  while IFS='|' read -r name cmd desc tags allowflags defaultargs hitl; do
    [ -z "$name" ] && continue
    GO_TOOLS["$name"]="$f|$cmd|$desc|$tags|$allowflags|$defaultargs|$hitl"
    GO_FAMILIES["$name"]="$pkg"
    echo "  $name  (cmd=$cmd, pkg=$pkg)"
    total_go=$((total_go + 1))
  done < <(awk '
    /Name:\s+"[^"]+"/ { name = $0; gsub(/.*Name:\s+"/, "", name); gsub(/".*/, "", name) }
    /Command:\s+"[^"]+"/ { cmd = $0; gsub(/.*Command:\s+"/, "", cmd); gsub(/".*/, "", cmd) }
    /Description:\s+"[^"]+"/ { desc = $0; gsub(/.*Description:\s+"/, "", desc); gsub(/".*/, "", desc) }
    /Tags:\s+\S/ {
      tags = $0
      gsub(/.*Tags:\s+\[/, "", tags)
      gsub(/\].*/, "", tags)
      gsub(/" "/, ",", tags)
      gsub(/"/, "", tags)
    }
    /AllowFlags:\s+true/ { af = "true" }
    /AllowFlags:\s+false/ { af = "false" }
    /AllowFlags:/ && /true|false/ { if (!af) af = "true" }
    /DefaultArgs:\s+\[/ {
      da = $0
      gsub(/.*DefaultArgs:\s+\[/, "", da)
      gsub(/\].*/, "", da)
      gsub(/" "/, ",", da)
      gsub(/"/, "", da)
    }
    /HITLRequired:\s+true/ { hitl = "true" }
    /HITLRequired:\s+false/ { hitl = "false" }
    /^	}\)$/ {  # end of New*Tool call — print it
      if (name != "" && cmd != "") {
        af2 = (af == "true") ? "true" : "false"
        hitl2 = (hitl == "true") ? "true" : "false"
        da2 = (da != "") ? da : "-"
        print name "|" cmd "|" desc "|" tags "|" af2 "|" da2 "|" hitl2
      }
      name = ""; cmd = ""; desc = ""; tags = ""; af = ""; da = ""; hitl = ""
    }
  ' "$f")
done

echo ""
echo "  Total Go cli tools: $total_go"

# ---------------------------------------------------------------------------
# 2. Extract tool names from manifests
# ---------------------------------------------------------------------------
echo ""
echo "--- Manifest tools ---"
declare -A MANIFEST_TOOLS  # tool_name -> "path|description|risk_class|effect_class|backend"
declare -A MANIFEST_FAMILIES
total_manifest=0

while IFS='|' read -r name desc risk effect backend path fam; do
  [ -z "$name" ] && continue
  MANIFEST_TOOLS["$name"]="$path|$desc|$risk|$effect|$backend"
  MANIFEST_FAMILIES["$name"]="$fam"
  echo "  $name  (backend=$backend, fam=$fam)"
  total_manifest=$((total_manifest + 1))
done < <(find "$MANIFEST_DIR" -name '*.tool.yaml' | while read -r m; do
  awk '
    /^name:\s+\S/ { name = $2 }
    /^description:\s+/ {
      desc = $0; gsub(/^description:\s+/, "", desc); gsub(/^"|"$/, "", desc)
    }
    /^\s+risk_class:/ { inrisk=1; risk=""; next }
    inrisk && /^\s+- / { risk = risk (risk==""?"":",") $2 }
    /^\s+effect_class:/ { inrisk=0; ineff=1; effect=""; next }
    ineff && /^\s+- / { effect = effect (effect==""?"":",") $2 }
    /^\s+backend:\s+\S+/ { backend = $2 }
    /^\s+family:\s+\S+/ { fam = $2 }
    /^capability:/ { capsection=1 }
    /^execution:/ { capsection=0 }
    /^$/ && name != "" {
      print name "|" desc "|" risk "|" effect "|" backend "|" FILENAME "|" fam
      name = ""; desc = ""; risk = ""; effect = ""; backend = ""; fam = ""; inrisk=0; ineff=0
    }
    END {
      if (name != "") print name "|" desc "|" risk "|" effect "|" backend "|" FILENAME "|" fam
    }
  ' "$m"
done)

echo ""
echo "  Total manifest tools: $total_manifest"

# ---------------------------------------------------------------------------
# 3. Set diff
# ---------------------------------------------------------------------------
echo ""
echo "=== SET DIFF ==="

echo ""
echo "--- Go-only (no manifest) ---"
go_only=0
for name in "${!GO_TOOLS[@]}"; do
  if [ -z "${MANIFEST_TOOLS[$name]:-}" ]; then
    echo "  $name  (${GO_TOOLS[$name]%%|*})"
    go_only=$((go_only + 1))
  fi
done
[ "$go_only" -eq 0 ] && echo "  (none)"
echo "  Count: $go_only"

echo ""
echo "--- Manifest-only (no Go impl) ---"
manifest_only=0
for name in "${!MANIFEST_TOOLS[@]}"; do
  if [ -z "${GO_TOOLS[$name]:-}" ]; then
    IFS='|' read -r mpath mdesc mrisk meffect mback <<< "${MANIFEST_TOOLS[$name]}"
    echo "  $name  (backend=$mback, $mpath)"
    manifest_only=$((manifest_only + 1))
  fi
done
[ "$manifest_only" -eq 0 ] && echo "  (none)"
echo "  Count: $manifest_only"

# ---------------------------------------------------------------------------
# 4. Config drift (only for tools present in both)
# ---------------------------------------------------------------------------
if [ "${MODE}" = "--diff" ] || [ "${MODE}" = "-d" ]; then
  echo ""
  echo "=== CONFIG DRIFT (tools in both Go + manifest) ==="
  drift_count=0
  for name in "${!GO_TOOLS[@]}"; do
    [ -z "${MANIFEST_TOOLS[$name]:-}" ] && continue
    IFS='|' read -r gopath gocmd godesc gotags goaf goda gohitl <<< "${GO_TOOLS[$name]}"
    IFS='|' read -r mpath mdesc mrisk meffect mback <<< "${MANIFEST_TOOLS[$name]}"
    drift=""
    # Description drift
    if [ "$godesc" != "$mdesc" ]; then
      drift="${drift}desc(GO=\"$godesc\" vs MANI=\"$mdesc\") "
    fi
    # Tag/risk drift
    gotags_clean="$(echo "$gotags" | tr ',' ' ' | xargs)"
    mrisk_clean="$(echo "$mrisk" | tr ',' ' ' | xargs)"
    if [ "$gotags_clean" != "$mrisk_clean" ] && [ "${gotags_clean#read-only}" != "$gotags_clean" ]; then
      # Go has "read-only" which manifest drops — this is expected drift, skip
      :
    elif [ "$gotags_clean" != "$mrisk_clean" ]; then
      drift="${drift}risk(GO=\"$gotags_clean\" vs MANI=\"$mrisk_clean\") "
    fi
    if [ -n "$drift" ]; then
      echo "  $name: $drift"
      drift_count=$((drift_count + 1))
    fi
  done
  [ "$drift_count" -eq 0 ] && echo "  (no drift detected)"
  echo "  Drift count: $drift_count"
fi

# ---------------------------------------------------------------------------
# 5. Triple-authoring surface area
# ---------------------------------------------------------------------------
echo ""
echo "=== TRIPLE AUTHORING ==="
echo "  cli_*.go files:  $total_go"
echo "  manifests:       $total_manifest"
echo "  Go-only tools:   $go_only"
echo "  Manifest-only:   $manifest_only"
echo "  Shared:          $((${#GO_TOOLS[@]} - go_only))"
echo "  Duplicated field declarations (est): $((total_go * 6))"

# ---------------------------------------------------------------------------
# 6. Hidden behaviors summary
# ---------------------------------------------------------------------------
echo ""
echo "=== HIDDEN BEHAVIORS (not in manifest) ==="
echo "  1. Flag injection guard (execute/executor.go:validateUserArgs)"
echo "     - Applies to all CommandTool, gated on AllowFlags (Go config only)"
echo "  2. SSRF egress screen (command/egress.go)"
echo "     - Triggered by Go Tag 'network', not manifest capability"
echo "  3. Cargo isolation (command/cli_command.go + execute/executor_cargo.go)"
echo "     - Triggered by command==cargo + subcommand in {test,build,check,clippy,metadata}"
echo "  4. Panic recovery (command/cli_command.go:defer/recover)"
echo "     - Not present in dormant subprocessTool"
echo "  5. Trust/effect derivation (framework/core/capability_types.go)"
echo "     - Sourced from Tool interface (Tags, Permissions), not manifest"
echo "  6. HITLRequired field (3 tools: gdb, perf, strace)"
echo "     - No manifest field for HITL requirement"

echo ""
echo "=== DONE ==="
