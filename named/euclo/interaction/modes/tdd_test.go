package modes

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

func TestTDDMode_FullFlow(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	resolver := interaction.NewAgencyResolver()
	m := TDDMode(emitter, resolver)

	m.State()["instruction"] = "Add tests for the auth module"

	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(emitter.Frames) == 0 {
		t.Fatal("expected frames to be emitted")
	}
}

func TestTDDMode_SkipSpecify(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	resolver := interaction.NewAgencyResolver()
	m := TDDMode(emitter, resolver)

	m.State()["instruction"] = "Test cases already specified"
	m.State()["has_test_specs"] = true

	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, f := range emitter.Frames {
		if f.Phase == "specify" && f.Kind == interaction.FrameQuestion {
			t.Error("specify phase should have been skipped")
		}
	}
}

func TestBehaviorSpecPhase_DefaultQuestions(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &BehaviorSpecPhase{}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "tdd",
		Phase:      "specify",
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
	// Default has 3 questions.
	if len(emitter.Frames) != 3 {
		t.Errorf("frames: got %d, want 3", len(emitter.Frames))
	}
	totalCases, _ := outcome.StateUpdates["specify.total_cases"].(int)
	if totalCases != 3 {
		t.Errorf("total_cases: got %d, want 3", totalCases)
	}
}

func TestBehaviorSpecPhase_Accumulation(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &BehaviorSpecPhase{
		GenerateQuestions: func(_ context.Context, _ interaction.PhaseMachineContext) ([]interaction.QuestionContent, error) {
			return []interaction.QuestionContent{
				{Question: "Happy path?", Options: []interaction.QuestionOption{{ID: "ok", Label: "OK"}}},
				{Question: "Edge case with empty input?", Options: []interaction.QuestionOption{{ID: "empty", Label: "Empty"}}},
				{Question: "Error on invalid input?", Options: []interaction.QuestionOption{{ID: "err", Label: "Error"}}},
			}, nil
		},
	}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "tdd",
		Phase:      "specify",
		PhaseIndex: 0,
		PhaseCount: 5,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	spec, ok := outcome.StateUpdates["specify.spec"].(BehaviorSpec)
	if !ok {
		t.Fatal("expected BehaviorSpec in state")
	}
	// "Happy path?" → happy, "Edge case with empty input?" → edge, "Error on invalid input?" → error
	if len(spec.HappyPaths) != 1 {
		t.Errorf("happy_paths: got %d, want 1", len(spec.HappyPaths))
	}
	if len(spec.EdgeCases) != 1 {
		t.Errorf("edge_cases: got %d, want 1", len(spec.EdgeCases))
	}
	if len(spec.ErrorCases) != 1 {
		t.Errorf("error_cases: got %d, want 1", len(spec.ErrorCases))
	}
}

func TestBehaviorSpec_AllCases(t *testing.T) {
	spec := BehaviorSpec{
		HappyPaths: []BehaviorCase{{Description: "h1"}, {Description: "h2"}},
		EdgeCases:  []BehaviorCase{{Description: "e1"}},
		ErrorCases: []BehaviorCase{{Description: "err1"}},
	}
	all := spec.AllCases()
	if len(all) != 4 {
		t.Errorf("all cases: got %d, want 4", len(all))
	}
	if spec.TotalCases() != 4 {
		t.Errorf("total: got %d, want 4", spec.TotalCases())
	}
}

func TestBehaviorSpec_Empty(t *testing.T) {
	spec := BehaviorSpec{}
	if spec.TotalCases() != 0 {
		t.Errorf("total: got %d, want 0", spec.TotalCases())
	}
	if len(spec.AllCases()) != 0 {
		t.Errorf("all: got %d, want 0", len(spec.AllCases()))
	}
}

func TestTestDraftPhase_Default(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &TestDraftPhase{}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "tdd",
		Phase:      "test_draft",
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

func TestTestDraftPhase_FromSpec(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &TestDraftPhase{}

	spec := BehaviorSpec{
		HappyPaths: []BehaviorCase{{Description: "returns correct sum"}},
		EdgeCases:  []BehaviorCase{{Description: "handles zero"}},
	}

	mc := interaction.PhaseMachineContext{
		Emitter: emitter,
		State: map[string]any{
			"specify.spec": spec,
		},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "tdd",
		Phase:      "test_draft",
		PhaseIndex: 1,
		PhaseCount: 5,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	items, _ := outcome.StateUpdates["test_draft.items"].([]interaction.DraftItem)
	if len(items) != 2 {
		t.Errorf("items: got %d, want 2", len(items))
	}
}

func TestTestResultPhase_AllRed(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &TestResultPhase{}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "tdd",
		Phase:      "review_tests",
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
	// Default action is "implement" (not "fix" — TDD red is normal).
	if outcome.StateUpdates["review_tests.response"] != "implement" {
		t.Errorf("response: got %v, want 'implement'", outcome.StateUpdates["review_tests.response"])
	}
}

func TestTestResultPhase_AddTests(t *testing.T) {
	emitter := &fixedResponseEmitter{
		response: interaction.UserResponse{ActionID: "add_tests"},
	}
	phase := &TestResultPhase{}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "tdd",
		Phase:      "review_tests",
		PhaseIndex: 2,
		PhaseCount: 5,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if outcome.JumpTo != "specify" {
		t.Errorf("jump_to: got %q, want 'specify'", outcome.JumpTo)
	}
}

func TestGreenStatusPhase_AllGreen(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &GreenStatusPhase{}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "tdd",
		Phase:      "green",
		PhaseIndex: 4,
		PhaseCount: 5,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Default on pass is "done".
	if outcome.StateUpdates["green.response"] != "done" {
		t.Errorf("response: got %v, want 'done'", outcome.StateUpdates["green.response"])
	}
}

func TestGreenStatusPhase_Failing(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &GreenStatusPhase{
		RunTests: func(_ context.Context, _ interaction.PhaseMachineContext) (interaction.ResultContent, error) {
			return interaction.ResultContent{Status: "partial"}, nil
		},
	}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "tdd",
		Phase:      "green",
		PhaseIndex: 4,
		PhaseCount: 5,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Default on partial is "fix".
	if outcome.StateUpdates["green.response"] != "fix" {
		t.Errorf("response: got %v, want 'fix'", outcome.StateUpdates["green.response"])
	}
	if outcome.JumpTo != "implement" {
		t.Errorf("jump_to: got %q, want 'implement'", outcome.JumpTo)
	}
}

func TestGreenStatusPhase_Refactor(t *testing.T) {
	emitter := &fixedResponseEmitter{
		response: interaction.UserResponse{ActionID: "refactor"},
	}
	phase := &GreenStatusPhase{}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "tdd",
		Phase:      "green",
		PhaseIndex: 4,
		PhaseCount: 5,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if outcome.Transition != "code" {
		t.Errorf("transition: got %q, want 'code'", outcome.Transition)
	}
	if outcome.StateUpdates["green.refactor_constraint"] != "tests must stay green" {
		t.Errorf("constraint: got %v", outcome.StateUpdates["green.refactor_constraint"])
	}
}

func TestGreenStatusPhase_AddMoreTests(t *testing.T) {
	emitter := &fixedResponseEmitter{
		response: interaction.UserResponse{ActionID: "add_tests"},
	}
	phase := &GreenStatusPhase{}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "tdd",
		Phase:      "green",
		PhaseIndex: 4,
		PhaseCount: 5,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if outcome.JumpTo != "specify" {
		t.Errorf("jump_to: got %q, want 'specify'", outcome.JumpTo)
	}
}

func TestRegisterTDDTriggers(t *testing.T) {
	resolver := interaction.NewAgencyResolver()
	RegisterTDDTriggers(resolver)

	triggers := resolver.TriggersForMode("tdd")
	if len(triggers) != 3 {
		t.Errorf("triggers: got %d, want 3", len(triggers))
	}

	trigger, ok := resolver.Resolve("tdd", "refactor")
	if !ok {
		t.Fatal("expected refactor trigger")
	}
	if trigger.Description == "" {
		t.Error("trigger should have description")
	}

	trigger, ok = resolver.Resolve("tdd", "add more tests")
	if !ok {
		t.Fatal("expected add more tests trigger")
	}
	if trigger.PhaseJump != "specify" {
		t.Errorf("phase_jump: got %q, want 'specify'", trigger.PhaseJump)
	}
}

func TestRegisterTDDTriggers_NilResolver(t *testing.T) {
	RegisterTDDTriggers(nil)
}

func TestTDDPhaseLabels(t *testing.T) {
	labels := TDDPhaseLabels()
	if len(labels) != 5 {
		t.Fatalf("labels: got %d, want 5", len(labels))
	}
}

func TestSkipSpecify(t *testing.T) {
	tests := []struct {
		name  string
		state map[string]any
		want  bool
	}{
		{"has_specs", map[string]any{"has_test_specs": true}, true},
		{"no_specs", map[string]any{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := skipSpecify(tt.state, interaction.NewArtifactBundle())
			if got != tt.want {
				t.Errorf("skipSpecify: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildGreenActions_Passed(t *testing.T) {
	actions := buildGreenActions(interaction.ResultContent{Status: "passed"})
	if len(actions) != 3 {
		t.Errorf("actions: got %d, want 3", len(actions))
	}
	if actions[0].ID != "done" {
		t.Errorf("first action: got %q, want 'done'", actions[0].ID)
	}
}

func TestBuildGreenActions_Failing(t *testing.T) {
	actions := buildGreenActions(interaction.ResultContent{Status: "partial"})
	if len(actions) != 4 {
		t.Errorf("actions: got %d, want 4", len(actions))
	}
	if actions[0].ID != "fix" {
		t.Errorf("first action: got %q, want 'fix'", actions[0].ID)
	}
}
