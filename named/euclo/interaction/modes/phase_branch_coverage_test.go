package modes

import (
	"context"
	"errors"
	"testing"

	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

func TestPlanningBranchMatrix(t *testing.T) {
	gen := &GeneratePhase{
		GenerateCandidates: func(context.Context, interaction.PhaseMachineContext) ([]interaction.Candidate, error) {
			return nil, errors.New("boom")
		},
	}
	if _, err := gen.Execute(context.Background(), testPhaseContext(nil, newScriptedEmitter())); err == nil {
		t.Fatal("expected GeneratePhase callback error")
	}

	gen = &GeneratePhase{
		GenerateCandidates: func(context.Context, interaction.PhaseMachineContext) ([]interaction.Candidate, error) {
			return []interaction.Candidate{{ID: "a", Summary: "A"}, {ID: "b", Summary: "B"}}, nil
		},
	}
	moreOutcome, err := gen.Execute(context.Background(), testPhaseContext(nil, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "more"})))
	if err != nil {
		t.Fatalf("GeneratePhase more: %v", err)
	}
	if moreOutcome.JumpTo != "generate" {
		t.Fatalf("expected generate jump, got %#v", moreOutcome)
	}
	mergeOutcome, err := gen.Execute(context.Background(), testPhaseContext(nil, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "merge", Text: "combine"})))
	if err != nil {
		t.Fatalf("GeneratePhase merge: %v", err)
	}
	if mergeOutcome.StateUpdates["generate.selected"] != "a" || mergeOutcome.StateUpdates["generate.merge_request"] != "combine" {
		t.Fatalf("unexpected merge updates: %#v", mergeOutcome.StateUpdates)
	}

	scope := &ScopePhase{
		AnalyzeScope: func(context.Context, interaction.PhaseMachineContext) (interaction.ProposalContent, error) {
			return interaction.ProposalContent{}, errors.New("scope failed")
		},
	}
	if _, err := scope.Execute(context.Background(), testPhaseContext(nil, newScriptedEmitter())); err == nil {
		t.Fatal("expected scope error")
	}
	scope = &ScopePhase{
		AnalyzeScope: func(context.Context, interaction.PhaseMachineContext) (interaction.ProposalContent, error) {
			return interaction.ProposalContent{Interpretation: "scope"}, nil
		},
	}
	scopeOutcome, err := scope.Execute(context.Background(), testPhaseContext(nil, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "broaden", Text: "expand"})))
	if err != nil {
		t.Fatalf("ScopePhase broaden: %v", err)
	}
	if scopeOutcome.StateUpdates["scope.correction"] != "expand" {
		t.Fatalf("unexpected scope correction: %#v", scopeOutcome.StateUpdates)
	}

	clarify := &ClarifyPhase{
		GenerateQuestions: func(context.Context, interaction.PhaseMachineContext) ([]interaction.QuestionContent, error) {
			return []interaction.QuestionContent{{Question: "Q1", Options: []interaction.QuestionOption{{ID: "opt1", Label: "One"}}}, {Question: "Q2"}, {Question: "Q3"}, {Question: "Q4"}}, nil
		},
	}
	clarifyOutcome, err := clarify.Execute(context.Background(), testPhaseContext(nil, newScriptedEmitter(
		interaction.ScriptedResponse{ActionID: "opt1"},
		interaction.ScriptedResponse{ActionID: "skip"},
	)))
	if err != nil {
		t.Fatalf("ClarifyPhase: %v", err)
	}
	if clarifyOutcome.StateUpdates["clarify.question_count"] != 3 {
		t.Fatalf("expected clarify truncation, got %#v", clarifyOutcome.StateUpdates)
	}
	if answers, ok := clarifyOutcome.StateUpdates["clarify.answers"].([]map[string]any); !ok || len(answers) != 1 {
		t.Fatalf("expected one answer before skip, got %#v", clarifyOutcome.StateUpdates["clarify.answers"])
	}

	compare := &ComparePhase{}
	if outcome, err := compare.Execute(context.Background(), testPhaseContext(nil, newScriptedEmitter())); err != nil || outcome.Advance != true || len(outcome.StateUpdates) != 0 {
		t.Fatalf("expected empty compare to short-circuit, got %#v err=%v", outcome, err)
	}
	compareOutcome, err := compare.Execute(context.Background(), testPhaseContext(map[string]any{"generate.candidates": []interaction.Candidate{{ID: "x", Summary: "X", Properties: map[string]string{"mode": "fast"}}}}, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "recommended"})))
	if err != nil {
		t.Fatalf("ComparePhase recommended: %v", err)
	}
	if compareOutcome.StateUpdates["generate.selected"] != "x" {
		t.Fatalf("unexpected compare update: %#v", compareOutcome.StateUpdates)
	}

	refine := &RefinePhase{}
	refineOutcome, err := refine.Execute(context.Background(), testPhaseContext(nil, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "commit", Text: "tweak"})))
	if err != nil {
		t.Fatalf("RefinePhase: %v", err)
	}
	if refineOutcome.StateUpdates["refine.edit_text"] != "tweak" {
		t.Fatalf("expected refine edit text, got %#v", refineOutcome.StateUpdates)
	}
}

func TestCodeBranchMatrix(t *testing.T) {
	intent := &IntentPhase{
		AnalyzeIntent: func(context.Context, interaction.PhaseMachineContext) (IntentAnalysis, error) {
			return IntentAnalysis{}, errors.New("intent failed")
		},
	}
	if _, err := intent.Execute(context.Background(), testPhaseContext(nil, newScriptedEmitter())); err == nil {
		t.Fatal("expected intent error")
	}
	intent = &IntentPhase{
		AnalyzeIntent: func(context.Context, interaction.PhaseMachineContext) (IntentAnalysis, error) {
			return IntentAnalysis{Interpretation: "intent", MutationFlag: true}, nil
		},
	}
	intentOutcome, err := intent.Execute(context.Background(), testPhaseContext(nil, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "plan_first"})))
	if err != nil {
		t.Fatalf("IntentPhase plan_first: %v", err)
	}
	if intentOutcome.Transition != "planning" {
		t.Fatalf("unexpected intent transition: %#v", intentOutcome)
	}

	propose := &EditProposalPhase{
		BuildProposal: func(context.Context, interaction.PhaseMachineContext) (interaction.DraftContent, error) {
			return interaction.DraftContent{}, errors.New("proposal failed")
		},
	}
	if _, err := propose.Execute(context.Background(), testPhaseContext(nil, newScriptedEmitter())); err == nil {
		t.Fatal("expected proposal error")
	}
	propose = &EditProposalPhase{
		BuildProposal: func(context.Context, interaction.PhaseMachineContext) (interaction.DraftContent, error) {
			return interaction.DraftContent{Items: []interaction.DraftItem{{ID: "d1", Editable: true}}}, nil
		},
	}
	proposeOutcome, err := propose.Execute(context.Background(), testPhaseContext(nil, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "skip"})))
	if err != nil {
		t.Fatalf("EditProposalPhase skip: %v", err)
	}
	if proposeOutcome.JumpTo != "verify" {
		t.Fatalf("unexpected proposal jump: %#v", proposeOutcome)
	}

	verify := &VerificationPhase{}
	verifyOutcome, err := verify.Execute(context.Background(), testPhaseContext(map[string]any{"verify.failure_count": 1}, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "debug"})))
	if err != nil {
		t.Fatalf("VerificationPhase debug: %v", err)
	}
	if verifyOutcome.Transition != "debug" || verifyOutcome.StateUpdates["verify.escalated"] != true {
		t.Fatalf("unexpected verify outcome: %#v", verifyOutcome)
	}
	verifyOutcome, err = verify.Execute(context.Background(), testPhaseContext(map[string]any{"verify.failure_count": VerifyFailureThreshold}, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "different_approach"})))
	if err != nil {
		t.Fatalf("VerificationPhase different approach: %v", err)
	}
	if verifyOutcome.JumpTo != "execute" {
		t.Fatalf("unexpected verify jump: %#v", verifyOutcome)
	}
}

func TestReviewDebugTDDBranchMatrix(t *testing.T) {
	reviewScope := &ReviewScopePhase{
		AnalyzeScope: func(context.Context, interaction.PhaseMachineContext) (interaction.ProposalContent, error) {
			return interaction.ProposalContent{}, errors.New("scope failed")
		},
	}
	if _, err := reviewScope.Execute(context.Background(), testPhaseContext(nil, newScriptedEmitter())); err == nil {
		t.Fatal("expected review scope error")
	}
	reviewScope = &ReviewScopePhase{
		AnalyzeScope: func(context.Context, interaction.PhaseMachineContext) (interaction.ProposalContent, error) {
			return interaction.ProposalContent{Interpretation: "review"}, nil
		},
	}
	reviewScopeOutcome, err := reviewScope.Execute(context.Background(), testPhaseContext(nil, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "narrow", Text: "file.go"})))
	if err != nil {
		t.Fatalf("ReviewScopePhase narrow: %v", err)
	}
	if reviewScopeOutcome.StateUpdates["scope.narrow_file"] != "file.go" {
		t.Fatalf("unexpected narrow update: %#v", reviewScopeOutcome.StateUpdates)
	}

	sweep := &ReviewSweepPhase{}
	sweepOutcome, err := sweep.Execute(context.Background(), testPhaseContext(map[string]any{"euclo.review_findings": map[string]any{"findings": []any{map[string]any{"severity": "warning", "description": "x"}}}}, newScriptedEmitter()))
	if err != nil {
		t.Fatalf("ReviewSweepPhase: %v", err)
	}
	if _, ok := sweepOutcome.StateUpdates["sweep.findings"].(interaction.FindingsContent); !ok {
		t.Fatalf("unexpected sweep result: %#v", sweepOutcome.StateUpdates)
	}

	triage := &TriagePhase{}
	noFindings, err := triage.Execute(context.Background(), testPhaseContext(map[string]any{"sweep.approval_status": "approved"}, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "done"})))
	if err != nil {
		t.Fatalf("TriagePhase no findings: %v", err)
	}
	if noFindings.StateUpdates["triage.no_fixes"] != true {
		t.Fatalf("expected no findings path: %#v", noFindings.StateUpdates)
	}
	withFindings, err := triage.Execute(context.Background(), testPhaseContext(map[string]any{"sweep.findings": interaction.FindingsContent{Critical: []interaction.Finding{{Description: "c"}}}}, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "pick"})))
	if err != nil {
		t.Fatalf("TriagePhase pick: %v", err)
	}
	if withFindings.StateUpdates["triage.fix_scope"] != "selected" {
		t.Fatalf("unexpected triage pick: %#v", withFindings.StateUpdates)
	}

	batch := &BatchFixPhase{}
	batchOutcome, err := batch.Execute(context.Background(), testPhaseContext(map[string]any{"triage.fix_scope": "all"}, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "accept"})))
	if err != nil {
		t.Fatalf("BatchFixPhase: %v", err)
	}
	if batchOutcome.StateUpdates["act.skip_re_review"] != true {
		t.Fatalf("expected accept branch: %#v", batchOutcome.StateUpdates)
	}

	rereview := &ReReviewPhase{}
	rereviewOutcome, err := rereview.Execute(context.Background(), testPhaseContext(nil, newScriptedEmitter()))
	if err != nil {
		t.Fatalf("ReReviewPhase: %v", err)
	}
	if rereviewOutcome.StateUpdates["re_review.status"] != "passed" {
		t.Fatalf("unexpected rereview status: %#v", rereviewOutcome.StateUpdates)
	}

	intake := &DiagnosticIntakePhase{}
	intakeOutcome, err := intake.Execute(context.Background(), testPhaseContext(nil, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "skip"})))
	if err != nil {
		t.Fatalf("DiagnosticIntakePhase: %v", err)
	}
	if intakeOutcome.StateUpdates["intake.question_count"] != 3 {
		t.Fatalf("unexpected intake count: %#v", intakeOutcome.StateUpdates)
	}

	repro := &ReproductionPhase{}
	reproOutcome, err := repro.Execute(context.Background(), testPhaseContext(nil, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "skip", Text: "root cause known"})))
	if err != nil {
		t.Fatalf("ReproductionPhase: %v", err)
	}
	if reproOutcome.StateUpdates["known_cause"] != true {
		t.Fatalf("unexpected reproduction known cause: %#v", reproOutcome.StateUpdates)
	}

	fix := &DebugFixProposalPhase{}
	fixOutcome, err := fix.Execute(context.Background(), testPhaseContext(map[string]any{"localize.result": interaction.ResultContent{Evidence: []interaction.EvidenceItem{{Location: "file.go:10"}}}}, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "reject"})))
	if err != nil {
		t.Fatalf("DebugFixProposalPhase: %v", err)
	}
	if fixOutcome.JumpTo != "localize" {
		t.Fatalf("unexpected fix jump: %#v", fixOutcome)
	}

	localize := &LocalizationPhase{}
	localizeOutcome, err := localize.Execute(context.Background(), testPhaseContext(nil, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "override", Text: "pkg/file.go"})))
	if err != nil {
		t.Fatalf("LocalizationPhase: %v", err)
	}
	if localizeOutcome.StateUpdates["localize.override_location"] != "pkg/file.go" {
		t.Fatalf("unexpected localization override: %#v", localizeOutcome.StateUpdates)
	}

	spec := &BehaviorSpecPhase{}
	specOutcome, err := spec.Execute(context.Background(), testPhaseContext(map[string]any{"function_target": "X"}, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "matrix"})))
	if err != nil {
		t.Fatalf("BehaviorSpecPhase: %v", err)
	}
	if specOutcome.StateUpdates["specify.question_count"] != 3 {
		t.Fatalf("unexpected spec count: %#v", specOutcome.StateUpdates)
	}

	draft := &TestDraftPhase{}
	draftOutcome, err := draft.Execute(context.Background(), testPhaseContext(map[string]any{"specify.spec": BehaviorSpec{HappyPaths: []BehaviorCase{{Description: "works"}}}}, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "write", Text: "handwritten"})))
	if err != nil {
		t.Fatalf("TestDraftPhase: %v", err)
	}
	if draftOutcome.StateUpdates["test_draft.edit_text"] != "handwritten" {
		t.Fatalf("unexpected draft edit text: %#v", draftOutcome.StateUpdates)
	}

	resultPhase := &TestResultPhase{}
	resultOutcome, err := resultPhase.Execute(context.Background(), testPhaseContext(map[string]any{"euclo.tdd.red_evidence": map[string]any{"summary": "failing"}}, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "add_tests"})))
	if err != nil {
		t.Fatalf("TestResultPhase: %v", err)
	}
	if resultOutcome.JumpTo != "specify" {
		t.Fatalf("unexpected result jump: %#v", resultOutcome)
	}

	green := &GreenStatusPhase{}
	greenOutcome, err := green.Execute(context.Background(), testPhaseContext(map[string]any{"tdd.refactor_requested": false}, newScriptedEmitter(interaction.ScriptedResponse{ActionID: "refactor"})))
	if err != nil {
		t.Fatalf("GreenStatusPhase: %v", err)
	}
	if greenOutcome.JumpTo != "implement" {
		t.Fatalf("unexpected green jump: %#v", greenOutcome)
	}
}
