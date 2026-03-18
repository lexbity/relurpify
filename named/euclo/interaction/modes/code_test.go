package modes

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

func TestCodeMode_FullFlow(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	resolver := interaction.NewAgencyResolver()
	m := CodeMode(emitter, resolver)

	m.State()["instruction"] = "Add error handling"
	m.State()["scope"] = []string{"handler.go"}

	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Should have emitted frames for understand, execute (status+result), verify (status+result), present.
	if len(emitter.Frames) == 0 {
		t.Fatal("expected frames to be emitted")
	}

	// Present phase default is "accept" so no transition.
	if _, ok := m.State()["transition.accepted"]; ok {
		t.Error("no transition expected on accept")
	}
}

func TestCodeMode_SkipPropose_SmallChange(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	resolver := interaction.NewAgencyResolver()
	m := CodeMode(emitter, resolver)

	m.State()["instruction"] = "Fix typo"
	m.State()["small_change"] = true

	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Propose phase should be skipped — no draft frame emitted with phase "propose".
	for _, f := range emitter.Frames {
		if f.Kind == interaction.FrameDraft && f.Phase == "propose" {
			t.Error("propose phase should have been skipped for small change")
		}
	}
}

func TestCodeMode_SkipPropose_JustDoIt(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	resolver := interaction.NewAgencyResolver()
	m := CodeMode(emitter, resolver)

	m.State()["instruction"] = "Quick fix"
	m.State()["just_do_it"] = true

	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, f := range emitter.Frames {
		if f.Kind == interaction.FrameDraft && f.Phase == "propose" {
			t.Error("propose phase should have been skipped with just_do_it")
		}
	}
}

func TestIntentPhase_DefaultAnalysis(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &IntentPhase{}

	mc := interaction.PhaseMachineContext{
		Emitter: emitter,
		State: map[string]any{
			"instruction": "Add retry logic",
			"scope":       []string{"client.go"},
		},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "code",
		Phase:      "understand",
		PhaseIndex: 0,
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
	if emitter.Frames[0].Kind != interaction.FrameProposal {
		t.Errorf("kind: got %q, want proposal", emitter.Frames[0].Kind)
	}
	// NoopEmitter picks default = confirm.
	if outcome.StateUpdates["understand.response"] != "confirm" {
		t.Errorf("response: got %v", outcome.StateUpdates["understand.response"])
	}
	// Should flag mutation.
	if outcome.StateUpdates["understand.mutation"] != true {
		t.Error("expected mutation flag")
	}
}

func TestIntentPhase_CustomAnalyzer(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	called := false
	phase := &IntentPhase{
		AnalyzeIntent: func(_ context.Context, _ interaction.PhaseMachineContext) (IntentAnalysis, error) {
			called = true
			return IntentAnalysis{
				Interpretation: "Custom intent",
				SmallChange:    true,
				MutationFlag:   false,
			}, nil
		},
	}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "code",
		Phase:      "understand",
		PhaseIndex: 0,
		PhaseCount: 5,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !called {
		t.Error("custom analyzer should have been called")
	}
	if outcome.StateUpdates["understand.small_change"] != true {
		t.Error("expected small_change from custom analyzer")
	}
}

func TestIntentPhase_PlanFirst(t *testing.T) {
	// Use a custom emitter that returns "plan_first" response.
	emitter := &fixedResponseEmitter{
		response: interaction.UserResponse{ActionID: "plan_first"},
	}
	phase := &IntentPhase{}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{"instruction": "Big change"},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "code",
		Phase:      "understand",
		PhaseIndex: 0,
		PhaseCount: 5,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if outcome.Transition != "planning" {
		t.Errorf("transition: got %q, want 'planning'", outcome.Transition)
	}
}

func TestEditProposalPhase_DefaultProposal(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &EditProposalPhase{}

	mc := interaction.PhaseMachineContext{
		Emitter: emitter,
		State: map[string]any{
			"scope": []string{"handler.go", "middleware.go"},
		},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "code",
		Phase:      "propose",
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
	if emitter.Frames[0].Kind != interaction.FrameDraft {
		t.Errorf("kind: got %q, want draft", emitter.Frames[0].Kind)
	}
}

func TestEditProposalPhase_SkipToVerify(t *testing.T) {
	emitter := &fixedResponseEmitter{
		response: interaction.UserResponse{ActionID: "skip"},
	}
	phase := &EditProposalPhase{}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "code",
		Phase:      "propose",
		PhaseIndex: 1,
		PhaseCount: 5,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if outcome.JumpTo != "verify" {
		t.Errorf("jump_to: got %q, want 'verify'", outcome.JumpTo)
	}
}

func TestCodeExecutionPhase_Default(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &CodeExecutionPhase{}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "code",
		Phase:      "execute",
		PhaseIndex: 2,
		PhaseCount: 5,
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
}

func TestCodeExecutionPhase_WithCapability(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	ran := false
	phase := &CodeExecutionPhase{
		RunCapability: func(_ context.Context, _ interaction.PhaseMachineContext) (interaction.ResultContent, []euclotypes.Artifact, error) {
			ran = true
			return interaction.ResultContent{Status: "completed"}, []euclotypes.Artifact{
				{ID: "edit-1", Kind: euclotypes.ArtifactKindEditIntent, Summary: "edit intent"},
			}, nil
		},
	}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
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
		t.Error("capability should have been called")
	}
	if len(outcome.Artifacts) != 1 {
		t.Errorf("artifacts: got %d, want 1", len(outcome.Artifacts))
	}
}

func TestVerificationPhase_Passed(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &VerificationPhase{}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "code",
		Phase:      "verify",
		PhaseIndex: 3,
		PhaseCount: 5,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !outcome.Advance {
		t.Error("expected advance")
	}
	// Default result is "passed", NoopEmitter picks "done".
	if outcome.StateUpdates["verify.response"] != "done" {
		t.Errorf("response: got %v", outcome.StateUpdates["verify.response"])
	}
	failCount, _ := outcome.StateUpdates["verify.failure_count"].(int)
	if failCount != 0 {
		t.Errorf("failure_count: got %d, want 0", failCount)
	}
}

func TestVerificationPhase_Failed_FirstTime(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &VerificationPhase{
		RunVerification: func(_ context.Context, _ interaction.PhaseMachineContext) (interaction.ResultContent, error) {
			return interaction.ResultContent{
				Status: "failed",
				Gaps:   []string{"test failure"},
			}, nil
		},
	}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "code",
		Phase:      "verify",
		PhaseIndex: 3,
		PhaseCount: 5,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	failCount, _ := outcome.StateUpdates["verify.failure_count"].(int)
	if failCount != 1 {
		t.Errorf("failure_count: got %d, want 1", failCount)
	}
	// Default on first failure is "fix_gaps".
	if outcome.StateUpdates["verify.response"] != "fix_gaps" {
		t.Errorf("response: got %v", outcome.StateUpdates["verify.response"])
	}
	// Should jump back to execute.
	if outcome.JumpTo != "execute" {
		t.Errorf("jump_to: got %q, want 'execute'", outcome.JumpTo)
	}
}

func TestVerificationPhase_Failed_Escalation(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &VerificationPhase{
		RunVerification: func(_ context.Context, _ interaction.PhaseMachineContext) (interaction.ResultContent, error) {
			return interaction.ResultContent{Status: "failed"}, nil
		},
	}

	mc := interaction.PhaseMachineContext{
		Emitter: emitter,
		State: map[string]any{
			"verify.failure_count": 1, // Already 1 failure.
		},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "code",
		Phase:      "verify",
		PhaseIndex: 3,
		PhaseCount: 5,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	failCount, _ := outcome.StateUpdates["verify.failure_count"].(int)
	if failCount != 2 {
		t.Errorf("failure_count: got %d, want 2", failCount)
	}
	// At threshold, default becomes "debug" — NoopEmitter picks default.
	if outcome.StateUpdates["verify.response"] != "debug" {
		t.Errorf("response: got %v, want 'debug'", outcome.StateUpdates["verify.response"])
	}
	if outcome.Transition != "debug" {
		t.Errorf("transition: got %q, want 'debug'", outcome.Transition)
	}
}

func TestVerificationPhase_ReVerify(t *testing.T) {
	emitter := &fixedResponseEmitter{
		response: interaction.UserResponse{ActionID: "re_verify"},
	}
	phase := &VerificationPhase{}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "code",
		Phase:      "verify",
		PhaseIndex: 3,
		PhaseCount: 5,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if outcome.JumpTo != "verify" {
		t.Errorf("jump_to: got %q, want 'verify'", outcome.JumpTo)
	}
}

func TestCodePresentPhase_Accept(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &CodePresentPhase{}

	mc := interaction.PhaseMachineContext{
		Emitter: emitter,
		State: map[string]any{
			"verify.result": interaction.ResultContent{Status: "passed"},
			"scope":         []string{"handler.go"},
		},
		Artifacts:  interaction.NewArtifactBundle(),
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
	if emitter.Frames[0].Kind != interaction.FrameSummary {
		t.Errorf("kind: got %q, want summary", emitter.Frames[0].Kind)
	}
	// Default is "accept", no transition.
	if outcome.Transition != "" {
		t.Errorf("transition: got %q, want empty", outcome.Transition)
	}
}

func TestCodePresentPhase_Review(t *testing.T) {
	emitter := &fixedResponseEmitter{
		response: interaction.UserResponse{ActionID: "review"},
	}
	phase := &CodePresentPhase{}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "code",
		Phase:      "present",
		PhaseIndex: 4,
		PhaseCount: 5,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if outcome.Transition != "review" {
		t.Errorf("transition: got %q, want 'review'", outcome.Transition)
	}
}

func TestCodePresentPhase_Undo(t *testing.T) {
	emitter := &fixedResponseEmitter{
		response: interaction.UserResponse{ActionID: "undo"},
	}
	phase := &CodePresentPhase{}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "code",
		Phase:      "present",
		PhaseIndex: 4,
		PhaseCount: 5,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if outcome.StateUpdates["present.undo"] != true {
		t.Error("expected undo flag")
	}
}

func TestRegisterCodeTriggers(t *testing.T) {
	resolver := interaction.NewAgencyResolver()
	RegisterCodeTriggers(resolver)

	triggers := resolver.TriggersForMode("code")
	if len(triggers) != 5 {
		t.Errorf("triggers: got %d, want 5", len(triggers))
	}

	tests := []struct {
		text    string
		wantOK  bool
		wantDesc string
	}{
		{"verify", true, "Re-run verification"},
		{"try different approach", true, "Re-enter execute with paradigm switch"},
		{"debug this", true, "Propose transition to debug mode"},
		{"plan first", true, "Transition to planning, return with plan artifact"},
		{"just do it", true, "Skip proposal phase, fast-path through execution"},
		{"unknown", false, ""},
	}
	for _, tt := range tests {
		trigger, ok := resolver.Resolve("code", tt.text)
		if ok != tt.wantOK {
			t.Errorf("Resolve(%q): got ok=%v, want %v", tt.text, ok, tt.wantOK)
			continue
		}
		if ok && trigger.Description != tt.wantDesc {
			t.Errorf("Resolve(%q): desc=%q, want %q", tt.text, trigger.Description, tt.wantDesc)
		}
	}
}

func TestRegisterCodeTriggers_NilResolver(t *testing.T) {
	RegisterCodeTriggers(nil)
}

func TestCodePhaseLabels(t *testing.T) {
	labels := CodePhaseLabels()
	if len(labels) != 5 {
		t.Fatalf("labels: got %d, want 5", len(labels))
	}
	ids := CodePhaseIDs()
	for i, l := range labels {
		if l.ID != ids[i] {
			t.Errorf("label[%d].ID: got %q, want %q", i, l.ID, ids[i])
		}
	}
}

func TestSkipPropose(t *testing.T) {
	tests := []struct {
		name  string
		state map[string]any
		want  bool
	}{
		{"just_do_it", map[string]any{"just_do_it": true}, true},
		{"small_change", map[string]any{"understand.small_change": true}, true},
		{"normal", map[string]any{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := skipPropose(tt.state, interaction.NewArtifactBundle())
			if got != tt.want {
				t.Errorf("skipPropose: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildVerifyActions_Passed(t *testing.T) {
	actions := buildVerifyActions(interaction.ResultContent{Status: "passed"}, 0)
	if len(actions) != 2 {
		t.Errorf("actions: got %d, want 2", len(actions))
	}
	if actions[0].ID != "done" {
		t.Errorf("first action: got %q, want 'done'", actions[0].ID)
	}
}

func TestBuildVerifyActions_Failed_BelowThreshold(t *testing.T) {
	actions := buildVerifyActions(interaction.ResultContent{Status: "failed"}, 1)
	// Should have: fix_gaps, re_verify, different_approach, debug (non-default).
	hasDebug := false
	debugDefault := false
	for _, a := range actions {
		if a.ID == "debug" {
			hasDebug = true
			debugDefault = a.Default
		}
	}
	if !hasDebug {
		t.Error("should have debug action")
	}
	if debugDefault {
		t.Error("debug should not be default below threshold")
	}
}

func TestBuildVerifyActions_Failed_AtThreshold(t *testing.T) {
	actions := buildVerifyActions(interaction.ResultContent{Status: "failed"}, 2)
	// Debug should be first and default.
	if actions[0].ID != "debug" {
		t.Errorf("first action: got %q, want 'debug'", actions[0].ID)
	}
	if !actions[0].Default {
		t.Error("debug should be default at threshold")
	}
	// fix_gaps should NOT be default.
	for _, a := range actions {
		if a.ID == "fix_gaps" && a.Default {
			t.Error("fix_gaps should not be default when debug is default")
		}
	}
}

// fixedResponseEmitter always returns a fixed UserResponse.
type fixedResponseEmitter struct {
	frames   []interaction.InteractionFrame
	response interaction.UserResponse
}

func (e *fixedResponseEmitter) Emit(_ context.Context, frame interaction.InteractionFrame) error {
	e.frames = append(e.frames, frame)
	return nil
}

func (e *fixedResponseEmitter) AwaitResponse(_ context.Context) (interaction.UserResponse, error) {
	return e.response, nil
}
