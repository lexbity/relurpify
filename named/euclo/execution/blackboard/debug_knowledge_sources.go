// Package blackboard provides knowledge sources for debug workflows.
//
// These knowledge sources implement hypothesis-driven debugging using the
// blackboard architecture. They share workspace context via the blackboard
// to avoid redundant file exploration (the HTN context isolation bug).
//
// Each KS is programming-language neutral where possible, using abstract
// concepts like "test runner" and "file system" rather than Go-specific tools.
package blackboard

import (
	"context"
	"fmt"
	"strings"
	"time"

	agentblackboard "codeburg.org/lexbit/relurpify/agents/blackboard"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
)

// FileExplorerKS explores the workspace and caches file listings in the blackboard.
// It only runs once per workspace (or when file cache is stale) to avoid
// redundant file_list calls across debug steps.
type FileExplorerKS struct{}

func (ks FileExplorerKS) Name() string { return "file_explorer" }

func (ks FileExplorerKS) Priority() int { return 100 } // High priority - runs early

func (ks FileExplorerKS) CanActivate(bb *agentblackboard.Blackboard) bool {
	// Only activate if we haven't explored yet, or if exploration is stale
	return !hasFact(bb, "workspace.explored")
}

func (ks FileExplorerKS) Execute(ctx context.Context, bb *agentblackboard.Blackboard, registry *capability.Registry, model core.LanguageModel, _ core.AgentSemanticContext) error {
	// Get file_list tool from registry (language-neutral file system access)
	tool, ok := registry.Get("file_list")
	if !ok {
		bb.AddFact("workspace.explored", "error", ks.Name())
		bb.AddFact("workspace.error", "file_list tool not available", ks.Name())
		return nil
	}

	// Call file_list to get workspace contents
	result, err := tool.Execute(ctx, nil, map[string]interface{}{})
	if err != nil || result == nil || !result.Success {
		bb.AddFact("workspace.explored", "error", ks.Name())
		bb.AddFact("workspace.error", fmt.Sprintf("file_list failed: %v", err), ks.Name())
		return nil
	}

	// Extract and cache file list in blackboard for other KS to use
	files := []string{}
	if fileList, ok := result.Data["files"].([]string); ok {
		files = fileList
	}

	// Store as JSON array for easy parsing
	filesJSON := "[]"
	if len(files) > 0 {
		// Build JSON array manually to avoid encoding complexity
		parts := make([]string, len(files))
		for i, f := range files {
			parts[i] = fmt.Sprintf("%q", f)
		}
		filesJSON = "[" + strings.Join(parts, ",") + "]"
	}

	bb.AddFact("workspace.explored", "true", ks.Name())
	bb.AddFact("workspace.files", filesJSON, ks.Name())
	bb.AddFact("workspace.file_count", fmt.Sprintf("%d", len(files)), ks.Name())

	return nil
}

func (ks FileExplorerKS) KnowledgeSourceSpec() agentblackboard.KnowledgeSourceSpec {
	return agentblackboard.KnowledgeSourceSpec{
		Name:     ks.Name(),
		Priority: ks.Priority(),
	}
}

// FaultLocalizerKS analyzes files to localize the root cause of a bug.
// It reads relevant files and adds hypotheses to the blackboard.
type FaultLocalizerKS struct{}

func (ks FaultLocalizerKS) Name() string { return "fault_localizer" }

func (ks FaultLocalizerKS) Priority() int { return 80 }

func (ks FaultLocalizerKS) CanActivate(bb *agentblackboard.Blackboard) bool {
	// Activate when we have workspace exploration but no hypothesis yet
	return hasFact(bb, "workspace.explored") && !hasFact(bb, "debug.hypothesis")
}

func (ks FaultLocalizerKS) Execute(ctx context.Context, bb *agentblackboard.Blackboard, registry *capability.Registry, model core.LanguageModel, _ core.AgentSemanticContext) error {
	// Get list of files from workspace (cached by FileExplorerKS)
	filesJSON := getFactValue(bb, "workspace.files")
	if filesJSON == "" || filesJSON == "[]" {
		bb.AddFact("debug.error", "no workspace files available for analysis", ks.Name())
		return nil
	}

	// For production, would use semantic analysis or LLM to pick the right file
	// from the workspace. Without language-specific heuristics, we cannot
	// determine which file to analyze.
	// TODO: Implement language-agnostic file selection using:
	// - Entry point detection (via project structure analysis)
	// - Recently modified files (via VCS integration)
	// - Error location from stack traces (if available)
	_ = filesJSON // Would be parsed to extract file list in production

	bb.AddFact("debug.error", "cannot determine target file without language-specific analysis", ks.Name())
	return nil
}

func (ks FaultLocalizerKS) KnowledgeSourceSpec() agentblackboard.KnowledgeSourceSpec {
	return agentblackboard.KnowledgeSourceSpec{
		Name:     ks.Name(),
		Priority: ks.Priority(),
	}
}

// PatchApplierKS applies a fix based on the current hypothesis.
// It writes file changes and records what was modified.
type PatchApplierKS struct{}

func (ks PatchApplierKS) Name() string { return "patch_applier" }

func (ks PatchApplierKS) Priority() int { return 60 }

func (ks PatchApplierKS) CanActivate(bb *agentblackboard.Blackboard) bool {
	// Activate when we have a hypothesis but no patch applied yet
	return hasFact(bb, "debug.hypothesis") && !hasFact(bb, "debug.patched")
}

func (ks PatchApplierKS) Execute(ctx context.Context, bb *agentblackboard.Blackboard, registry *capability.Registry, model core.LanguageModel, _ core.AgentSemanticContext) error {
	// Get suspect file from blackboard (set by FaultLocalizerKS)
	suspectFile := getFactValue(bb, "debug.suspect_file")
	if suspectFile == "" {
		bb.AddFact("debug.patched", "error", ks.Name())
		bb.AddFact("debug.error", "no suspect file identified for patching", ks.Name())
		return nil
	}

	// Get file_write tool from registry
	tool, ok := registry.Get("file_write")
	if !ok {
		bb.AddFact("debug.patched", "error", ks.Name())
		bb.AddFact("debug.error", "file_write tool not available", ks.Name())
		return nil
	}

	// In production, would generate fix based on hypothesis using LLM
	// For now, we cannot generate a proper fix without language-specific knowledge
	_ = tool
	_ = suspectFile

	bb.AddFact("debug.patched", "error", ks.Name())
	bb.AddFact("debug.error", "cannot generate fix without language-specific patch generation", ks.Name())
	return nil
}

func (ks PatchApplierKS) KnowledgeSourceSpec() agentblackboard.KnowledgeSourceSpec {
	return agentblackboard.KnowledgeSourceSpec{
		Name:     ks.Name(),
		Priority: ks.Priority(),
	}
}

// VerifierKS runs tests to verify a patch works.
// Language-neutral - uses abstract "test runner" capability.
type VerifierKS struct{}

func (ks VerifierKS) Name() string { return "verifier" }

func (ks VerifierKS) Priority() int { return 40 }

func (ks VerifierKS) CanActivate(bb *agentblackboard.Blackboard) bool {
	// Activate when patch applied but not yet verified
	return hasFact(bb, "debug.patched") && !hasFact(bb, "debug.verified")
}

func (ks VerifierKS) Execute(ctx context.Context, bb *agentblackboard.Blackboard, registry *capability.Registry, model core.LanguageModel, _ core.AgentSemanticContext) error {
	// Language-neutral approach: look for test runner capability in registry
	// Try go_test first, then generic test_runner
	testRunnerNames := []string{"go_test", "test_runner", "cargo_test", "pytest"}
	var testRunner core.Tool
	found := false

	for _, name := range testRunnerNames {
		if tool, ok := registry.Get(name); ok {
			testRunner = tool
			found = true
			break
		}
	}

	if !found {
		// No test runner available - mark as needing manual verification
		bb.AddFact("debug.verified", "pending", ks.Name())
		bb.AddFact("debug.test_results", "no_runner", ks.Name())
		return nil
	}

	// Run tests to verify the fix
	result, err := testRunner.Execute(ctx, nil, map[string]interface{}{})
	if err != nil || result == nil {
		bb.AddFact("debug.verified", "error", ks.Name())
		bb.AddFact("debug.test_results", fmt.Sprintf("execution error: %v", err), ks.Name())
		return nil
	}

	// Record test results
	if result.Success {
		bb.AddFact("debug.verified", "true", ks.Name())
		bb.AddFact("debug.test_results", "pass", ks.Name())
	} else {
		bb.AddFact("debug.verified", "failed", ks.Name())
		bb.AddFact("debug.test_results", fmt.Sprintf("fail: %v", result.Error), ks.Name())
	}

	return nil
}

func (ks VerifierKS) KnowledgeSourceSpec() agentblackboard.KnowledgeSourceSpec {
	return agentblackboard.KnowledgeSourceSpec{
		Name:     ks.Name(),
		Priority: ks.Priority(),
	}
}

// DebugKnowledgeSources returns the complete set of knowledge sources
// for hypothesis-driven debugging using the blackboard architecture.
//
// The activation order is:
// 1. FileExplorerKS (priority 100) - explore workspace once
// 2. FaultLocalizerKS (priority 80) - analyze and form hypothesis
// 3. PatchApplierKS (priority 60) - apply fix based on hypothesis
// 4. VerifierKS (priority 40) - verify the fix works
//
// This ordering ensures context is shared: FileExplorerKS runs first,
// and subsequent KS benefit from its cached file list.
func DebugKnowledgeSources() []agentblackboard.KnowledgeSource {
	return []agentblackboard.KnowledgeSource{
		FileExplorerKS{},
		FaultLocalizerKS{},
		PatchApplierKS{},
		VerifierKS{},
	}
}

// DebugTerminationPredicate checks if the debug workflow is complete.
// Returns true when the bug is verified as fixed or a terminal state is reached.
func DebugTerminationPredicate(bb *agentblackboard.Blackboard) bool {
	if bb == nil {
		return false
	}

	// Check if verification passed
	verified := getFactValue(bb, "debug.verified")
	if verified == "true" || verified == "pending" {
		return true
	}

	// Check if we've reached an error state
	return hasFact(bb, "debug.error")
}

// getFactValue retrieves a fact value from the blackboard as a string.
// Returns empty string if fact not found.
func getFactValue(bb *agentblackboard.Blackboard, key string) string {
	if bb == nil {
		return ""
	}
	for _, fact := range bb.Facts {
		if fact.Key == key {
			return fact.Value
		}
	}
	return ""
}

// setFact adds or updates a fact in the blackboard.
func setFact(bb *agentblackboard.Blackboard, key, value, source string) {
	if bb == nil {
		return
	}
	bb.AddFact(key, value, source)
}

// hasFact checks if a fact exists in the blackboard.
func hasFact(bb *agentblackboard.Blackboard, key string) bool {
	if bb == nil {
		return false
	}
	for _, fact := range bb.Facts {
		if fact.Key == key {
			return true
		}
	}
	return false
}

// DebugExplorerKS seeds debug-oriented initial facts from the full Euclo
// ExecutorSemanticContext. It replaces the generic ExplorerKS for debug
// mode blackboard sessions by running at higher priority and writing the
// exploration.status fact first.
type DebugExplorerKS struct {
	SemCtx euclotypes.ExecutorSemanticContext
}

func (k *DebugExplorerKS) Name() string  { return "DebugExplorer" }
func (k *DebugExplorerKS) Priority() int { return 110 }

func (k *DebugExplorerKS) CanActivate(bb *agentblackboard.Blackboard) bool {
	return !bb.HasFact("exploration.status")
}

func (k *DebugExplorerKS) Execute(
	ctx context.Context,
	bb *agentblackboard.Blackboard,
	_ *capability.Registry,
	_ core.LanguageModel,
	_ core.AgentSemanticContext,
) error {
	// 1. Seed generic context (AST symbols + BKC chunks) from embedded
	//    AgentSemanticContext via the shared seeding function.
	agentblackboard.SeedBlackboardFromSemanticContext(bb, k.SemCtx.AgentSemanticContext)

	// 2. Tensions as hypotheses — candidate root causes for debug.
	for _, tension := range k.SemCtx.Tensions {
		confidence := tensionSeverityToConfidence(tension.Severity)
		bb.AddHypothesisWithOrigin(
			fmt.Sprintf("tension.%s", tension.ID),
			tension.Summary,
			confidence,
			k.Name(),
			&agentblackboard.FactOrigin{
				SourceSystem: "archaeo_tensions",
				RecordID:     tension.ID,
				Kind:         tension.Kind,
				CapturedAt:   time.Now(),
			},
		)
	}

	// 3. High-severity tensions surface immediately as issues.
	for _, tension := range k.SemCtx.Tensions {
		if tension.Severity == "high" || tension.Severity == "blocking" {
			_ = bb.AddIssue(
				fmt.Sprintf("tension_issue.%s", tension.ID),
				fmt.Sprintf("[tension] %s", tension.Summary),
				tensionSeverityToIssueSeverity(tension.Severity),
				k.Name(),
			)
		}
	}

	// 4. Patterns as constraint facts.
	for _, pattern := range k.SemCtx.Patterns {
		bb.AddFactWithOrigin(
			fmt.Sprintf("pattern.constraint.%s", sanitizeKey(pattern.Title)),
			pattern.Summary,
			k.Name(),
			&agentblackboard.FactOrigin{
				SourceSystem: "archaeo_patterns",
				RecordID:     pattern.ID,
				CapturedAt:   time.Now(),
			},
		)
	}

	// 5. Active plan as context fact.
	if k.SemCtx.ActivePlanSummary != "" {
		bb.AddFact("plan.active_summary", k.SemCtx.ActivePlanSummary, k.Name())
	}

	bb.AddFact("exploration.status", "explored", k.Name())
	return nil
}

func tensionSeverityToConfidence(severity string) float64 {
	switch severity {
	case "blocking":
		return 0.95
	case "high":
		return 0.80
	case "medium":
		return 0.60
	default:
		return 0.40
	}
}

func tensionSeverityToIssueSeverity(severity string) string {
	switch severity {
	case "blocking":
		return "high"
	case "high":
		return "high"
	case "medium":
		return "medium"
	default:
		return "low"
	}
}

// ArchaeologyExplorerKS seeds archaeology-oriented initial facts.
// It uses learning interactions and patterns as primary inputs rather
// than tensions, which are more relevant to debug workflows.
type ArchaeologyExplorerKS struct {
	SemCtx euclotypes.ExecutorSemanticContext
}

func (k *ArchaeologyExplorerKS) Name() string  { return "ArchaeologyExplorer" }
func (k *ArchaeologyExplorerKS) Priority() int { return 110 }

func (k *ArchaeologyExplorerKS) CanActivate(bb *agentblackboard.Blackboard) bool {
	return !bb.HasFact("exploration.status")
}

func (k *ArchaeologyExplorerKS) Execute(
	ctx context.Context,
	bb *agentblackboard.Blackboard,
	_ *capability.Registry,
	_ core.LanguageModel,
	_ core.AgentSemanticContext,
) error {
	// 1. Generic AST + BKC seeding.
	agentblackboard.SeedBlackboardFromSemanticContext(bb, k.SemCtx.AgentSemanticContext)

	// 2. Learning interactions as confirmed facts (highest confidence
	//    source in the graph — human-stated or human-confirmed).
	for _, li := range k.SemCtx.LearningInteractions {
		bb.AddFactWithOrigin(
			fmt.Sprintf("learning.confirmed.%s", sanitizeKey(li.Title)),
			li.Summary,
			k.Name(),
			&agentblackboard.FactOrigin{
				SourceSystem: "archaeo_learning",
				RecordID:     li.ID,
				CapturedAt:   time.Now(),
			},
		)
	}

	// 3. Patterns as high-confidence hypotheses about codebase structure.
	for _, pattern := range k.SemCtx.Patterns {
		bb.AddHypothesisWithOrigin(
			fmt.Sprintf("pattern.%s", sanitizeKey(pattern.Title)),
			pattern.Summary,
			0.8,
			k.Name(),
			&agentblackboard.FactOrigin{
				SourceSystem: "archaeo_patterns",
				RecordID:     pattern.ID,
				CapturedAt:   time.Now(),
			},
		)
	}

	// 4. Active plan as context.
	if k.SemCtx.ActivePlanSummary != "" {
		bb.AddFact("plan.active_summary", k.SemCtx.ActivePlanSummary, k.Name())
	}

	bb.AddFact("exploration.status", "explored", k.Name())
	return nil
}

// sanitizeKey replaces non-alphanumeric characters with underscores
// to produce valid fact key strings.
func sanitizeKey(key string) string {
	var result strings.Builder
	for _, r := range key {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			result.WriteRune(r)
		} else {
			result.WriteRune('_')
		}
	}
	return result.String()
}
