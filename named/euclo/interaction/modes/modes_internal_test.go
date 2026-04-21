package modes

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

// ---------------------------------------------------------------------------
// skipChatReflect
// ---------------------------------------------------------------------------

func TestSkipChatReflect_False_NoFlags(t *testing.T) {
	state := map[string]any{}
	if skipChatReflect(state, nil) {
		t.Fatal("expected false with no flags set")
	}
}

func TestSkipChatReflect_True_SkipFlag(t *testing.T) {
	state := map[string]any{"chat.skip_reflect": true}
	if !skipChatReflect(state, nil) {
		t.Fatal("expected true when chat.skip_reflect is set")
	}
}

func TestSkipChatReflect_True_AskSubModeWithAnswer(t *testing.T) {
	state := map[string]any{
		"chat.sub_mode":    "ask",
		"present.answered": true,
	}
	if !skipChatReflect(state, nil) {
		t.Fatal("expected true for ask sub-mode with answered")
	}
}

func TestSkipChatReflect_False_AskSubModeWithoutAnswer(t *testing.T) {
	state := map[string]any{"chat.sub_mode": "ask"}
	// present.answered not set
	if skipChatReflect(state, nil) {
		t.Fatal("expected false for ask sub-mode without answered")
	}
}

func TestSkipChatReflect_False_NonAskSubMode(t *testing.T) {
	state := map[string]any{"chat.sub_mode": "explore"}
	if skipChatReflect(state, nil) {
		t.Fatal("expected false for non-ask sub-mode")
	}
}

// ---------------------------------------------------------------------------
// skipPropose
// ---------------------------------------------------------------------------

func TestSkipPropose_False_NoFlags(t *testing.T) {
	state := map[string]any{}
	if skipPropose(state, nil) {
		t.Fatal("expected false with no flags")
	}
}

func TestSkipPropose_True_JustDoIt(t *testing.T) {
	state := map[string]any{"just_do_it": true}
	if !skipPropose(state, nil) {
		t.Fatal("expected true for just_do_it")
	}
}

func TestSkipPropose_True_SmallChange(t *testing.T) {
	state := map[string]any{"understand.small_change": true}
	if !skipPropose(state, nil) {
		t.Fatal("expected true for small change")
	}
}

// ---------------------------------------------------------------------------
// skipIntake
// ---------------------------------------------------------------------------

func TestSkipIntake_False_NoFlags(t *testing.T) {
	state := map[string]any{}
	if skipIntake(state, nil) {
		t.Fatal("expected false with no flags")
	}
}

func TestSkipIntake_True_HasEvidence(t *testing.T) {
	state := map[string]any{"has_evidence": true}
	if !skipIntake(state, nil) {
		t.Fatal("expected true for has_evidence")
	}
}

func TestSkipIntake_True_RequiresEvidenceWithSignal(t *testing.T) {
	state := map[string]any{
		"requires_evidence_before_mutation": true,
		"evidence_in_instruction":           "stack trace",
	}
	if !skipIntake(state, nil) {
		t.Fatal("expected true when requires_evidence but signal present")
	}
}

func TestSkipIntake_False_RequiresEvidenceWithoutSignal(t *testing.T) {
	state := map[string]any{
		"requires_evidence_before_mutation": true,
		// evidence_in_instruction absent
	}
	if skipIntake(state, nil) {
		t.Fatal("expected false when requires_evidence but no signal")
	}
}

// ---------------------------------------------------------------------------
// skipReproduce
// ---------------------------------------------------------------------------

func TestSkipReproduce_False_NoFlags(t *testing.T) {
	if skipReproduce(map[string]any{}, nil) {
		t.Fatal("expected false with no flags")
	}
}

func TestSkipReproduce_True_SkipFlag(t *testing.T) {
	if !skipReproduce(map[string]any{"skip_reproduction": true}, nil) {
		t.Fatal("expected true for skip_reproduction")
	}
}

func TestSkipReproduce_True_KnownCause(t *testing.T) {
	if !skipReproduce(map[string]any{"known_cause": true}, nil) {
		t.Fatal("expected true for known_cause")
	}
}

// ---------------------------------------------------------------------------
// skipClarify
// ---------------------------------------------------------------------------

func TestSkipClarify_False_NoFlags(t *testing.T) {
	if skipClarify(map[string]any{}, nil) {
		t.Fatal("expected false with no flags")
	}
}

func TestSkipClarify_True_JustPlanIt(t *testing.T) {
	if !skipClarify(map[string]any{"just_plan_it": true}, nil) {
		t.Fatal("expected true for just_plan_it")
	}
}

func TestSkipClarify_True_ScopeConfirmed(t *testing.T) {
	if !skipClarify(map[string]any{"scope.response": "confirm"}, nil) {
		t.Fatal("expected true when scope.response=confirm")
	}
}

func TestSkipClarify_False_ScopeNotConfirmed(t *testing.T) {
	if skipClarify(map[string]any{"scope.response": "correct"}, nil) {
		t.Fatal("expected false when scope.response=correct")
	}
}

// ---------------------------------------------------------------------------
// skipCompare
// ---------------------------------------------------------------------------

func TestSkipCompare_False_MultipleNonSelected(t *testing.T) {
	state := map[string]any{"generate.candidate_count": 3}
	if skipCompare(state, nil) {
		t.Fatal("expected false with 3 candidates and none selected")
	}
}

func TestSkipCompare_True_JustPlanIt(t *testing.T) {
	if !skipCompare(map[string]any{"just_plan_it": true}, nil) {
		t.Fatal("expected true for just_plan_it")
	}
}

func TestSkipCompare_True_OneCandidate(t *testing.T) {
	if !skipCompare(map[string]any{"generate.candidate_count": 1}, nil) {
		t.Fatal("expected true with only 1 candidate")
	}
}

func TestSkipCompare_True_ZeroCandidates(t *testing.T) {
	// count=0 <= 1 → skip
	if !skipCompare(map[string]any{"generate.candidate_count": 0}, nil) {
		t.Fatal("expected true with 0 candidates")
	}
}

func TestSkipCompare_True_AlreadySelected(t *testing.T) {
	state := map[string]any{
		"generate.candidate_count": 3,
		"generate.selected":        "plan-b",
	}
	if !skipCompare(state, nil) {
		t.Fatal("expected true when already selected")
	}
}

// ---------------------------------------------------------------------------
// skipRefine
// ---------------------------------------------------------------------------

func TestSkipRefine_False_NoFlags(t *testing.T) {
	if skipRefine(map[string]any{}, nil) {
		t.Fatal("expected false with no flags")
	}
}

func TestSkipRefine_True_JustPlanIt(t *testing.T) {
	if !skipRefine(map[string]any{"just_plan_it": true}, nil) {
		t.Fatal("expected true for just_plan_it")
	}
}

// ---------------------------------------------------------------------------
// skipAct
// ---------------------------------------------------------------------------

func TestSkipAct_False_NoFlags(t *testing.T) {
	if skipAct(map[string]any{}, nil) {
		t.Fatal("expected false with no flags")
	}
}

func TestSkipAct_True_NoFixes(t *testing.T) {
	if !skipAct(map[string]any{"triage.no_fixes": true}, nil) {
		t.Fatal("expected true when triage.no_fixes")
	}
}

// ---------------------------------------------------------------------------
// skipReReview
// ---------------------------------------------------------------------------

func TestSkipReReview_False_NoFlags(t *testing.T) {
	if skipReReview(map[string]any{}, nil) {
		t.Fatal("expected false with no flags")
	}
}

func TestSkipReReview_True_SkipFlag(t *testing.T) {
	if !skipReReview(map[string]any{"act.skip_re_review": true}, nil) {
		t.Fatal("expected true for act.skip_re_review")
	}
}

func TestSkipReReview_True_ActSkipped(t *testing.T) {
	if !skipReReview(map[string]any{"triage.no_fixes": true}, nil) {
		t.Fatal("expected true when act was skipped (no_fixes)")
	}
}

// ---------------------------------------------------------------------------
// skipSpecify
// ---------------------------------------------------------------------------

func TestSkipSpecify_False_NoFlags(t *testing.T) {
	if skipSpecify(map[string]any{}, nil) {
		t.Fatal("expected false with no flags")
	}
}

func TestSkipSpecify_True_HasTestSpecs(t *testing.T) {
	if !skipSpecify(map[string]any{"has_test_specs": true}, nil) {
		t.Fatal("expected true for has_test_specs")
	}
}

// ---------------------------------------------------------------------------
// planArtifact
// ---------------------------------------------------------------------------

func TestPlanArtifact_WithSummary(t *testing.T) {
	state := map[string]any{
		"generate.selected_summary": "implement auth module",
		"refine.items":              []string{"step1", "step2"},
	}
	a := planArtifact(state)
	if a.Kind != euclotypes.ArtifactKindPlan {
		t.Fatalf("expected ArtifactKindPlan, got %q", a.Kind)
	}
	if a.Summary != "implement auth module" {
		t.Fatalf("expected summary from state, got %q", a.Summary)
	}
	if a.ID != "interaction_plan" {
		t.Fatalf("expected ID=interaction_plan, got %q", a.ID)
	}
}

func TestPlanArtifact_DefaultSummary(t *testing.T) {
	a := planArtifact(map[string]any{})
	if a.Summary != "plan generated" {
		t.Fatalf("expected default summary, got %q", a.Summary)
	}
}

// ---------------------------------------------------------------------------
// chatPhases: classifyChatSubMode (package-level, testable internally)
// ---------------------------------------------------------------------------

func TestClassifyChatSubMode_Ask(t *testing.T) {
	result := classifyChatSubMode("what does this function do?")
	if result != "ask" {
		t.Fatalf("expected 'ask', got %q", result)
	}
}

func TestClassifyChatSubMode_Implement(t *testing.T) {
	result := classifyChatSubMode("implement the login feature")
	if result != "implement" {
		t.Fatalf("expected 'implement', got %q", result)
	}
}

func TestClassifyChatSubMode_Other(t *testing.T) {
	result := classifyChatSubMode("this is some generic request")
	if result == "" {
		t.Fatal("expected non-empty classification")
	}
}

// ---------------------------------------------------------------------------
// interaction ArtifactBundle helper (modes use NewArtifactBundle internally)
// ---------------------------------------------------------------------------

func TestArtifactBundleInModes(t *testing.T) {
	b := interaction.NewArtifactBundle()
	b.Add(euclotypes.Artifact{Kind: euclotypes.ArtifactKindPlan, Summary: "test"})
	if !b.Has(euclotypes.ArtifactKindPlan) {
		t.Fatal("expected bundle to have plan artifact")
	}
}

// ---------------------------------------------------------------------------
// tddStateRecord
// ---------------------------------------------------------------------------

func TestTddStateRecord_NilState(t *testing.T) {
	if tddStateRecord(nil, "key") != nil {
		t.Fatal("expected nil for nil state")
	}
}

func TestTddStateRecord_MissingKey(t *testing.T) {
	if tddStateRecord(map[string]any{}, "key") != nil {
		t.Fatal("expected nil for missing key")
	}
}

func TestTddStateRecord_NilValue(t *testing.T) {
	if tddStateRecord(map[string]any{"key": nil}, "key") != nil {
		t.Fatal("expected nil for nil value")
	}
}

func TestTddStateRecord_MapValue(t *testing.T) {
	state := map[string]any{"key": map[string]any{"status": "pass"}}
	record := tddStateRecord(state, "key")
	if record == nil || record["status"] != "pass" {
		t.Fatalf("expected map value, got %v", record)
	}
}

// ---------------------------------------------------------------------------
// tddRedResultFromState
// ---------------------------------------------------------------------------

func TestTddRedResultFromState_EmptyPayload(t *testing.T) {
	result := tddRedResultFromState(map[string]any{})
	if result.Status != "all_red" {
		t.Fatalf("expected all_red, got %q", result.Status)
	}
	if len(result.Evidence) == 0 {
		t.Fatal("expected default evidence")
	}
}

func TestTddRedResultFromState_WithPayloadFail(t *testing.T) {
	state := map[string]any{
		"euclo.tdd.red_evidence": map[string]any{"status": "fail", "summary": "tests failing"},
	}
	result := tddRedResultFromState(state)
	if result.Status != "all_red" {
		t.Fatalf("expected all_red, got %q", result.Status)
	}
}

func TestTddRedResultFromState_WithPayloadPass(t *testing.T) {
	state := map[string]any{
		"euclo.tdd.red_evidence": map[string]any{"status": "pass"},
	}
	result := tddRedResultFromState(state)
	if result.Status != "passed" {
		t.Fatalf("expected passed, got %q", result.Status)
	}
}

func TestTddRedResultFromState_SummaryFallback(t *testing.T) {
	state := map[string]any{
		"euclo.tdd.red_evidence": map[string]any{"status": "other", "summary": "custom summary"},
	}
	result := tddRedResultFromState(state)
	if len(result.Evidence) == 0 {
		t.Fatal("expected evidence from summary fallback")
	}
}

// ---------------------------------------------------------------------------
// tddGreenResultFromState
// ---------------------------------------------------------------------------

func TestTddGreenResultFromState_EmptyPayload(t *testing.T) {
	result := tddGreenResultFromState(map[string]any{})
	if result.Status != "passed" {
		t.Fatalf("expected passed, got %q", result.Status)
	}
}

func TestTddGreenResultFromState_WithGreenEvidence(t *testing.T) {
	state := map[string]any{
		"euclo.tdd.green_evidence": map[string]any{"status": "pass"},
	}
	result := tddGreenResultFromState(state)
	if result.Status != "passed" {
		t.Fatalf("expected passed, got %q", result.Status)
	}
}

func TestTddGreenResultFromState_FallbackRefactorEvidence(t *testing.T) {
	state := map[string]any{
		"euclo.tdd.refactor_evidence": map[string]any{"status": "fail"},
	}
	result := tddGreenResultFromState(state)
	if result.Status != "failed" {
		t.Fatalf("expected failed, got %q", result.Status)
	}
}

func TestTddGreenResultFromState_UnknownStatus(t *testing.T) {
	state := map[string]any{
		"euclo.tdd.green_evidence": map[string]any{"status": "unknown", "summary": "partial run"},
	}
	result := tddGreenResultFromState(state)
	if result.Status != "partial" {
		t.Fatalf("expected partial, got %q", result.Status)
	}
}

// ---------------------------------------------------------------------------
// verificationEvidenceItems
// ---------------------------------------------------------------------------

func TestVerificationEvidenceItems_NoChecks(t *testing.T) {
	items := verificationEvidenceItems(map[string]any{})
	if items != nil {
		t.Fatal("expected nil for missing checks key")
	}
}

func TestVerificationEvidenceItems_NilChecks(t *testing.T) {
	items := verificationEvidenceItems(map[string]any{"checks": nil})
	if items != nil {
		t.Fatal("expected nil for nil checks")
	}
}

func TestVerificationEvidenceItems_AnySlice(t *testing.T) {
	payload := map[string]any{
		"checks": []any{
			map[string]any{"details": "test passed", "working_directory": "/src"},
			map[string]any{"name": "test_b", "status": "pass"},
			map[string]any{}, // empty record — skipped
		},
	}
	items := verificationEvidenceItems(payload)
	if len(items) != 2 {
		t.Fatalf("expected 2 evidence items, got %d", len(items))
	}
	if items[0].Detail != "test passed" {
		t.Fatalf("expected detail 'test passed', got %q", items[0].Detail)
	}
	if items[0].Location != "/src" {
		t.Fatalf("expected location '/src', got %q", items[0].Location)
	}
}

func TestVerificationEvidenceItems_MapSlice(t *testing.T) {
	payload := map[string]any{
		"checks": []map[string]any{
			{"name": "TestFoo", "status": "pass", "working_directory": "/app"},
		},
	}
	items := verificationEvidenceItems(payload)
	if len(items) != 1 {
		t.Fatalf("expected 1 evidence item, got %d", len(items))
	}
}

// ---------------------------------------------------------------------------
// firstNonEmpty
// ---------------------------------------------------------------------------

func TestFirstNonEmpty_ReturnsFirst(t *testing.T) {
	if firstNonEmpty("a", "b", "c") != "a" {
		t.Fatal("expected first non-empty")
	}
}

func TestFirstNonEmpty_SkipsWhitespace(t *testing.T) {
	if firstNonEmpty("  ", "", "found") != "found" {
		t.Fatal("expected to skip whitespace")
	}
}

func TestFirstNonEmpty_AllEmpty(t *testing.T) {
	if firstNonEmpty("", "  ", "") != "" {
		t.Fatal("expected empty string")
	}
}

// ---------------------------------------------------------------------------
// normalizeReviewFindings
// ---------------------------------------------------------------------------

func TestNormalizeReviewFindings_MapSlice(t *testing.T) {
	input := []map[string]any{{"issue": "x"}, {"issue": "y"}}
	out := normalizeReviewFindings(input)
	if len(out) != 2 {
		t.Fatalf("expected 2, got %d", len(out))
	}
}

func TestNormalizeReviewFindings_AnySlice(t *testing.T) {
	input := []any{
		map[string]any{"issue": "a"},
		"not a map", // skipped
		map[string]any{"issue": "b"},
	}
	out := normalizeReviewFindings(input)
	if len(out) != 2 {
		t.Fatalf("expected 2 (non-maps skipped), got %d", len(out))
	}
}

func TestNormalizeReviewFindings_NilOrUnknown(t *testing.T) {
	if normalizeReviewFindings(nil) != nil {
		t.Fatal("expected nil for nil input")
	}
	if normalizeReviewFindings("string") != nil {
		t.Fatal("expected nil for string input")
	}
}

// ---------------------------------------------------------------------------
// emitSpecMatrix
// ---------------------------------------------------------------------------

func TestEmitSpecMatrix_EmitsFrameWithItems(t *testing.T) {
	noop := &interaction.NoopEmitter{}
	spec := BehaviorSpec{
		HappyPaths: []BehaviorCase{{Description: "returns sum"}},
		EdgeCases:  []BehaviorCase{{Description: "overflow"}},
		ErrorCases: []BehaviorCase{{Description: "nil input"}},
	}
	mc := interaction.PhaseMachineContext{
		Emitter: noop,
		Mode:    "tdd",
		Phase:   "specify",
		State:   map[string]any{},
	}
	if err := emitSpecMatrix(context.Background(), mc, spec); err != nil {
		t.Fatalf("emitSpecMatrix error: %v", err)
	}
	if len(noop.Frames) != 1 {
		t.Fatalf("expected 1 frame, got %d", len(noop.Frames))
	}
	if noop.Frames[0].Kind != interaction.FrameDraft {
		t.Fatalf("expected FrameDraft, got %q", noop.Frames[0].Kind)
	}
	content, ok := noop.Frames[0].Content.(interaction.DraftContent)
	if !ok {
		t.Fatalf("expected DraftContent, got %T", noop.Frames[0].Content)
	}
	if len(content.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(content.Items))
	}
}

func TestEmitSpecMatrix_EmptySpec(t *testing.T) {
	noop := &interaction.NoopEmitter{}
	mc := interaction.PhaseMachineContext{
		Emitter: noop,
		Mode:    "tdd",
		Phase:   "specify",
		State:   map[string]any{},
	}
	if err := emitSpecMatrix(context.Background(), mc, BehaviorSpec{}); err != nil {
		t.Fatalf("emitSpecMatrix error: %v", err)
	}
	content, _ := noop.Frames[0].Content.(interaction.DraftContent)
	if len(content.Items) != 0 {
		t.Fatalf("expected 0 items for empty spec, got %d", len(content.Items))
	}
}

// ---------------------------------------------------------------------------
// defaultBehaviorQuestions
// ---------------------------------------------------------------------------

func TestDefaultBehaviorQuestions_NotEmpty(t *testing.T) {
	questions := defaultBehaviorQuestions()
	if len(questions) == 0 {
		t.Fatal("expected non-empty default behavior questions")
	}
	for _, q := range questions {
		if q.Question == "" {
			t.Fatal("expected non-empty question text")
		}
	}
}
