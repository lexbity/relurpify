package modes

import (
	"context"
	"reflect"
	"testing"

	"codeburg.org/lexbit/relurpify/ayenitd"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	"codeburg.org/lexbit/relurpify/named/euclo/runtime/pretask"
)

func newScriptedEmitter(responses ...interaction.ScriptedResponse) *interaction.TestFrameEmitter {
	return interaction.NewTestFrameEmitter(responses...)
}

func testPhaseContext(state map[string]any, emitter interaction.FrameEmitter) interaction.PhaseMachineContext {
	if state == nil {
		state = map[string]any{}
	}
	if emitter == nil {
		emitter = newScriptedEmitter()
	}
	return interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      state,
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "test",
		Phase:      "phase",
		PhaseIndex: 1,
		PhaseCount: 3,
	}
}

func TestChatModeWithContextMatchesLegacyWrapper(t *testing.T) {
	m1 := ChatModeLegacy(&interaction.NoopEmitter{}, interaction.NewAgencyResolver())
	m2 := ChatModeWithContext(&interaction.NoopEmitter{}, interaction.NewAgencyResolver(), ayenitd.WorkspaceEnvironment{})
	if m1 == nil || m2 == nil {
		t.Fatal("expected non-nil machines")
	}
	if m1.CurrentPhase() != m2.CurrentPhase() {
		t.Fatalf("expected same entry phase, got %q vs %q", m1.CurrentPhase(), m2.CurrentPhase())
	}
}

func TestLoadAndSaveSessionPins(t *testing.T) {
	store := &stubMemoryStore{
		recall: &memory.MemoryRecord{
			Value: map[string]interface{}{"paths": []interface{}{"a", "", "b"}},
		},
	}
	if got := loadSessionPinsFromMemory(context.Background(), store); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("unexpected loaded pins: %#v", got)
	}
	saveSessionPinsToMemory(context.Background(), store, []string{"x", "y"})
	if len(store.remembered) == 0 {
		t.Fatal("expected pins to be remembered")
	}
}

func TestTrustClassToInsertionAction(t *testing.T) {
	cases := map[string]string{
		"":                  "direct",
		"builtin-trusted":   "direct",
		"workspace-trusted": "direct",
		"remote-approved":   "summarized",
		"other":             "metadata-only",
	}
	for input, want := range cases {
		if got := trustClassToInsertionAction(input); got != want {
			t.Fatalf("trustClassToInsertionAction(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestConvertToContextProposalContent(t *testing.T) {
	bundle := pretask.EnrichedContextBundle{
		AnchoredFiles:     []pretask.CodeEvidenceItem{{Path: "a.go", Summary: "anchor", Score: 1, Source: pretask.EvidenceSourceAnchor, TrustClass: "workspace-trusted"}},
		ExpandedFiles:     []pretask.CodeEvidenceItem{{Path: "b.go", Summary: "expanded", Score: 0.5, Source: pretask.EvidenceSourceIndex, TrustClass: "remote-approved"}},
		KnowledgeTopic:    []pretask.KnowledgeEvidenceItem{{RefID: "k1", Kind: pretask.KnowledgeKindDecision, Title: "Decision", Summary: "topic", Source: pretask.EvidenceSourceArchaeoTopic, TrustClass: "builtin-trusted"}},
		KnowledgeExpanded: []pretask.KnowledgeEvidenceItem{{RefID: "k2", Kind: pretask.KnowledgeKindInteraction, Title: "Interaction", Summary: "expanded", Source: pretask.EvidenceSourceArchaeoExpanded, TrustClass: "custom"}},
		PipelineTrace:     pretask.PipelineTrace{AnchorsExtracted: 1, TotalTokenEstimate: 33},
	}
	content := convertToContextProposalContent(bundle)
	if len(content.AnchoredFiles) != 1 || content.AnchoredFiles[0].InsertionAction != "direct" {
		t.Fatalf("unexpected anchored conversion: %#v", content.AnchoredFiles)
	}
	if len(content.ExpandedFiles) != 1 || content.ExpandedFiles[0].InsertionAction != "summarized" {
		t.Fatalf("unexpected expanded conversion: %#v", content.ExpandedFiles)
	}
	if len(content.KnowledgeItems) != 2 {
		t.Fatalf("unexpected knowledge conversion: %#v", content.KnowledgeItems)
	}
}

func TestScopePhaseDefaultAndExecuteBranches(t *testing.T) {
	emitter := newScriptedEmitter(interaction.ScriptedResponse{ActionID: "clarify", Text: "narrow scope"})
	phase := &ScopePhase{}
	outcome, err := phase.Execute(context.Background(), testPhaseContext(map[string]any{"instruction": "Review this change"}, emitter))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if outcome.StateUpdates["scope.correction"] != "narrow scope" {
		t.Fatalf("expected clarify correction, got %#v", outcome.StateUpdates)
	}
	if got := defaultScopeProposal(testPhaseContext(map[string]any{}, nil)); got.Approach != "plan_stage_execute" {
		t.Fatalf("unexpected default scope proposal: %#v", got)
	}
}

func TestGenerateAndCommitPhases(t *testing.T) {
	genEmitter := newScriptedEmitter(interaction.ScriptedResponse{ActionID: "more"})
	gen := &GeneratePhase{}
	genOutcome, err := gen.Execute(context.Background(), testPhaseContext(nil, genEmitter))
	if err != nil {
		t.Fatalf("GeneratePhase.Execute: %v", err)
	}
	if genOutcome.JumpTo != "generate" {
		t.Fatalf("expected more to jump back to generate, got %#v", genOutcome)
	}
	if len(genOutcome.StateUpdates["generate.candidates"].([]interaction.Candidate)) == 0 {
		t.Fatal("expected generated candidates")
	}

	commitEmitter := newScriptedEmitter(interaction.ScriptedResponse{ActionID: "execute"})
	commit := &CommitPhase{}
	commitCtx := testPhaseContext(map[string]any{"generate.selected_summary": "selected plan"}, commitEmitter)
	commitCtx.Artifacts.Add(euclotypes.Artifact{Kind: euclotypes.ArtifactKindTrace})
	commitOutcome, err := commit.Execute(context.Background(), commitCtx)
	if err != nil {
		t.Fatalf("CommitPhase.Execute: %v", err)
	}
	if commitOutcome.Transition != "code" {
		t.Fatalf("expected commit execute to transition to code, got %#v", commitOutcome)
	}
	if len(commitOutcome.Artifacts) != 1 || commitOutcome.Artifacts[0].Kind != euclotypes.ArtifactKindPlan {
		t.Fatalf("expected plan artifact, got %#v", commitOutcome.Artifacts)
	}
}

func TestCodeIntentAndProposalAndExecutionPhases(t *testing.T) {
	intentEmitter := newScriptedEmitter(interaction.ScriptedResponse{ActionID: "plan_first"})
	intentPhase := &IntentPhase{}
	intentOutcome, err := intentPhase.Execute(context.Background(), testPhaseContext(map[string]any{"instruction": "Update docs"}, intentEmitter))
	if err != nil {
		t.Fatalf("IntentPhase.Execute: %v", err)
	}
	if intentOutcome.Transition != "planning" {
		t.Fatalf("expected plan_first to transition to planning, got %#v", intentOutcome)
	}
	if got := defaultIntentAnalysis(testPhaseContext(nil, nil)); got.Approach != "edit_verify_repair" || !got.MutationFlag {
		t.Fatalf("unexpected default intent analysis: %#v", got)
	}

	proposeEmitter := newScriptedEmitter(interaction.ScriptedResponse{ActionID: "skip"})
	proposePhase := &EditProposalPhase{}
	proposeCtx := testPhaseContext(map[string]any{"scope": []string{"file-a.go", "file-b.go"}}, proposeEmitter)
	proposeOutcome, err := proposePhase.Execute(context.Background(), proposeCtx)
	if err != nil {
		t.Fatalf("EditProposalPhase.Execute: %v", err)
	}
	if proposeOutcome.JumpTo != "verify" {
		t.Fatalf("expected skip to jump to verify, got %#v", proposeOutcome)
	}
	if got := defaultEditProposal(testPhaseContext(map[string]any{}, nil)); len(got.Items) != 1 {
		t.Fatalf("expected fallback edit proposal, got %#v", got)
	}

	execEmitter := newScriptedEmitter(interaction.ScriptedResponse{ActionID: "continue"})
	execPhase := &CodeExecutionPhase{
		RunCapability: func(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.ResultContent, []euclotypes.Artifact, error) {
			return interaction.ResultContent{Status: "completed"}, []euclotypes.Artifact{{Kind: euclotypes.ArtifactKindExecutionStatus}}, nil
		},
	}
	execOutcome, err := execPhase.Execute(context.Background(), testPhaseContext(nil, execEmitter))
	if err != nil {
		t.Fatalf("CodeExecutionPhase.Execute: %v", err)
	}
	if len(execOutcome.Artifacts) != 1 {
		t.Fatalf("expected one artifact, got %#v", execOutcome.Artifacts)
	}

	presentEmitter := newScriptedEmitter(interaction.ScriptedResponse{ActionID: "review"})
	presentPhase := &CodePresentPhase{}
	presentCtx := testPhaseContext(map[string]any{"verify.result": interaction.ResultContent{Status: "passed"}}, presentEmitter)
	presentCtx.Artifacts.Add(euclotypes.Artifact{Kind: euclotypes.ArtifactKindVerification})
	presentOutcome, err := presentPhase.Execute(context.Background(), presentCtx)
	if err != nil {
		t.Fatalf("CodePresentPhase.Execute: %v", err)
	}
	if presentOutcome.Transition != "review" {
		t.Fatalf("expected review transition, got %#v", presentOutcome)
	}
}

func TestCodeVerifyBuildAndExecuteBranches(t *testing.T) {
	passedActions := buildVerifyActions(interaction.ResultContent{Status: "passed"}, 0)
	if len(passedActions) != 2 || passedActions[0].ID != "done" {
		t.Fatalf("unexpected passed actions: %#v", passedActions)
	}
	failedActions := buildVerifyActions(interaction.ResultContent{Status: "failed"}, VerifyFailureThreshold)
	if failedActions[0].ID != "debug" || failedActions[1].Default {
		t.Fatalf("unexpected threshold actions: %#v", failedActions)
	}

	emitter := newScriptedEmitter(interaction.ScriptedResponse{ActionID: "debug"})
	phase := &VerificationPhase{}
	outcome, err := phase.Execute(context.Background(), testPhaseContext(map[string]any{"verify.failure_count": 1}, emitter))
	if err != nil {
		t.Fatalf("VerificationPhase.Execute: %v", err)
	}
	if outcome.Transition != "debug" {
		t.Fatalf("expected debug transition, got %#v", outcome)
	}
}

func TestPlanningPhasesAndHelpers(t *testing.T) {
	scopePhase := &ScopePhase{}
	scopeOutcome, err := scopePhase.Execute(context.Background(), testPhaseContext(map[string]any{"instruction": "Plan it"}, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "confirm"})))
	if err != nil {
		t.Fatalf("ScopePhase.Execute: %v", err)
	}
	if scopeOutcome.StateUpdates["scope.response"] != "confirm" {
		t.Fatalf("unexpected scope response: %#v", scopeOutcome.StateUpdates)
	}

	compare := &ComparePhase{}
	compareOutcome, err := compare.Execute(context.Background(), testPhaseContext(map[string]any{"generate.candidates": []interaction.Candidate{{ID: "a", Summary: "A", Properties: map[string]string{"mode": "fast"}}}}, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "recommended"})))
	if err != nil {
		t.Fatalf("ComparePhase.Execute: %v", err)
	}
	if compareOutcome.StateUpdates["generate.selected"] != "a" {
		t.Fatalf("unexpected recommended selection: %#v", compareOutcome.StateUpdates)
	}
	if got := defaultComparison([]interaction.Candidate{{ID: "a", Properties: map[string]string{"x": "1"}}}); len(got.Matrix) != 1 {
		t.Fatalf("unexpected default comparison: %#v", got)
	}

	refine := &RefinePhase{}
	refineOutcome, err := refine.Execute(context.Background(), testPhaseContext(map[string]any{"generate.selected_summary": "summary"}, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "commit"})))
	if err != nil {
		t.Fatalf("RefinePhase.Execute: %v", err)
	}
	if refineOutcome.StateUpdates["refine.response"] != "commit" {
		t.Fatalf("unexpected refine response: %#v", refineOutcome.StateUpdates)
	}
	if got := defaultDraft(testPhaseContext(map[string]any{}, nil)); len(got.Items) != 1 {
		t.Fatalf("unexpected default draft: %#v", got)
	}
}

func TestDebugPhasesAndHelpers(t *testing.T) {
	intake := &DiagnosticIntakePhase{}
	intakeOutcome, err := intake.Execute(context.Background(), testPhaseContext(nil, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "skip"})))
	if err != nil {
		t.Fatalf("DiagnosticIntakePhase.Execute: %v", err)
	}
	if intakeOutcome.StateUpdates["intake.question_count"] != 3 {
		t.Fatalf("unexpected intake count: %#v", intakeOutcome.StateUpdates)
	}
	if got := defaultDiagnosticQuestions(); len(got) != 3 {
		t.Fatalf("unexpected default questions: %#v", got)
	}

	repro := &ReproductionPhase{}
	reproOutcome, err := repro.Execute(context.Background(), testPhaseContext(nil, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "skip", Text: "panic in handler"})))
	if err != nil {
		t.Fatalf("ReproductionPhase.Execute: %v", err)
	}
	if reproOutcome.StateUpdates["known_cause"] != true {
		t.Fatalf("expected known cause flag, got %#v", reproOutcome.StateUpdates)
	}

	fix := &DebugFixProposalPhase{}
	fixOutcome, err := fix.Execute(context.Background(), testPhaseContext(map[string]any{"localize.result": interaction.ResultContent{Evidence: []interaction.EvidenceItem{{Location: "file.go:12"}}}}, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "reject"})))
	if err != nil {
		t.Fatalf("DebugFixProposalPhase.Execute: %v", err)
	}
	if fixOutcome.JumpTo != "localize" {
		t.Fatalf("unexpected fix outcome: %#v", fixOutcome)
	}
	if got := defaultFixProposal(testPhaseContext(map[string]any{"localize.result": interaction.ResultContent{Evidence: []interaction.EvidenceItem{{Location: "file.go:12"}}}}, nil)); got.Items[0].Content != "Fix at file.go:12" {
		t.Fatalf("unexpected default fix proposal: %#v", got)
	}

	localize := &LocalizationPhase{}
	localizeOutcome, err := localize.Execute(context.Background(), testPhaseContext(nil, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "override", Text: "pkg/root.go"})))
	if err != nil {
		t.Fatalf("LocalizationPhase.Execute: %v", err)
	}
	if localizeOutcome.StateUpdates["localize.override_location"] != "pkg/root.go" {
		t.Fatalf("unexpected localization override: %#v", localizeOutcome.StateUpdates)
	}
}

func TestReviewPhasesAndHelpers(t *testing.T) {
	scope := &ReviewScopePhase{}
	scopeOutcome, err := scope.Execute(context.Background(), testPhaseContext(map[string]any{"instruction": "Review the patch", "scope": []string{"a.go"}}, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "compat"})))
	if err != nil {
		t.Fatalf("ReviewScopePhase.Execute: %v", err)
	}
	if scopeOutcome.StateUpdates["scope.check_compatibility"] != true {
		t.Fatalf("unexpected review scope updates: %#v", scopeOutcome.StateUpdates)
	}
	if got := defaultReviewScope(testPhaseContext(map[string]any{}, nil)); got.Interpretation != "Review recent changes" {
		t.Fatalf("unexpected default review scope: %#v", got)
	}

	sweep := &ReviewSweepPhase{}
	sweepOutcome, err := sweep.Execute(context.Background(), testPhaseContext(map[string]any{
		"euclo.review_findings": map[string]any{
			"findings": []any{
				map[string]any{"severity": "critical", "location": "x", "description": "bad", "suggestion": "fix"},
			},
			"approval_decision": map[string]any{"status": "approved"},
		},
	}, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "done"})))
	if err != nil {
		t.Fatalf("ReviewSweepPhase.Execute: %v", err)
	}
	if sweepOutcome.StateUpdates["sweep.approval_status"] != "approved" {
		t.Fatalf("unexpected sweep approval: %#v", sweepOutcome.StateUpdates)
	}

	triage := &TriagePhase{}
	triageOutcome, err := triage.Execute(context.Background(), testPhaseContext(map[string]any{"sweep.findings": interaction.FindingsContent{Critical: []interaction.Finding{{Description: "c"}}}}, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "pick"})))
	if err != nil {
		t.Fatalf("TriagePhase.Execute: %v", err)
	}
	if triageOutcome.StateUpdates["triage.fix_scope"] != "selected" {
		t.Fatalf("unexpected triage updates: %#v", triageOutcome.StateUpdates)
	}
	if len(buildTriageActions(interaction.FindingsContent{})) != 3 {
		t.Fatalf("unexpected empty triage actions")
	}

	batch := &BatchFixPhase{}
	batchOutcome, err := batch.Execute(context.Background(), testPhaseContext(map[string]any{"triage.fix_scope": "all"}, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "accept"})))
	if err != nil {
		t.Fatalf("BatchFixPhase.Execute: %v", err)
	}
	if batchOutcome.StateUpdates["act.skip_re_review"] != true {
		t.Fatalf("unexpected batch fix updates: %#v", batchOutcome.StateUpdates)
	}

	rereview := &ReReviewPhase{}
	rereviewOutcome, err := rereview.Execute(context.Background(), testPhaseContext(nil, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "done"})))
	if err != nil {
		t.Fatalf("ReReviewPhase.Execute: %v", err)
	}
	if rereviewOutcome.StateUpdates["re_review.status"] != "passed" {
		t.Fatalf("unexpected rereview updates: %#v", rereviewOutcome.StateUpdates)
	}
}

func TestTDDPhasesAndHelpers(t *testing.T) {
	specPhase := &BehaviorSpecPhase{}
	specEmitter := newScriptedEmitter(
		interaction.ScriptedResponse{ActionID: "standard", Text: "Add returns a sum"},
		interaction.ScriptedResponse{ActionID: "empty", Text: "empty input returns zero"},
		interaction.ScriptedResponse{ActionID: "invalid_input", Text: "invalid types are rejected"},
	)
	specOutcome, err := specPhase.Execute(context.Background(), testPhaseContext(map[string]any{"function_target": "Add"}, specEmitter))
	if err != nil {
		t.Fatalf("BehaviorSpecPhase.Execute: %v", err)
	}
	if specOutcome.StateUpdates["specify.total_cases"].(int) != 3 {
		t.Fatalf("unexpected spec outcome: %#v", specOutcome.StateUpdates)
	}
	if got := loadOrCreateSpec(testPhaseContext(map[string]any{"function_target": "Add"}, nil)); got.FunctionTarget != "Add" {
		t.Fatalf("unexpected loaded spec: %#v", got)
	}
	spec := &BehaviorSpec{}
	accumulateBehavior(spec, "what about edge cases?", interaction.UserResponse{ActionID: "boundary", Text: "empty input"})
	if len(spec.EdgeCases) != 1 {
		t.Fatalf("expected edge case to be recorded: %#v", spec)
	}
	if !containsAny("Hello World", "world") || !containsLower(toLower("HELLO"), "hell") || !searchString("abc", "bc") {
		t.Fatal("expected helper predicates to match")
	}

	draftPhase := &TestDraftPhase{}
	draftOutcome, err := draftPhase.Execute(context.Background(), testPhaseContext(map[string]any{"specify.spec": BehaviorSpec{HappyPaths: []BehaviorCase{{Description: "works"}}}}, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "write"})))
	if err != nil {
		t.Fatalf("TestDraftPhase.Execute: %v", err)
	}
	if draftOutcome.StateUpdates["test_draft.response"] != "write" {
		t.Fatalf("unexpected draft updates: %#v", draftOutcome.StateUpdates)
	}
	if got := defaultTestDraft(testPhaseContext(nil, nil)); len(got.Items) != 1 {
		t.Fatalf("unexpected default test draft: %#v", got)
	}

	resultPhase := &TestResultPhase{}
	resultOutcome, err := resultPhase.Execute(context.Background(), testPhaseContext(map[string]any{"euclo.tdd.red_evidence": map[string]any{"summary": "failing"}}, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "add_tests"})))
	if err != nil {
		t.Fatalf("TestResultPhase.Execute: %v", err)
	}
	if resultOutcome.JumpTo != "specify" {
		t.Fatalf("unexpected result outcome: %#v", resultOutcome)
	}
	if got := tddRedResultFromState(map[string]any{}); got.Status != "all_red" {
		t.Fatalf("unexpected red result: %#v", got)
	}

	greenPhase := &GreenStatusPhase{}
	greenOutcome, err := greenPhase.Execute(context.Background(), testPhaseContext(map[string]any{"tdd.refactor_requested": false}, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "refactor"})))
	if err != nil {
		t.Fatalf("GreenStatusPhase.Execute: %v", err)
	}
	if greenOutcome.JumpTo != "implement" {
		t.Fatalf("unexpected green outcome: %#v", greenOutcome)
	}
	if got := tddGreenResultFromState(map[string]any{}); got.Status != "passed" {
		t.Fatalf("unexpected green result: %#v", got)
	}
	if got := buildGreenActions(map[string]any{"tdd.refactor_requested": true}, interaction.ResultContent{Status: "passed"}); len(got) != 2 {
		t.Fatalf("unexpected green actions: %#v", got)
	}
	if got := tddStateRecord(map[string]any{"euclo.tdd.green_evidence": map[string]any{"status": "pass"}}, "euclo.tdd.green_evidence"); len(got) == 0 {
		t.Fatal("expected tddStateRecord to return payload")
	}
	if got := verificationEvidenceItems(map[string]any{"checks": []any{map[string]any{"details": "x", "working_directory": "/tmp"}}}); len(got) != 1 {
		t.Fatalf("unexpected verification evidence items: %#v", got)
	}
	if redResultStatus("failed") != "all_red" || greenResultStatus("unknown") != "partial" || firstNonEmpty("", "  ", "x") != "x" {
		t.Fatal("unexpected tdd status helpers")
	}
}

type stubMemoryStore struct {
	recall     *memory.MemoryRecord
	remembered []map[string]interface{}
}

func (s *stubMemoryStore) Remember(_ context.Context, _ string, value map[string]interface{}, _ memory.MemoryScope) error {
	s.remembered = append(s.remembered, value)
	return nil
}

func (s *stubMemoryStore) Recall(_ context.Context, _ string, _ memory.MemoryScope) (*memory.MemoryRecord, bool, error) {
	if s.recall == nil {
		return nil, false, nil
	}
	return s.recall, true, nil
}

func (s *stubMemoryStore) Search(_ context.Context, _ string, _ memory.MemoryScope) ([]memory.MemoryRecord, error) {
	return nil, nil
}

func (s *stubMemoryStore) Forget(_ context.Context, _ string, _ memory.MemoryScope) error {
	return nil
}

func (s *stubMemoryStore) Summarize(_ context.Context, _ memory.MemoryScope) (string, error) {
	return "", nil
}
