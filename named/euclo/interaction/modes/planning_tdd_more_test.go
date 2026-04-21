package modes

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

type sequenceEmitter struct {
	frames    []interaction.InteractionFrame
	responses []interaction.UserResponse
}

func (e *sequenceEmitter) Emit(_ context.Context, frame interaction.InteractionFrame) error {
	e.frames = append(e.frames, frame)
	return nil
}

func (e *sequenceEmitter) AwaitResponse(context.Context) (interaction.UserResponse, error) {
	if len(e.responses) == 0 {
		return interaction.UserResponse{}, nil
	}
	resp := e.responses[0]
	e.responses = e.responses[1:]
	return resp, nil
}

func TestClarifyPhaseAndQuestionActions(t *testing.T) {
	emitter := &sequenceEmitter{
		responses: []interaction.UserResponse{
			{ActionID: "yes", Text: "first"},
			{ActionID: "skip"},
		},
	}
	phase := &ClarifyPhase{
		GenerateQuestions: func(context.Context, interaction.PhaseMachineContext) ([]interaction.QuestionContent, error) {
			return []interaction.QuestionContent{
				{Question: "Q1", Options: []interaction.QuestionOption{{ID: "yes", Label: "Yes"}}},
				{Question: "Q2", AllowFreetext: true},
				{Question: "Q3"},
				{Question: "Q4"},
			}, nil
		},
	}
	outcome, err := phase.Execute(context.Background(), interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Mode:       "planning",
		Phase:      "clarify",
		PhaseIndex: 1,
		PhaseCount: 6,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if got := outcome.StateUpdates["clarify.question_count"]; got != 3 {
		t.Fatalf("expected capped question count, got %#v", got)
	}
	if len(emitter.frames) != 2 {
		t.Fatalf("expected two emitted question frames, got %d", len(emitter.frames))
	}
	actions := buildQuestionActions(interaction.QuestionContent{}, 0)
	if len(actions) == 0 || !actions[len(actions)-1].Default {
		t.Fatalf("expected skip to become default for empty question, got %#v", actions)
	}
}

func TestComparePhaseAndDefaultComparison(t *testing.T) {
	emitter := &sequenceEmitter{responses: []interaction.UserResponse{{ActionID: "recommended"}}}
	phase := &ComparePhase{}
	outcome, err := phase.Execute(context.Background(), interaction.PhaseMachineContext{
		Emitter: emitter,
		State: map[string]any{
			"generate.candidates": []interaction.Candidate{
				{ID: "cand-1", Summary: "one", Properties: map[string]string{"a": "1", "b": ""}},
				{ID: "cand-2", Summary: "two", Properties: map[string]string{"b": "2"}},
			},
		},
		Mode:  "planning",
		Phase: "compare",
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if got := outcome.StateUpdates["generate.selected"]; got != "cand-1" {
		t.Fatalf("expected recommended candidate selection, got %#v", got)
	}
	comparison := defaultComparison([]interaction.Candidate{
		{ID: "cand-1", Properties: map[string]string{"a": "1"}},
		{ID: "cand-2", Properties: map[string]string{"b": "2"}},
	})
	if len(comparison.Dimensions) != 2 || len(comparison.Matrix) != 2 {
		t.Fatalf("unexpected comparison content: %#v", comparison)
	}
}

func TestRefinePhaseAndDefaultDraft(t *testing.T) {
	emitter := &sequenceEmitter{responses: []interaction.UserResponse{{ActionID: "commit", Text: "edit"}}}
	phase := &RefinePhase{}
	outcome, err := phase.Execute(context.Background(), interaction.PhaseMachineContext{
		Emitter: emitter,
		State: map[string]any{
			"generate.selected_summary": "selected plan",
		},
		Mode:       "planning",
		Phase:      "refine",
		PhaseIndex: 4,
		PhaseCount: 6,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if got := outcome.StateUpdates["refine.edit_text"]; got != "edit" {
		t.Fatalf("expected edit text to be recorded, got %#v", got)
	}
	draft := defaultDraft(interaction.PhaseMachineContext{State: map[string]any{}})
	if draft.Items[0].Content == "" || !draft.Addable {
		t.Fatalf("unexpected default draft: %#v", draft)
	}
}

func TestBehaviorSpecAndTDDHelpers(t *testing.T) {
	spec := BehaviorSpec{FunctionTarget: "pkg.Func"}
	accumulateBehavior(&spec, "What error conditions should be tested?", interaction.UserResponse{ActionID: "invalid", Text: "bad input"})
	accumulateBehavior(&spec, "Any edge cases?", interaction.UserResponse{ActionID: "boundary", Text: "empty data"})
	accumulateBehavior(&spec, "What is the happy path?", interaction.UserResponse{ActionID: "ok", Text: "normal data"})
	if len(spec.ErrorCases) != 1 || len(spec.EdgeCases) != 1 || len(spec.HappyPaths) != 1 {
		t.Fatalf("unexpected spec accumulation: %#v", spec)
	}
	if !containsAny("Compile plan now", "compile plan") || !containsLower("abc", "b") || !searchString("abcdef", "cd") {
		t.Fatal("expected string matching helpers to succeed")
	}

	emitter := &sequenceEmitter{responses: []interaction.UserResponse{{ActionID: "matrix"}, {ActionID: "skip"}}}
	phase := &BehaviorSpecPhase{}
	outcome, err := phase.Execute(context.Background(), interaction.PhaseMachineContext{
		Emitter: emitter,
		State: map[string]any{
			"function_target": "pkg.Func",
		},
		Mode:       "tdd",
		Phase:      "specify",
		PhaseIndex: 0,
		PhaseCount: 4,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if got := outcome.StateUpdates["specify.question_count"]; got != len(defaultBehaviorQuestions()) {
		t.Fatalf("unexpected question count: %#v", got)
	}
	if len(emitter.frames) < 3 {
		t.Fatalf("expected matrix emission plus question frames, got %d frames", len(emitter.frames))
	}

	draft := defaultTestDraft(interaction.PhaseMachineContext{State: map[string]any{"specify.spec": spec}})
	if len(draft.Items) != 3 || !draft.Addable {
		t.Fatalf("unexpected test draft: %#v", draft)
	}

	red := tddRedResultFromState(map[string]any{"euclo.tdd.red_evidence": map[string]any{"status": "fail", "checks": []any{map[string]any{"name": "go test", "status": "pass"}}}})
	if red.Status != "all_red" || len(red.Evidence) != 1 {
		t.Fatalf("unexpected red result: %#v", red)
	}
	green := tddGreenResultFromState(map[string]any{"euclo.tdd.green_evidence": map[string]any{"status": "pass", "summary": "ok"}})
	if green.Status != "passed" {
		t.Fatalf("unexpected green result: %#v", green)
	}
	if got := verificationEvidenceItems(map[string]any{"checks": []map[string]any{{"details": "d", "working_directory": "/w"}}}); len(got) != 1 || got[0].Detail != "d" {
		t.Fatalf("unexpected verification evidence items: %#v", got)
	}
	if redResultStatus("failed") != "all_red" || greenResultStatus("failed") != "failed" || firstNonEmpty("", "  x  ") != "x" {
		t.Fatal("unexpected status helpers")
	}
}
