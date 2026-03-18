package interaction

import (
	"context"
	"testing"
)

func TestConfirmationPhase_Confirm(t *testing.T) {
	emitter := &NoopEmitter{}
	phase := &ConfirmationPhase{
		BuildProposal: func(mc PhaseMachineContext) ProposalContent {
			return ProposalContent{
				Interpretation: "Add logging to handler",
				Scope:          []string{"handler.go"},
			}
		},
	}

	mc := PhaseMachineContext{
		Emitter:    emitter,
		State:      make(map[string]any),
		Artifacts:  NewArtifactBundle(),
		Mode:       "code",
		Phase:      "understand",
		PhaseIndex: 0,
		PhaseCount: 3,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !outcome.Advance {
		t.Error("expected advance on confirm")
	}
	if len(emitter.Frames) != 1 {
		t.Fatalf("frames: got %d, want 1", len(emitter.Frames))
	}
	if emitter.Frames[0].Kind != FrameProposal {
		t.Errorf("kind: got %q, want %q", emitter.Frames[0].Kind, FrameProposal)
	}
	if outcome.StateUpdates["understand.response"] != "confirm" {
		t.Errorf("response: got %v", outcome.StateUpdates["understand.response"])
	}
}

func TestQuestionPhase_SelectsFirst(t *testing.T) {
	emitter := &NoopEmitter{}
	phase := &QuestionPhase{
		BuildQuestion: func(mc PhaseMachineContext) QuestionContent {
			return QuestionContent{
				Question: "Which approach?",
				Options: []QuestionOption{
					{ID: "opt1", Label: "Option 1"},
					{ID: "opt2", Label: "Option 2"},
				},
				AllowFreetext: false,
			}
		},
	}

	mc := PhaseMachineContext{
		Emitter:    emitter,
		State:      make(map[string]any),
		Artifacts:  NewArtifactBundle(),
		Mode:       "planning",
		Phase:      "clarify",
		PhaseIndex: 1,
		PhaseCount: 5,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !outcome.Advance {
		t.Error("expected advance")
	}
	if len(emitter.Frames) != 1 {
		t.Fatalf("frames: got %d, want 1", len(emitter.Frames))
	}
	if emitter.Frames[0].Kind != FrameQuestion {
		t.Errorf("kind: got %q, want %q", emitter.Frames[0].Kind, FrameQuestion)
	}
	// NoopEmitter picks default (first option = opt1).
	if outcome.StateUpdates["clarify.response"] != "opt1" {
		t.Errorf("response: got %v", outcome.StateUpdates["clarify.response"])
	}
}

func TestQuestionPhase_WithFreetext(t *testing.T) {
	emitter := &NoopEmitter{}
	phase := &QuestionPhase{
		BuildQuestion: func(mc PhaseMachineContext) QuestionContent {
			return QuestionContent{
				Question:      "Any other concerns?",
				Options:       nil,
				AllowFreetext: true,
			}
		},
	}

	mc := PhaseMachineContext{
		Emitter:    emitter,
		State:      make(map[string]any),
		Artifacts:  NewArtifactBundle(),
		Mode:       "debug",
		Phase:      "intake",
		PhaseIndex: 0,
		PhaseCount: 4,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !outcome.Advance {
		t.Error("expected advance")
	}

	// With no options but freetext+skip, NoopEmitter picks first (freetext).
	frame := emitter.Frames[0]
	if len(frame.Actions) != 2 {
		t.Errorf("actions: got %d, want 2 (freetext + skip)", len(frame.Actions))
	}
}

func TestExecutionPhase_RunsAndEmits(t *testing.T) {
	emitter := &NoopEmitter{}
	ran := false
	phase := &ExecutionPhase{
		RunFunc: func(ctx context.Context, mc PhaseMachineContext) (ResultContent, error) {
			ran = true
			return ResultContent{
				Status: "passed",
				Evidence: []EvidenceItem{
					{Kind: "test_correlation", Detail: "all tests pass"},
				},
			}, nil
		},
	}

	mc := PhaseMachineContext{
		Emitter:    emitter,
		State:      make(map[string]any),
		Artifacts:  NewArtifactBundle(),
		Mode:       "code",
		Phase:      "execute",
		PhaseIndex: 2,
		PhaseCount: 5,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !ran {
		t.Error("RunFunc was not called")
	}
	if !outcome.Advance {
		t.Error("expected advance")
	}
	// Should emit status + result frames.
	if len(emitter.Frames) != 2 {
		t.Fatalf("frames: got %d, want 2", len(emitter.Frames))
	}
	if emitter.Frames[0].Kind != FrameStatus {
		t.Errorf("frame[0].kind: got %q, want %q", emitter.Frames[0].Kind, FrameStatus)
	}
	if emitter.Frames[1].Kind != FrameResult {
		t.Errorf("frame[1].kind: got %q, want %q", emitter.Frames[1].Kind, FrameResult)
	}
}

func TestSummaryPhase_Emits(t *testing.T) {
	emitter := &NoopEmitter{}
	phase := &SummaryPhase{
		BuildSummary: func(mc PhaseMachineContext) SummaryContent {
			return SummaryContent{
				Description: "All done",
				Artifacts:   []string{"plan", "edit_intent"},
				Changes:     []string{"main.go"},
			}
		},
	}

	mc := PhaseMachineContext{
		Emitter:    emitter,
		State:      make(map[string]any),
		Artifacts:  NewArtifactBundle(),
		Mode:       "code",
		Phase:      "present",
		PhaseIndex: 4,
		PhaseCount: 5,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !outcome.Advance {
		t.Error("expected advance")
	}
	if len(emitter.Frames) != 1 {
		t.Fatalf("frames: got %d, want 1", len(emitter.Frames))
	}
	if emitter.Frames[0].Kind != FrameSummary {
		t.Errorf("kind: got %q, want %q", emitter.Frames[0].Kind, FrameSummary)
	}
}
