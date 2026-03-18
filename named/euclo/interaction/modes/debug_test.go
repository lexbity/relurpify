package modes

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

func TestDebugMode_FullFlow(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	resolver := interaction.NewAgencyResolver()
	m := DebugMode(emitter, resolver)

	m.State()["instruction"] = "Fix the nil pointer panic"

	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Should have emitted frames for multiple phases.
	if len(emitter.Frames) == 0 {
		t.Fatal("expected frames to be emitted")
	}
}

func TestDebugMode_SkipIntake_HasEvidence(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	resolver := interaction.NewAgencyResolver()
	m := DebugMode(emitter, resolver)

	m.State()["instruction"] = "Fix panic at handler.go:42"
	m.State()["has_evidence"] = true

	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Intake should be skipped — no question frames for intake phase.
	for _, f := range emitter.Frames {
		if f.Kind == interaction.FrameQuestion && f.Phase == "intake" {
			t.Error("intake phase should have been skipped")
		}
	}
}

func TestDebugMode_SkipReproduce_KnownCause(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	resolver := interaction.NewAgencyResolver()
	m := DebugMode(emitter, resolver)

	m.State()["instruction"] = "Fix the bug"
	m.State()["known_cause"] = true

	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, f := range emitter.Frames {
		if f.Phase == "reproduce" && f.Kind == interaction.FrameResult {
			t.Error("reproduce phase should have been skipped")
		}
	}
}

func TestDiagnosticIntakePhase_DefaultQuestions(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &DiagnosticIntakePhase{}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "debug",
		Phase:      "intake",
		PhaseIndex: 0,
		PhaseCount: 6,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !outcome.Advance {
		t.Error("expected advance")
	}
	// Default has 3 questions.
	if len(emitter.Frames) != 3 {
		t.Errorf("frames: got %d, want 3", len(emitter.Frames))
	}
	for _, f := range emitter.Frames {
		if f.Kind != interaction.FrameQuestion {
			t.Errorf("kind: got %q, want question", f.Kind)
		}
	}
	symptoms, _ := outcome.StateUpdates["intake.symptoms"].([]map[string]any)
	if len(symptoms) != 3 {
		t.Errorf("symptoms: got %d, want 3", len(symptoms))
	}
}

func TestDiagnosticIntakePhase_CustomQuestions(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &DiagnosticIntakePhase{
		GenerateQuestions: func(_ context.Context, _ interaction.PhaseMachineContext) ([]interaction.QuestionContent, error) {
			return []interaction.QuestionContent{
				{Question: "What error?", Options: []interaction.QuestionOption{{ID: "a", Label: "A"}}},
			}, nil
		},
	}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "debug",
		Phase:      "intake",
		PhaseIndex: 0,
		PhaseCount: 6,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(emitter.Frames) != 1 {
		t.Errorf("frames: got %d, want 1", len(emitter.Frames))
	}
	count, _ := outcome.StateUpdates["intake.question_count"].(int)
	if count != 1 {
		t.Errorf("question_count: got %d, want 1", count)
	}
}

func TestReproductionPhase_Default(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &ReproductionPhase{}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "debug",
		Phase:      "reproduce",
		PhaseIndex: 1,
		PhaseCount: 6,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !outcome.Advance {
		t.Error("expected advance")
	}
	// Status + result.
	if len(emitter.Frames) != 2 {
		t.Errorf("frames: got %d, want 2", len(emitter.Frames))
	}
	// Default is "continue".
	if outcome.StateUpdates["reproduce.response"] != "continue" {
		t.Errorf("response: got %v", outcome.StateUpdates["reproduce.response"])
	}
}

func TestReproductionPhase_WrongError(t *testing.T) {
	emitter := &fixedResponseEmitter{
		response: interaction.UserResponse{ActionID: "wrong_error"},
	}
	phase := &ReproductionPhase{}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "debug",
		Phase:      "reproduce",
		PhaseIndex: 1,
		PhaseCount: 6,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if outcome.JumpTo != "intake" {
		t.Errorf("jump_to: got %q, want 'intake'", outcome.JumpTo)
	}
}

func TestReproductionPhase_KnownCause(t *testing.T) {
	emitter := &fixedResponseEmitter{
		response: interaction.UserResponse{ActionID: "skip", Text: "null pointer in handler"},
	}
	phase := &ReproductionPhase{}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "debug",
		Phase:      "reproduce",
		PhaseIndex: 1,
		PhaseCount: 6,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if outcome.StateUpdates["reproduce.known_cause"] != "null pointer in handler" {
		t.Errorf("known_cause: got %v", outcome.StateUpdates["reproduce.known_cause"])
	}
	if outcome.StateUpdates["known_cause"] != true {
		t.Error("expected known_cause flag")
	}
}

func TestLocalizationPhase_Default(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &LocalizationPhase{}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "debug",
		Phase:      "localize",
		PhaseIndex: 2,
		PhaseCount: 6,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !outcome.Advance {
		t.Error("expected advance")
	}
	// Status + result.
	if len(emitter.Frames) != 2 {
		t.Errorf("frames: got %d, want 2", len(emitter.Frames))
	}
	// Default is "fix".
	if outcome.StateUpdates["localize.response"] != "fix" {
		t.Errorf("response: got %v", outcome.StateUpdates["localize.response"])
	}
}

func TestLocalizationPhase_Deeper(t *testing.T) {
	emitter := &fixedResponseEmitter{
		response: interaction.UserResponse{ActionID: "deeper"},
	}
	phase := &LocalizationPhase{}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "debug",
		Phase:      "localize",
		PhaseIndex: 2,
		PhaseCount: 6,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if outcome.JumpTo != "localize" {
		t.Errorf("jump_to: got %q, want 'localize'", outcome.JumpTo)
	}
}

func TestLocalizationPhase_Override(t *testing.T) {
	emitter := &fixedResponseEmitter{
		response: interaction.UserResponse{ActionID: "override", Text: "pkg/auth/token.go:55"},
	}
	phase := &LocalizationPhase{}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "debug",
		Phase:      "localize",
		PhaseIndex: 2,
		PhaseCount: 6,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if outcome.StateUpdates["localize.override_location"] != "pkg/auth/token.go:55" {
		t.Errorf("override: got %v", outcome.StateUpdates["localize.override_location"])
	}
}

func TestDebugFixProposalPhase_Default(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &DebugFixProposalPhase{}

	mc := interaction.PhaseMachineContext{
		Emitter: emitter,
		State: map[string]any{
			"localize.result": interaction.ResultContent{
				Status: "localized",
				Evidence: []interaction.EvidenceItem{
					{Kind: "code_read", Detail: "nil check missing", Location: "handler.go:42"},
				},
			},
		},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "debug",
		Phase:      "propose_fix",
		PhaseIndex: 3,
		PhaseCount: 6,
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
	if emitter.Frames[0].Kind != interaction.FrameDraft {
		t.Errorf("kind: got %q, want draft", emitter.Frames[0].Kind)
	}
}

func TestDebugFixProposalPhase_Reject(t *testing.T) {
	emitter := &fixedResponseEmitter{
		response: interaction.UserResponse{ActionID: "reject"},
	}
	phase := &DebugFixProposalPhase{}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "debug",
		Phase:      "propose_fix",
		PhaseIndex: 3,
		PhaseCount: 6,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if outcome.JumpTo != "localize" {
		t.Errorf("jump_to: got %q, want 'localize'", outcome.JumpTo)
	}
}

func TestRegisterDebugTriggers(t *testing.T) {
	resolver := interaction.NewAgencyResolver()
	RegisterDebugTriggers(resolver)

	triggers := resolver.TriggersForMode("debug")
	if len(triggers) != 4 {
		t.Errorf("triggers: got %d, want 4", len(triggers))
	}

	trigger, ok := resolver.Resolve("debug", "investigate regression")
	if !ok {
		t.Fatal("expected regression trigger")
	}
	if trigger.CapabilityID != "euclo:debug.investigate_regression" {
		t.Errorf("capability: got %q", trigger.CapabilityID)
	}

	trigger, ok = resolver.Resolve("debug", "show trace")
	if !ok {
		t.Fatal("expected trace trigger")
	}
	if trigger.CapabilityID != "euclo:trace.analyze" {
		t.Errorf("trace capability: got %q", trigger.CapabilityID)
	}
}

func TestRegisterDebugTriggers_NilResolver(t *testing.T) {
	RegisterDebugTriggers(nil)
}

func TestDebugPhaseLabels(t *testing.T) {
	labels := DebugPhaseLabels()
	if len(labels) != 6 {
		t.Fatalf("labels: got %d, want 6", len(labels))
	}
}

func TestSkipIntake(t *testing.T) {
	tests := []struct {
		name  string
		state map[string]any
		want  bool
	}{
		{"has_evidence", map[string]any{"has_evidence": true}, true},
		{"evidence_in_instruction", map[string]any{
			"requires_evidence_before_mutation": true,
			"evidence_in_instruction":           true,
		}, true},
		{"no_evidence", map[string]any{}, false},
		{"requires_but_no_instruction_evidence", map[string]any{
			"requires_evidence_before_mutation": true,
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := skipIntake(tt.state, interaction.NewArtifactBundle())
			if got != tt.want {
				t.Errorf("skipIntake: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSkipReproduce(t *testing.T) {
	tests := []struct {
		name  string
		state map[string]any
		want  bool
	}{
		{"skip_reproduction", map[string]any{"skip_reproduction": true}, true},
		{"known_cause", map[string]any{"known_cause": true}, true},
		{"normal", map[string]any{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := skipReproduce(tt.state, interaction.NewArtifactBundle())
			if got != tt.want {
				t.Errorf("skipReproduce: got %v, want %v", got, tt.want)
			}
		})
	}
}
