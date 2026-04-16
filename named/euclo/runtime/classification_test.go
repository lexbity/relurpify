package runtime_test

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	testutil "github.com/lexcodex/relurpify/testutil/euclotestutil"
)

// ─────────────────────────────────────────────────────────────────────────────
// CollectSignals
// ─────────────────────────────────────────────────────────────────────────────

func TestCollectSignals_KeywordDebug(t *testing.T) {
	env := eucloruntime.TaskEnvelope{
		Instruction:   "The tests are failing after the last commit",
		EditPermitted: true,
	}
	signals := eucloruntime.CollectSignals(env)
	assertSignalMode(t, signals, "debug", "keyword")
}

func TestCollectSignals_KeywordReview(t *testing.T) {
	env := eucloruntime.TaskEnvelope{
		Instruction:   "please do a code review of this PR",
		EditPermitted: true,
	}
	signals := eucloruntime.CollectSignals(env)
	assertSignalMode(t, signals, "review", "keyword")
}

func TestCollectSignals_KeywordCompare(t *testing.T) {
	env := eucloruntime.TaskEnvelope{
		Instruction:   "Compare the behavior of MemoryStore and an in-memory null store",
		EditPermitted: false,
	}
	signals := eucloruntime.CollectSignals(env)
	assertSignalMode(t, signals, "review", "keyword")
}

func TestCollectSignals_KeywordCode(t *testing.T) {
	env := eucloruntime.TaskEnvelope{
		Instruction:   "implement the new cache layer",
		EditPermitted: true,
	}
	signals := eucloruntime.CollectSignals(env)
	assertSignalMode(t, signals, "code", "keyword")
}

func TestCollectSignals_ErrorTextPatterns_DebugSignals(t *testing.T) {
	cases := []struct {
		instruction string
		label       string
	}{
		{"goroutine 1 [running]:\nmain.main()", "goroutine dump"},
		{"nil pointer dereference at runtime", "nil pointer"},
		{"panic: runtime error: index out of range", "panic"},
		{"error: unexpected EOF while reading", "error prefix"},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			signals := eucloruntime.CollectSignals(eucloruntime.TaskEnvelope{
				Instruction:   tc.instruction,
				EditPermitted: true,
			})
			assertSignalMode(t, signals, "debug", "error_text")
		})
	}
}

func TestCollectSignals_TaskStructureDebug(t *testing.T) {
	cases := []string{
		"this used to work but broke after the refactor",
		"the build used to pass on CI but now fails",
		"regression: auth stopped working",
		"why is the server returning 500",
	}
	for _, instr := range cases {
		t.Run(instr[:20], func(t *testing.T) {
			signals := eucloruntime.CollectSignals(eucloruntime.TaskEnvelope{
				Instruction:   instr,
				EditPermitted: true,
			})
			assertSignalMode(t, signals, "debug", "task_structure")
		})
	}
}

func TestCollectSignals_TaskStructurePlanning(t *testing.T) {
	cases := []string{
		"how should we migrate the database",
		"design a strategy for the new API",
		"we need to rewrite the auth module",
		"plan a redesign of the storage layer",
	}
	for _, instr := range cases {
		t.Run(instr[:20], func(t *testing.T) {
			signals := eucloruntime.CollectSignals(eucloruntime.TaskEnvelope{
				Instruction:   instr,
				EditPermitted: true,
			})
			assertSignalMode(t, signals, "planning", "task_structure")
		})
	}
}

func TestCollectSignals_FilePatternTDD(t *testing.T) {
	env := eucloruntime.TaskEnvelope{
		Instruction:   "update the handler_test.go file to cover edge cases",
		EditPermitted: true,
	}
	signals := eucloruntime.CollectSignals(env)
	assertSignalMode(t, signals, "tdd", "file_pattern")
}

func TestCollectSignals_FilePatternPlanning(t *testing.T) {
	env := eucloruntime.TaskEnvelope{
		Instruction:   "update the Dockerfile for the new base image",
		EditPermitted: true,
	}
	signals := eucloruntime.CollectSignals(env)
	assertSignalMode(t, signals, "planning", "file_pattern")
}

func TestCollectSignals_ContextHintExplicit(t *testing.T) {
	env := eucloruntime.TaskEnvelope{
		ModeHint:      "debug",
		Instruction:   "do something",
		EditPermitted: true,
	}
	signals := eucloruntime.CollectSignals(env)
	for _, s := range signals {
		if s.Kind == "context_hint" && s.Mode == "debug" && s.Value == "mode_hint:debug" {
			return
		}
	}
	t.Fatalf("expected context_hint:debug signal, got %+v", signals)
}

func TestCollectSignals_ResumedMode(t *testing.T) {
	env := eucloruntime.TaskEnvelope{
		ResumedMode:   "review",
		Instruction:   "continue",
		EditPermitted: true,
	}
	signals := eucloruntime.CollectSignals(env)
	for _, s := range signals {
		if s.Kind == "context_hint" && s.Mode == "review" && s.Value == "resumed_mode:review" {
			return
		}
	}
	t.Fatalf("expected resumed_mode signal, got %+v", signals)
}

func TestCollectSignals_ReadOnlyWorkspace_AddsReviewSignal(t *testing.T) {
	env := eucloruntime.TaskEnvelope{
		Instruction:   "look at this",
		EditPermitted: false, // no write tools → workspace_state:read_only
	}
	signals := eucloruntime.CollectSignals(env)
	for _, s := range signals {
		if s.Kind == "workspace_state" && s.Value == "read_only" && s.Mode == "review" {
			return
		}
	}
	t.Fatalf("expected workspace_state:read_only signal, got %+v", signals)
}

func TestCollectSignals_PreviousArtifacts_PlanArtifact(t *testing.T) {
	env := eucloruntime.TaskEnvelope{
		Instruction:           "continue with the plan",
		EditPermitted:         true,
		PreviousArtifactKinds: []string{"plan_output"},
	}
	signals := eucloruntime.CollectSignals(env)
	for _, s := range signals {
		if s.Kind == "workspace_state" && s.Value == "has_plan_artifact" {
			return
		}
	}
	t.Fatalf("expected has_plan_artifact signal, got %+v", signals)
}

// ─────────────────────────────────────────────────────────────────────────────
// ScoreSignals / IsAmbiguous / NormalizeConfidence
// ─────────────────────────────────────────────────────────────────────────────

func TestScoreSignals_AggregatesAndRanks(t *testing.T) {
	signals := []eucloruntime.ClassificationSignal{
		{Kind: "keyword", Value: "implement", Weight: 0.3, Mode: "code"},
		{Kind: "keyword", Value: "fix", Weight: 0.3, Mode: "code"},
		{Kind: "keyword", Value: "review", Weight: 0.45, Mode: "review"},
	}
	candidates := eucloruntime.ScoreSignals(signals)
	if len(candidates) < 2 {
		t.Fatalf("expected ≥2 candidates, got %d", len(candidates))
	}
	// code (0.6) > review (0.45)
	if candidates[0].Mode != "code" {
		t.Fatalf("expected code to win, got %q (score %.2f)", candidates[0].Mode, candidates[0].Score)
	}
}

func TestScoreSignals_EmptySignals_ReturnsEmpty(t *testing.T) {
	candidates := eucloruntime.ScoreSignals(nil)
	if len(candidates) != 0 {
		t.Fatalf("expected empty candidates, got %d", len(candidates))
	}
}

func TestIsAmbiguous_CloseScores(t *testing.T) {
	candidates := []eucloruntime.ModeCandidate{
		{Mode: "code", Score: 0.6},
		{Mode: "review", Score: 0.58}, // gap = (0.6-0.58)/0.6 ≈ 0.033 < 0.15
	}
	if !eucloruntime.IsAmbiguous(candidates) {
		t.Fatal("expected ambiguous when scores are close")
	}
}

func TestIsAmbiguous_ClearWinner(t *testing.T) {
	candidates := []eucloruntime.ModeCandidate{
		{Mode: "debug", Score: 1.2},
		{Mode: "code", Score: 0.3}, // gap = 0.9/1.2 = 0.75 >> 0.15
	}
	if eucloruntime.IsAmbiguous(candidates) {
		t.Fatal("expected clear winner, not ambiguous")
	}
}

func TestIsAmbiguous_SingleCandidate_NotAmbiguous(t *testing.T) {
	candidates := []eucloruntime.ModeCandidate{{Mode: "code", Score: 0.5}}
	if eucloruntime.IsAmbiguous(candidates) {
		t.Fatal("single candidate cannot be ambiguous")
	}
}

func TestNormalizeConfidence_PerfectScore(t *testing.T) {
	candidates := []eucloruntime.ModeCandidate{{Mode: "debug", Score: 0.6}}
	conf := eucloruntime.NormalizeConfidence(candidates, 0.6)
	if conf != 1.0 {
		t.Fatalf("expected 1.0, got %.3f", conf)
	}
}

func TestNormalizeConfidence_ZeroWeight(t *testing.T) {
	candidates := []eucloruntime.ModeCandidate{{Mode: "code", Score: 0.3}}
	conf := eucloruntime.NormalizeConfidence(candidates, 0)
	if conf != 0 {
		t.Fatalf("expected 0 on zero weight, got %.3f", conf)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// ClassifyTaskScored — full pipeline
// ─────────────────────────────────────────────────────────────────────────────

func TestClassifyTaskScored_DebugKeywords(t *testing.T) {
	env := eucloruntime.TaskEnvelope{
		Instruction:   "goroutine 7 [running]: panic: nil pointer dereference",
		EditPermitted: true,
	}
	result := eucloruntime.ClassifyTaskScored(env)
	if result.RecommendedMode != "debug" {
		t.Fatalf("expected debug, got %q (candidates: %+v)", result.RecommendedMode, result.Candidates)
	}
}

func TestClassifyTaskScored_ReviewKeywords(t *testing.T) {
	env := eucloruntime.TaskEnvelope{
		Instruction:   "please review this pull request and audit the auth changes",
		EditPermitted: true,
	}
	result := eucloruntime.ClassifyTaskScored(env)
	if result.RecommendedMode != "review" {
		t.Fatalf("expected review, got %q", result.RecommendedMode)
	}
}

func TestClassifyTaskScored_DefaultsToCode_EmptyInstruction(t *testing.T) {
	env := eucloruntime.TaskEnvelope{
		Instruction:   "",
		EditPermitted: true,
	}
	result := eucloruntime.ClassifyTaskScored(env)
	if result.RecommendedMode != "code" {
		t.Fatalf("expected code default, got %q", result.RecommendedMode)
	}
}

func TestClassifyTaskScored_PlanningKeywords(t *testing.T) {
	env := eucloruntime.TaskEnvelope{
		Instruction:   "what architecture should we use for the new service? design the approach",
		EditPermitted: true,
	}
	result := eucloruntime.ClassifyTaskScored(env)
	if result.RecommendedMode != "planning" {
		t.Fatalf("expected planning, got %q", result.RecommendedMode)
	}
	if !result.RequiresDeterministicStages {
		t.Fatal("planning mode should require deterministic stages")
	}
}

func TestClassifyTaskScored_CrossCuttingScope(t *testing.T) {
	env := eucloruntime.TaskEnvelope{
		Instruction:   "implement the change across multiple packages",
		EditPermitted: true,
	}
	result := eucloruntime.ClassifyTaskScored(env)
	if result.Scope != "cross_cutting" {
		t.Fatalf("expected cross_cutting scope, got %q", result.Scope)
	}
	if result.RiskLevel != "medium" {
		t.Fatalf("expected medium risk, got %q", result.RiskLevel)
	}
}

func TestClassifyTaskScored_ReadOnly_LowRisk_Review(t *testing.T) {
	env := eucloruntime.TaskEnvelope{
		Instruction:   "please review this code and audit the security model",
		EditPermitted: false,
	}
	result := eucloruntime.ClassifyTaskScored(env)
	if result.RecommendedMode != "review" {
		t.Fatalf("expected review, got %q", result.RecommendedMode)
	}
	if result.RiskLevel != "low" {
		t.Fatalf("expected low risk for read-only review, got %q", result.RiskLevel)
	}
}

func TestClassifyTaskScored_VerificationTools_ReasonCode(t *testing.T) {
	env := eucloruntime.TaskEnvelope{
		Instruction: "fix the bug",
		CapabilitySnapshot: euclotypes.CapabilitySnapshot{
			HasVerificationTools: true,
			HasWriteTools:        true,
		},
		EditPermitted: true,
	}
	result := eucloruntime.ClassifyTaskScored(env)
	for _, code := range result.ReasonCodes {
		if code == "capability:verification_available" {
			return
		}
	}
	t.Fatalf("expected capability:verification_available reason code, got %+v", result.ReasonCodes)
}

func TestClassifyTaskScored_Debug_RequiresEvidence(t *testing.T) {
	env := eucloruntime.TaskEnvelope{
		Instruction:   "nil pointer panic in the auth handler",
		EditPermitted: true,
	}
	result := eucloruntime.ClassifyTaskScored(env)
	if !result.RequiresEvidenceBeforeMutation {
		t.Fatal("debug mode should require evidence before mutation")
	}
}

func TestClassifyTaskScored_ExplicitVerification_RequiresEvidence(t *testing.T) {
	env := eucloruntime.TaskEnvelope{
		Instruction:          "implement the payment handler",
		ExplicitVerification: "run go test ./payment/...",
		EditPermitted:        true,
	}
	result := eucloruntime.ClassifyTaskScored(env)
	if !result.RequiresEvidenceBeforeMutation {
		t.Fatal("explicit verification string should require evidence before mutation")
	}
}

func TestClassifyTaskScored_ReadOnly_AddsReasonCode(t *testing.T) {
	env := eucloruntime.TaskEnvelope{
		Instruction:   "analyze this",
		EditPermitted: false,
	}
	result := eucloruntime.ClassifyTaskScored(env)
	for _, code := range result.ReasonCodes {
		if code == "constraint:read_only" {
			return
		}
	}
	t.Fatalf("expected constraint:read_only reason code, got %+v", result.ReasonCodes)
}

// ─────────────────────────────────────────────────────────────────────────────
// ResolveMode
// ─────────────────────────────────────────────────────────────────────────────

func TestResolveMode_ExplicitHint_OverridesClassifier(t *testing.T) {
	envelope := eucloruntime.TaskEnvelope{ModeHint: "debug", EditPermitted: true}
	classification := eucloruntime.TaskClassification{
		IntentFamilies:  []string{"code"},
		RecommendedMode: "code",
	}
	registry := euclotypes.DefaultModeRegistry()
	resolution := eucloruntime.ResolveMode(envelope, classification, registry)
	if resolution.ModeID != "debug" {
		t.Fatalf("expected debug from hint, got %q", resolution.ModeID)
	}
	if resolution.Source != "explicit" {
		t.Fatalf("expected source=explicit, got %q", resolution.Source)
	}
}

func TestResolveMode_ResumedMode_FallsBackWhenNoHint(t *testing.T) {
	envelope := eucloruntime.TaskEnvelope{ResumedMode: "review", EditPermitted: true}
	classification := eucloruntime.TaskClassification{
		IntentFamilies:  []string{"code"},
		RecommendedMode: "code",
	}
	registry := euclotypes.DefaultModeRegistry()
	resolution := eucloruntime.ResolveMode(envelope, classification, registry)
	if resolution.ModeID != "review" {
		t.Fatalf("expected review from resumed mode, got %q", resolution.ModeID)
	}
	if resolution.Source != "resumed" {
		t.Fatalf("expected source=resumed, got %q", resolution.Source)
	}
}

func TestResolveMode_NilRegistry_UsesClassifier(t *testing.T) {
	envelope := eucloruntime.TaskEnvelope{EditPermitted: true}
	classification := eucloruntime.TaskClassification{
		IntentFamilies:  []string{"tdd"},
		RecommendedMode: "tdd",
	}
	resolution := eucloruntime.ResolveMode(envelope, classification, nil)
	if resolution.ModeID != "tdd" {
		t.Fatalf("expected tdd from classifier, got %q", resolution.ModeID)
	}
	if resolution.Source != "classifier" {
		t.Fatalf("expected source=classifier, got %q", resolution.Source)
	}
}

func TestResolveMode_UnknownHint_FallsBackToClassifier(t *testing.T) {
	// Registry only has code/debug/etc, not "unknown"
	envelope := eucloruntime.TaskEnvelope{ModeHint: "unknown_mode", EditPermitted: true}
	classification := eucloruntime.TaskClassification{
		IntentFamilies:  []string{"code"},
		RecommendedMode: "code",
	}
	registry := euclotypes.DefaultModeRegistry()
	resolution := eucloruntime.ResolveMode(envelope, classification, registry)
	if resolution.ModeID != "code" {
		t.Fatalf("expected fallback to code, got %q", resolution.ModeID)
	}
	if resolution.Source != "classifier" {
		t.Fatalf("expected source=classifier after fallback, got %q", resolution.Source)
	}
	hasFallbackCode := false
	for _, r := range resolution.ReasonCodes {
		if r == "mode:fallback_to_classifier" {
			hasFallbackCode = true
		}
	}
	if !hasFallbackCode {
		t.Fatalf("expected mode:fallback_to_classifier in reason codes, got %v", resolution.ReasonCodes)
	}
}

func TestResolveMode_EditNotPermitted_AddsConstraint(t *testing.T) {
	envelope := eucloruntime.TaskEnvelope{EditPermitted: false}
	classification := eucloruntime.TaskClassification{
		IntentFamilies:  []string{"code"},
		RecommendedMode: "code",
	}
	resolution := eucloruntime.ResolveMode(envelope, classification, nil)
	for _, c := range resolution.Constraints {
		if c == "mutation_blocked" {
			return
		}
	}
	t.Fatalf("expected mutation_blocked constraint, got %+v", resolution.Constraints)
}

func TestResolveMode_PlanningRequiresDeterministic_ConstraintApplied(t *testing.T) {
	envelope := eucloruntime.TaskEnvelope{EditPermitted: true}
	classification := eucloruntime.TaskClassification{
		IntentFamilies:              []string{"planning"},
		RecommendedMode:             "planning",
		RequiresDeterministicStages: true,
	}
	registry := euclotypes.DefaultModeRegistry()
	resolution := eucloruntime.ResolveMode(envelope, classification, registry)
	if resolution.ModeID != "planning" {
		t.Fatalf("expected planning mode, got %q", resolution.ModeID)
	}
	if resolution.Source != "constraint" {
		t.Fatalf("expected source=constraint for planning, got %q", resolution.Source)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// SelectExecutionProfile
// ─────────────────────────────────────────────────────────────────────────────

func TestSelectExecutionProfile_DebugMode_ReproduceLocalizePatch(t *testing.T) {
	env := eucloruntime.TaskEnvelope{EditPermitted: true}
	class := eucloruntime.TaskClassification{}
	mode := euclotypes.ModeResolution{ModeID: "debug"}
	registry := euclotypes.DefaultExecutionProfileRegistry()
	sel := eucloruntime.SelectExecutionProfile(env, class, mode, registry)
	if sel.ProfileID != "reproduce_localize_patch" {
		t.Fatalf("expected reproduce_localize_patch, got %q", sel.ProfileID)
	}
}

func TestSelectExecutionProfile_TDDMode_TestDrivenGeneration(t *testing.T) {
	env := eucloruntime.TaskEnvelope{EditPermitted: true}
	class := eucloruntime.TaskClassification{}
	mode := euclotypes.ModeResolution{ModeID: "tdd"}
	registry := euclotypes.DefaultExecutionProfileRegistry()
	sel := eucloruntime.SelectExecutionProfile(env, class, mode, registry)
	if sel.ProfileID != "test_driven_generation" {
		t.Fatalf("expected test_driven_generation, got %q", sel.ProfileID)
	}
}

func TestSelectExecutionProfile_ReviewMode_ReviewSuggestImplement(t *testing.T) {
	env := eucloruntime.TaskEnvelope{EditPermitted: true}
	class := eucloruntime.TaskClassification{}
	mode := euclotypes.ModeResolution{ModeID: "review"}
	registry := euclotypes.DefaultExecutionProfileRegistry()
	sel := eucloruntime.SelectExecutionProfile(env, class, mode, registry)
	if sel.ProfileID != "review_suggest_implement" {
		t.Fatalf("expected review_suggest_implement, got %q", sel.ProfileID)
	}
}

func TestSelectExecutionProfile_PlanningMode_PlanStageExecute(t *testing.T) {
	env := eucloruntime.TaskEnvelope{EditPermitted: true}
	class := eucloruntime.TaskClassification{}
	mode := euclotypes.ModeResolution{ModeID: "planning"}
	registry := euclotypes.DefaultExecutionProfileRegistry()
	sel := eucloruntime.SelectExecutionProfile(env, class, mode, registry)
	if sel.ProfileID != "plan_stage_execute" {
		t.Fatalf("expected plan_stage_execute, got %q", sel.ProfileID)
	}
}

func TestSelectExecutionProfile_NilRegistry_DefaultProfile(t *testing.T) {
	env := eucloruntime.TaskEnvelope{EditPermitted: true}
	class := eucloruntime.TaskClassification{}
	mode := euclotypes.ModeResolution{ModeID: "code"}
	sel := eucloruntime.SelectExecutionProfile(env, class, mode, nil)
	if sel.ProfileID != "edit_verify_repair" {
		t.Fatalf("expected edit_verify_repair default, got %q", sel.ProfileID)
	}
}

func TestSelectExecutionProfile_EvidenceFirst_UpgradesToReproduceLocalize(t *testing.T) {
	// When RequiresEvidenceBeforeMutation=true in code mode, profileForCodeMode
	// returns reproduce_localize_patch directly (early return in the function).
	// The outer evidence-first upgrade path only fires when profile was already
	// edit_verify_repair — use that path by passing a pre-existing edit_verify_repair
	// selection indirectly via a nil envelope (which keeps mutation blocked), so we
	// just verify the profile lands on reproduce_localize_patch regardless of path.
	env := eucloruntime.TaskEnvelope{EditPermitted: true}
	class := eucloruntime.TaskClassification{
		RequiresEvidenceBeforeMutation: true,
	}
	mode := euclotypes.ModeResolution{ModeID: "code"}
	registry := euclotypes.DefaultExecutionProfileRegistry()
	sel := eucloruntime.SelectExecutionProfile(env, class, mode, registry)
	if sel.ProfileID != "reproduce_localize_patch" {
		t.Fatalf("expected reproduce_localize_patch for evidence-first code task, got %q", sel.ProfileID)
	}
}

func TestSelectExecutionProfile_SummaryTask_PlanStageExecute(t *testing.T) {
	// "summarize current status" → plan_stage_execute (read-only task)
	env := eucloruntime.TaskEnvelope{
		Instruction:   "summarize current status of the module",
		EditPermitted: true,
	}
	class := eucloruntime.TaskClassification{}
	mode := euclotypes.ModeResolution{ModeID: "code"}
	registry := euclotypes.DefaultExecutionProfileRegistry()
	sel := eucloruntime.SelectExecutionProfile(env, class, mode, registry)
	if sel.ProfileID != "plan_stage_execute" {
		t.Fatalf("expected plan_stage_execute for summary-only task, got %q", sel.ProfileID)
	}
}

func TestSelectExecutionProfile_ReadOnlyEnv_MutationNotAllowed(t *testing.T) {
	env := eucloruntime.TaskEnvelope{EditPermitted: false}
	class := eucloruntime.TaskClassification{}
	mode := euclotypes.ModeResolution{ModeID: "code"}
	registry := euclotypes.DefaultExecutionProfileRegistry()
	sel := eucloruntime.SelectExecutionProfile(env, class, mode, registry)
	if sel.MutationAllowed {
		t.Fatal("mutation should not be allowed when EditPermitted=false")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// SnapshotCapabilities + NormalizeTaskEnvelope — registry seam tests
// ─────────────────────────────────────────────────────────────────────────────

func TestSnapshotCapabilities_NilRegistry(t *testing.T) {
	snap := eucloruntime.SnapshotCapabilities(nil)
	if snap.HasWriteTools || snap.HasReadTools || snap.HasVerificationTools {
		t.Fatalf("nil registry should produce empty snapshot, got %+v", snap)
	}
	if len(snap.ToolNames) != 0 {
		t.Fatalf("nil registry should produce empty tool names, got %+v", snap.ToolNames)
	}
}

func TestSnapshotCapabilities_ReadOnlyRegistry_NoWriteTools(t *testing.T) {
	registry := testutil.RegistryWith(testutil.EchoTool{})
	snap := eucloruntime.SnapshotCapabilities(registry)
	if snap.HasWriteTools {
		t.Fatal("EchoTool has no write permissions; HasWriteTools should be false")
	}
	if len(snap.ToolNames) == 0 {
		t.Fatal("expected echo in tool names")
	}
}

func TestSnapshotCapabilities_WriteRegistry_DetectsWrite(t *testing.T) {
	registry := testutil.RegistryWith(testutil.FileWriteTool{})
	snap := eucloruntime.SnapshotCapabilities(registry)
	if !snap.HasWriteTools {
		t.Fatal("FileWriteTool has write permission; HasWriteTools should be true")
	}
}

func TestSnapshotCapabilities_VerificationToolName_DetectsVerification(t *testing.T) {
	// Tool named "test_runner" triggers HasVerificationTools
	registry := testutil.RegistryWith(testutil.EchoTool{ToolName: "test_runner"})
	snap := eucloruntime.SnapshotCapabilities(registry)
	if !snap.HasVerificationTools {
		t.Fatal("tool named 'test_runner' should set HasVerificationTools=true")
	}
}

func TestSnapshotCapabilities_AST_Tool_DetectsAST(t *testing.T) {
	registry := testutil.RegistryWith(testutil.EchoTool{ToolName: "ast_query"})
	snap := eucloruntime.SnapshotCapabilities(registry)
	if !snap.HasASTOrLSPTools {
		t.Fatal("tool named 'ast_query' should set HasASTOrLSPTools=true")
	}
}

// NormalizeTaskEnvelope seam: registry permissions reach EditPermitted via SnapshotCapabilities.
func TestNormalizeTaskEnvelope_WriteRegistry_EditPermitted(t *testing.T) {
	registry := testutil.RegistryWith(testutil.FileWriteTool{})
	task := &core.Task{
		ID:          "t1",
		Instruction: "implement feature X",
	}
	env := eucloruntime.NormalizeTaskEnvelope(task, nil, registry)
	if !env.EditPermitted {
		t.Fatal("registry with write tool should set EditPermitted=true")
	}
	if !env.CapabilitySnapshot.HasWriteTools {
		t.Fatal("snapshot should reflect write tools")
	}
}

func TestNormalizeTaskEnvelope_ReadOnlyRegistry_EditNotPermitted(t *testing.T) {
	registry := testutil.RegistryWith(testutil.EchoTool{})
	task := &core.Task{ID: "t2", Instruction: "read something"}
	env := eucloruntime.NormalizeTaskEnvelope(task, nil, registry)
	if env.EditPermitted {
		t.Fatal("read-only registry should not set EditPermitted")
	}
}

func TestNormalizeTaskEnvelope_ModeHintFromContext(t *testing.T) {
	task := &core.Task{
		ID:          "t3",
		Instruction: "debug the crash",
		Context:     map[string]any{"euclo.mode": "debug"},
	}
	env := eucloruntime.NormalizeTaskEnvelope(task, nil, nil)
	if env.ModeHint != "debug" {
		t.Fatalf("expected mode hint 'debug', got %q", env.ModeHint)
	}
}

func TestNormalizeTaskEnvelope_NilTask(t *testing.T) {
	registry := testutil.RegistryWith(testutil.FileWriteTool{})
	env := eucloruntime.NormalizeTaskEnvelope(nil, nil, registry)
	// EditPermitted driven by HasWriteTools when task is nil.
	if !env.EditPermitted {
		t.Fatal("nil task with write registry should still set EditPermitted from snapshot")
	}
}

func TestNormalizeTaskEnvelope_ExplicitVerificationFromContext(t *testing.T) {
	task := &core.Task{
		ID:          "t4",
		Instruction: "fix the handler",
		Context:     map[string]any{"verification": "run go test ./..."},
	}
	env := eucloruntime.NormalizeTaskEnvelope(task, nil, nil)
	if env.ExplicitVerification != "run go test ./..." {
		t.Fatalf("expected explicit verification from context, got %q", env.ExplicitVerification)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// End-to-end pipeline: registry → normalize → classify → resolve → profile
// ─────────────────────────────────────────────────────────────────────────────

// TestClassificationPipeline_FullFlow exercises the full inter-package pipeline:
// capability.Registry → NormalizeTaskEnvelope → ClassifyTaskScored → ResolveMode
// → SelectExecutionProfile. Validates that registry tool permissions propagate
// all the way through to the execution profile selection.
func TestClassificationPipeline_WriteRegistry_CodeTask_EditVerifyRepair(t *testing.T) {
	registry := capability.NewRegistry()
	if err := registry.Register(testutil.FileWriteTool{}); err != nil {
		t.Fatalf("register: %v", err)
	}
	task := &core.Task{
		ID:          "pipeline-1",
		Instruction: "implement the new cache layer",
	}
	state := core.NewContext()

	envelope := eucloruntime.NormalizeTaskEnvelope(task, state, registry)
	scored := eucloruntime.ClassifyTaskScored(envelope)
	mode := eucloruntime.ResolveMode(envelope, scored.TaskClassification, euclotypes.DefaultModeRegistry())
	profile := eucloruntime.SelectExecutionProfile(envelope, scored.TaskClassification, mode, euclotypes.DefaultExecutionProfileRegistry())

	if !envelope.EditPermitted {
		t.Fatal("write registry should produce EditPermitted=true")
	}
	if mode.ModeID != "code" {
		t.Fatalf("expected code mode, got %q", mode.ModeID)
	}
	if profile.ProfileID != "edit_verify_repair" {
		t.Fatalf("expected edit_verify_repair profile, got %q", profile.ProfileID)
	}
	if !profile.MutationAllowed {
		t.Fatal("write registry + code mode should allow mutation")
	}
}

func TestClassificationPipeline_ReadOnlyRegistry_ReviewTask_NoMutation(t *testing.T) {
	// EchoTool has no write permissions → EditPermitted=false → profile disallows mutation.
	registry := capability.NewRegistry()
	if err := registry.Register(testutil.EchoTool{}); err != nil {
		t.Fatalf("register: %v", err)
	}
	task := &core.Task{
		ID:          "pipeline-2",
		Instruction: "review this pull request",
	}

	envelope := eucloruntime.NormalizeTaskEnvelope(task, nil, registry)
	scored := eucloruntime.ClassifyTaskScored(envelope)
	mode := eucloruntime.ResolveMode(envelope, scored.TaskClassification, euclotypes.DefaultModeRegistry())
	profile := eucloruntime.SelectExecutionProfile(envelope, scored.TaskClassification, mode, euclotypes.DefaultExecutionProfileRegistry())

	if envelope.EditPermitted {
		t.Fatal("read-only registry should not permit editing")
	}
	if profile.MutationAllowed {
		t.Fatal("read-only registry should produce profile where mutation is not allowed")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func assertSignalMode(t *testing.T, signals []eucloruntime.ClassificationSignal, mode, kind string) {
	t.Helper()
	for _, s := range signals {
		if s.Mode == mode && (kind == "" || s.Kind == kind) {
			return
		}
	}
	t.Fatalf("expected signal mode=%q kind=%q in %+v", mode, kind, signals)
}
