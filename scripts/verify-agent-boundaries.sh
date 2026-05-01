#!/bin/bash
# Agent Boundary Verification Script
# Verifies that agent packages conform to envelope-first, stream-aware architecture

set -e

echo "=== Agent Boundary Verification ==="
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

FAILED=0

# Check for stale core.Context runtime references
echo "Checking for stale core.Context runtime references..."
if grep -r "core\.NewContext\|core\.Context\b.*runtime\|\\*core\.Context" --include="*.go" agents/ 2>/dev/null | grep -v "context.Context\|FMP\|protocol\|Manifest\|Sealed\|Lineage" | head -20; then
    echo -e "${RED}FAIL: Found stale core.Context runtime references${NC}"
    FAILED=1
else
    echo -e "${GREEN}PASS: No stale core.Context runtime references found${NC}"
fi
echo ""

# Check for envelope usage in agent packages
echo "Checking for contextdata.Envelope usage in agent packages..."
ENVELOPE_USAGE=$(grep -l "contextdata.Envelope\|contextdata.NewEnvelope" --include="*.go" agents/*/ 2>/dev/null | wc -l)
if [ "$ENVELOPE_USAGE" -gt 0 ]; then
    echo -e "${GREEN}PASS: Found $ENVELOPE_USAGE agent packages using contextdata.Envelope${NC}"
else
    echo -e "${YELLOW}WARN: No agent packages found using contextdata.Envelope${NC}"
fi
echo ""

# Check for proper handoff patterns
echo "Checking for proper envelope handoff patterns (Clone/Merge)..."
HANDOFF_PATTERNS=$(grep -r "CloneEnvelope\|env.Clone()\|env.Merge\|HandoffSnapshot" --include="*.go" agents/ 2>/dev/null | wc -l)
if [ "$HANDOFF_PATTERNS" -gt 0 ]; then
    echo -e "${GREEN}PASS: Found $HANDOFF_PATTERNS handoff pattern usages${NC}"
else
    echo -e "${YELLOW}WARN: No envelope handoff patterns found${NC}"
fi
echo ""

# Check for streaming trigger integration
echo "Checking for streaming trigger integration..."
STREAMING_TRIGGERS=$(grep -r "StreamTrigger\|contextstream.Trigger\|NewContextStreamNode" --include="*.go" agents/*/ 2>/dev/null | wc -l)
if [ "$STREAMING_TRIGGERS" -gt 0 ]; then
    echo -e "${GREEN}PASS: Found $STREAMING_TRIGGERS streaming trigger references${NC}"
else
    echo -e "${YELLOW}WARN: No streaming trigger references found${NC}"
fi
echo ""

# Check for checkpoint request patterns (not direct ownership)
echo "Checking for proper checkpoint request patterns..."
CHECKPOINT_PATTERNS=$(grep -r "RequestCheckpoint\|env\.RequestCheckpoint" --include="*.go" agents/ 2>/dev/null | wc -l)
if [ "$CHECKPOINT_PATTERNS" -gt 0 ]; then
    echo -e "${GREEN}PASS: Found $CHECKPOINT_PATTERNS checkpoint request patterns${NC}"
else
    echo -e "${YELLOW}WARN: No checkpoint request patterns found${NC}"
fi
echo ""

# Verify framework persistence usage
echo "Checking for framework persistence usage..."
PERSISTENCE_USAGE=$(grep -r "frameworkpersistence\|SaveCheckpointArtifact\|LoadLatestCheckpointArtifact" --include="*.go" agents/ 2>/dev/null | wc -l)
if [ "$PERSISTENCE_USAGE" -gt 0 ]; then
    echo -e "${GREEN}PASS: Found $PERSISTENCE_USAGE framework persistence usages${NC}"
else
    echo -e "${YELLOW}WARN: No framework persistence usages found${NC}"
fi
echo ""

# Check for ad-hoc map copying (anti-pattern)
echo "Checking for ad-hoc state map copying (anti-pattern)..."
MAP_COPYING=$(grep -r "map\[string\]any{}\|make(map\[string\]any)\|WorkingData\s*=.*map" --include="*.go" agents/ 2>/dev/null | grep -v "test\|_test.go" | head -10)
if [ -n "$MAP_COPYING" ]; then
    echo -e "${YELLOW}WARN: Found potential ad-hoc map copying:${NC}"
    echo "$MAP_COPYING"
else
    echo -e "${GREEN}PASS: No ad-hoc map copying patterns found${NC}"
fi
echo ""

# Summary
echo "=== Verification Summary ==="
if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}All critical boundary checks passed${NC}"
    exit 0
else
    echo -e "${RED}Some boundary checks failed${NC}"
    exit 1
fi
