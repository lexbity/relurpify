package modes

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

func TestPlanningMode_FullFlow(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	resolver := interaction.NewAgencyResolver()
	m := PlanningMode(emitter, resolver)

	// Seed state with instruction for scope phase.
	m.State()["instruction"] = "Add retry logic to HTTP client"
	m.State()["scope"] = []string{"pkg/http/client.go"}

	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Should have produced a plan artifact.
	if !m.Artifacts().Has(euclotypes.ArtifactKindPlan) {
		t.Error("expected plan artifact")
	}

	// Commit phase should have proposed transition to code (default action = execute).
	if m.State()["transition.accepted"] != "code" {
		t.Errorf("expected transition to code, got %v", m.State()["transition.accepted"])
	}
}

func TestPlanningMode_SkipClarify_OnConfirm(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	resolver := interaction.NewAgencyResolver()
	m := PlanningMode(emitter, resolver)

	m.State()["instruction"] = "Simple task"

	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Scope default is "confirm" → clarify should be skipped.
	// Check that no question frames were emitted (clarify uses FrameQuestion).
	for _, f := range emitter.Frames {
		if f.Kind == interaction.FrameQuestion && f.Phase == "clarify" {
			t.Error("clarify phase should have been skipped after scope confirm")
		}
	}
}

func TestPlanningMode_SkipCompare_SingleCandidate(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	resolver := interaction.NewAgencyResolver()
	m := PlanningMode(emitter, resolver)

	m.State()["instruction"] = "Simple task"

	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Default generate produces 1 candidate → compare should be skipped.
	for _, f := range emitter.Frames {
		if f.Kind == interaction.FrameComparison {
			t.Error("compare phase should have been skipped with single candidate")
		}
	}
}

func TestPlanningMode_JustPlanIt(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	resolver := interaction.NewAgencyResolver()
	m := PlanningMode(emitter, resolver)

	// Simulate "just plan it" by pre-setting state.
	m.State()["instruction"] = "Quick task"
	m.State()["just_plan_it"] = true

	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Clarify and refine should be skipped.
	for _, f := range emitter.Frames {
		if f.Phase == "clarify" && f.Kind == interaction.FrameQuestion {
			t.Error("clarify should be skipped with just_plan_it")
		}
		if f.Phase == "refine" {
			t.Error("refine should be skipped with just_plan_it")
		}
	}

	if !m.Artifacts().Has(euclotypes.ArtifactKindPlan) {
		t.Error("should still produce plan artifact")
	}
}

func TestScopePhase_DefaultProposal(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &ScopePhase{}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{"instruction": "Add tests", "scope": []string{"main.go"}},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "planning",
		Phase:      "scope",
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
	if len(emitter.Frames) != 1 {
		t.Fatalf("frames: got %d, want 1", len(emitter.Frames))
	}
	if emitter.Frames[0].Kind != interaction.FrameProposal {
		t.Errorf("kind: got %q, want proposal", emitter.Frames[0].Kind)
	}
	if outcome.StateUpdates["scope.response"] != "confirm" {
		t.Errorf("response: got %v", outcome.StateUpdates["scope.response"])
	}
}

func TestScopePhase_CustomAnalyzer(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	analyzerCalled := false
	phase := &ScopePhase{
		AnalyzeScope: func(_ context.Context, mc interaction.PhaseMachineContext) (interaction.ProposalContent, error) {
			analyzerCalled = true
			return interaction.ProposalContent{
				Interpretation: "Custom analysis",
				Scope:          []string{"custom.go"},
			}, nil
		},
	}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "planning",
		Phase:      "scope",
		PhaseIndex: 0,
		PhaseCount: 6,
	}

	_, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !analyzerCalled {
		t.Error("custom analyzer should have been called")
	}
}

func TestClarifyPhase_NoQuestions(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &ClarifyPhase{}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "planning",
		Phase:      "clarify",
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
	if len(emitter.Frames) != 0 {
		t.Errorf("no frames expected when no questions, got %d", len(emitter.Frames))
	}
}

func TestClarifyPhase_WithQuestions(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &ClarifyPhase{
		GenerateQuestions: func(_ context.Context, _ interaction.PhaseMachineContext) ([]interaction.QuestionContent, error) {
			return []interaction.QuestionContent{
				{
					Question: "REST or GraphQL?",
					Options: []interaction.QuestionOption{
						{ID: "rest", Label: "REST"},
						{ID: "graphql", Label: "GraphQL"},
					},
				},
				{
					Question: "New file or extend existing?",
					Options: []interaction.QuestionOption{
						{ID: "new", Label: "New file"},
						{ID: "extend", Label: "Extend existing"},
					},
				},
			}, nil
		},
	}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "planning",
		Phase:      "clarify",
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
	// Should have emitted 2 question frames.
	if len(emitter.Frames) != 2 {
		t.Errorf("frames: got %d, want 2", len(emitter.Frames))
	}
	answers, _ := outcome.StateUpdates["clarify.answers"].([]map[string]any)
	if len(answers) != 2 {
		t.Errorf("answers: got %d, want 2", len(answers))
	}
}

func TestClarifyPhase_CapsAt3Questions(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &ClarifyPhase{
		GenerateQuestions: func(_ context.Context, _ interaction.PhaseMachineContext) ([]interaction.QuestionContent, error) {
			return []interaction.QuestionContent{
				{Question: "Q1", Options: []interaction.QuestionOption{{ID: "a", Label: "A"}}},
				{Question: "Q2", Options: []interaction.QuestionOption{{ID: "a", Label: "A"}}},
				{Question: "Q3", Options: []interaction.QuestionOption{{ID: "a", Label: "A"}}},
				{Question: "Q4", Options: []interaction.QuestionOption{{ID: "a", Label: "A"}}},
				{Question: "Q5", Options: []interaction.QuestionOption{{ID: "a", Label: "A"}}},
			}, nil
		},
	}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "planning",
		Phase:      "clarify",
		PhaseIndex: 1,
		PhaseCount: 6,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Should cap at 3.
	if len(emitter.Frames) != 3 {
		t.Errorf("frames: got %d, want 3 (capped)", len(emitter.Frames))
	}
	count, _ := outcome.StateUpdates["clarify.question_count"].(int)
	if count != 3 {
		t.Errorf("question_count: got %d, want 3", count)
	}
}

func TestGeneratePhase_DefaultCandidate(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &GeneratePhase{}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "planning",
		Phase:      "generate",
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
	// Status + candidates frames.
	if len(emitter.Frames) != 2 {
		t.Errorf("frames: got %d, want 2", len(emitter.Frames))
	}
	if emitter.Frames[0].Kind != interaction.FrameStatus {
		t.Errorf("frame[0]: got %q, want status", emitter.Frames[0].Kind)
	}
	if emitter.Frames[1].Kind != interaction.FrameCandidates {
		t.Errorf("frame[1]: got %q, want candidates", emitter.Frames[1].Kind)
	}
	count, _ := outcome.StateUpdates["generate.candidate_count"].(int)
	if count != 1 {
		t.Errorf("candidate_count: got %d, want 1", count)
	}
}

func TestGeneratePhase_MultipleCandidates(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &GeneratePhase{
		GenerateCandidates: func(_ context.Context, _ interaction.PhaseMachineContext) ([]interaction.Candidate, error) {
			return []interaction.Candidate{
				{ID: "a", Summary: "Plan A", Properties: map[string]string{"risk": "low"}},
				{ID: "b", Summary: "Plan B", Properties: map[string]string{"risk": "high"}},
			}, nil
		},
	}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "planning",
		Phase:      "generate",
		PhaseIndex: 2,
		PhaseCount: 6,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	count, _ := outcome.StateUpdates["generate.candidate_count"].(int)
	if count != 2 {
		t.Errorf("candidate_count: got %d, want 2", count)
	}
	// NoopEmitter selects default (first = "a").
	selected, _ := outcome.StateUpdates["generate.selected"].(string)
	if selected != "a" {
		t.Errorf("selected: got %q, want 'a'", selected)
	}
}

func TestComparePhase_BuildsMatrix(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &ComparePhase{}

	candidates := []interaction.Candidate{
		{ID: "a", Summary: "Plan A", Properties: map[string]string{"risk": "low", "speed": "fast"}},
		{ID: "b", Summary: "Plan B", Properties: map[string]string{"risk": "high", "speed": "slow"}},
	}

	mc := interaction.PhaseMachineContext{
		Emitter: emitter,
		State: map[string]any{
			"generate.candidates": candidates,
		},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "planning",
		Phase:      "compare",
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
	if emitter.Frames[0].Kind != interaction.FrameComparison {
		t.Errorf("kind: got %q, want comparison", emitter.Frames[0].Kind)
	}
}

func TestComparePhase_NoCandidates(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &ComparePhase{}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "planning",
		Phase:      "compare",
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
	if len(emitter.Frames) != 0 {
		t.Errorf("no frames expected with no candidates, got %d", len(emitter.Frames))
	}
}

func TestRefinePhase_DefaultDraft(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &RefinePhase{}

	mc := interaction.PhaseMachineContext{
		Emitter: emitter,
		State: map[string]any{
			"generate.selected_summary": "Add retry to HTTP client",
		},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "planning",
		Phase:      "refine",
		PhaseIndex: 4,
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

func TestCommitPhase_ProducesArtifact(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &CommitPhase{}

	mc := interaction.PhaseMachineContext{
		Emitter: emitter,
		State: map[string]any{
			"generate.selected_summary": "Plan A",
		},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "planning",
		Phase:      "commit",
		PhaseIndex: 5,
		PhaseCount: 6,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !outcome.Advance {
		t.Error("expected advance")
	}
	if len(outcome.Artifacts) != 1 {
		t.Fatalf("artifacts: got %d, want 1", len(outcome.Artifacts))
	}
	if outcome.Artifacts[0].Kind != euclotypes.ArtifactKindPlan {
		t.Errorf("artifact kind: got %q, want plan", outcome.Artifacts[0].Kind)
	}
	// Default action is "execute" → should propose transition to code.
	if outcome.Transition != "code" {
		t.Errorf("transition: got %q, want 'code'", outcome.Transition)
	}
	if len(emitter.Frames) != 1 {
		t.Fatalf("frames: got %d, want 1", len(emitter.Frames))
	}
	if emitter.Frames[0].Kind != interaction.FrameSummary {
		t.Errorf("kind: got %q, want summary", emitter.Frames[0].Kind)
	}
}

func TestRegisterPlanningTriggers(t *testing.T) {
	resolver := interaction.NewAgencyResolver()
	RegisterPlanningTriggers(resolver)

	triggers := resolver.TriggersForMode("planning")
	if len(triggers) != 3 {
		t.Errorf("triggers: got %d, want 3", len(triggers))
	}

	// Test alternatives trigger.
	trigger, ok := resolver.Resolve("planning", "show alternatives")
	if !ok {
		t.Fatal("expected alternatives trigger")
	}
	if trigger.PhaseJump != "generate" {
		t.Errorf("alternatives phase_jump: got %q, want 'generate'", trigger.PhaseJump)
	}
	if trigger.CapabilityID != "euclo:design.alternatives" {
		t.Errorf("alternatives capability: got %q, want euclo:design.alternatives", trigger.CapabilityID)
	}

	// Test just plan it trigger.
	trigger, ok = resolver.Resolve("planning", "just plan it")
	if !ok {
		t.Fatal("expected just_plan_it trigger")
	}
	if trigger.Description == "" {
		t.Error("trigger should have description")
	}

	// Test risk analysis trigger.
	trigger, ok = resolver.Resolve("planning", "what are the risks")
	if !ok {
		t.Fatal("expected risks trigger")
	}
	if trigger.CapabilityID != "euclo:planner.plan" {
		t.Errorf("risks capability: got %q", trigger.CapabilityID)
	}
}

func TestPlanningPhaseLabels(t *testing.T) {
	labels := PlanningPhaseLabels()
	if len(labels) != 6 {
		t.Fatalf("labels: got %d, want 6", len(labels))
	}
	ids := PlanningPhaseIDs()
	for i, l := range labels {
		if l.ID != ids[i] {
			t.Errorf("label[%d].ID: got %q, want %q", i, l.ID, ids[i])
		}
	}
}

func TestRegisterPlanningTriggers_NilResolver(t *testing.T) {
	// Should not panic.
	RegisterPlanningTriggers(nil)
}

func TestSkipClarify(t *testing.T) {
	tests := []struct {
		name  string
		state map[string]any
		want  bool
	}{
		{"confirmed", map[string]any{"scope.response": "confirm"}, true},
		{"corrected", map[string]any{"scope.response": "clarify"}, false},
		{"just_plan_it", map[string]any{"just_plan_it": true}, true},
		{"empty", map[string]any{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := skipClarify(tt.state, interaction.NewArtifactBundle())
			if got != tt.want {
				t.Errorf("skipClarify: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSkipCompare(t *testing.T) {
	tests := []struct {
		name  string
		state map[string]any
		want  bool
	}{
		{"single_candidate", map[string]any{"generate.candidate_count": 1}, true},
		{"already_selected", map[string]any{"generate.candidate_count": 3, "generate.selected": "a"}, true},
		{"multiple_no_selection", map[string]any{"generate.candidate_count": 3}, false},
		{"just_plan_it", map[string]any{"just_plan_it": true}, true},
		{"zero_candidates", map[string]any{"generate.candidate_count": 0}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := skipCompare(tt.state, interaction.NewArtifactBundle())
			if got != tt.want {
				t.Errorf("skipCompare: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSkipRefine(t *testing.T) {
	tests := []struct {
		name  string
		state map[string]any
		want  bool
	}{
		{"just_plan_it", map[string]any{"just_plan_it": true}, true},
		{"normal", map[string]any{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := skipRefine(tt.state, interaction.NewArtifactBundle())
			if got != tt.want {
				t.Errorf("skipRefine: got %v, want %v", got, tt.want)
			}
		})
	}
}
