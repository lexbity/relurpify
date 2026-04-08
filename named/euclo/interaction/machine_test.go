package interaction

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

// Mock phase handler for testing
type mockPhaseHandler struct {
	executeFunc func(ctx context.Context, mc PhaseMachineContext) (PhaseOutcome, error)
}

func (m *mockPhaseHandler) Execute(ctx context.Context, mc PhaseMachineContext) (PhaseOutcome, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, mc)
	}
	return PhaseOutcome{Advance: true}, nil
}

// Mock frame emitter for testing
type testFrameEmitter struct {
	frames   []InteractionFrame
	response UserResponse
	err      error
}

func (t *testFrameEmitter) Emit(ctx context.Context, frame InteractionFrame) error {
	t.frames = append(t.frames, frame)
	return t.err
}

func (t *testFrameEmitter) AwaitResponse(ctx context.Context) (UserResponse, error) {
	return t.response, t.err
}

func TestNewPhaseMachine(t *testing.T) {
	emitter := &testFrameEmitter{}
	phases := []PhaseDefinition{
		{
			ID:      "phase1",
			Label:   "Phase 1",
			Handler: &mockPhaseHandler{},
		},
	}

	cfg := PhaseMachineConfig{
		Mode:    "chat",
		Phases:  phases,
		Emitter: emitter,
	}

	machine := NewPhaseMachine(cfg)
	if machine == nil {
		t.Fatal("NewPhaseMachine returned nil")
	}

	if machine.mode != "chat" {
		t.Errorf("Expected mode 'chat', got %s", machine.mode)
	}
	if len(machine.phases) != 1 {
		t.Errorf("Expected 1 phase, got %d", len(machine.phases))
	}
	if machine.current != 0 {
		t.Errorf("Expected current phase index 0, got %d", machine.current)
	}
}

func TestPhaseMachineState(t *testing.T) {
	cfg := PhaseMachineConfig{
		Mode:    "test",
		Phases:  []PhaseDefinition{},
		Emitter: &testFrameEmitter{},
	}

	machine := NewPhaseMachine(cfg)

	// Test State()
	state := machine.State()
	if state == nil {
		t.Fatal("State should not return nil")
	}

	// Modify state through machine
	machine.state["test.key"] = "test.value"
	if machine.State()["test.key"] != "test.value" {
		t.Error("State should reflect modifications")
	}
}

func TestPhaseMachineArtifacts(t *testing.T) {
	cfg := PhaseMachineConfig{
		Mode:    "test",
		Phases:  []PhaseDefinition{},
		Emitter: &testFrameEmitter{},
	}

	machine := NewPhaseMachine(cfg)

	// Test Artifacts()
	artifacts := machine.Artifacts()
	if artifacts == nil {
		t.Fatal("Artifacts should not return nil")
	}

	// Add artifact
	artifact := euclotypes.Artifact{
		ID:   "test",
		Kind: euclotypes.ArtifactKindExplore,
	}
	artifacts.Add(artifact)

	if len(machine.Artifacts().All()) != 1 {
		t.Error("Artifacts should reflect additions")
	}
}

func TestPhaseMachineCurrentPhase(t *testing.T) {
	phases := []PhaseDefinition{
		{ID: "phase1", Handler: &mockPhaseHandler{}},
		{ID: "phase2", Handler: &mockPhaseHandler{}},
	}

	cfg := PhaseMachineConfig{
		Mode:    "test",
		Phases:  phases,
		Emitter: &testFrameEmitter{},
	}

	machine := NewPhaseMachine(cfg)

	// Initially at first phase
	if machine.CurrentPhase() != "phase1" {
		t.Errorf("Expected CurrentPhase 'phase1', got %s", machine.CurrentPhase())
	}

	// Advance
	machine.current = 1
	if machine.CurrentPhase() != "phase2" {
		t.Errorf("Expected CurrentPhase 'phase2', got %s", machine.CurrentPhase())
	}

	// Beyond phases
	machine.current = 2
	if machine.CurrentPhase() != "" {
		t.Errorf("Expected empty CurrentPhase when beyond phases, got %s", machine.CurrentPhase())
	}
}

func TestPhaseMachineJumpToPhase(t *testing.T) {
	phases := []PhaseDefinition{
		{ID: "phase1", Handler: &mockPhaseHandler{}},
		{ID: "phase2", Handler: &mockPhaseHandler{}},
		{ID: "phase3", Handler: &mockPhaseHandler{}},
	}

	cfg := PhaseMachineConfig{
		Mode:    "test",
		Phases:  phases,
		Emitter: &testFrameEmitter{},
	}

	machine := NewPhaseMachine(cfg)

	// Jump to existing phase
	if !machine.JumpToPhase("phase3") {
		t.Error("JumpToPhase should succeed for existing phase")
	}
	if machine.current != 2 {
		t.Errorf("Expected current index 2 after jump, got %d", machine.current)
	}

	// Jump to non-existent phase
	if machine.JumpToPhase("nonexistent") {
		t.Error("JumpToPhase should fail for non-existent phase")
	}
	// Current index should remain unchanged
	if machine.current != 2 {
		t.Errorf("Current index should remain 2 after failed jump, got %d", machine.current)
	}
}

func TestPhaseMachineRun(t *testing.T) {
	ctx := context.Background()

	// Track execution order
	executionOrder := []string{}

	phases := []PhaseDefinition{
		{
			ID:    "phase1",
			Label: "Phase 1",
			Handler: &mockPhaseHandler{
				executeFunc: func(ctx context.Context, mc PhaseMachineContext) (PhaseOutcome, error) {
					executionOrder = append(executionOrder, "phase1")
					return PhaseOutcome{Advance: true}, nil
				},
			},
		},
		{
			ID:    "phase2",
			Label: "Phase 2",
			Handler: &mockPhaseHandler{
				executeFunc: func(ctx context.Context, mc PhaseMachineContext) (PhaseOutcome, error) {
					executionOrder = append(executionOrder, "phase2")
					return PhaseOutcome{Advance: true}, nil
				},
			},
		},
	}

	emitter := &testFrameEmitter{}
	cfg := PhaseMachineConfig{
		Mode:    "test",
		Phases:  phases,
		Emitter: emitter,
	}

	machine := NewPhaseMachine(cfg)

	// Run the machine
	err := machine.Run(ctx)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Check execution order
	if len(executionOrder) != 2 {
		t.Fatalf("Expected 2 phases executed, got %d", len(executionOrder))
	}
	if executionOrder[0] != "phase1" || executionOrder[1] != "phase2" {
		t.Errorf("Execution order incorrect: %v", executionOrder)
	}

	// Check executed phases
	executed := machine.ExecutedPhases()
	if len(executed) != 2 {
		t.Errorf("Expected 2 executed phases, got %d", len(executed))
	}
}

func TestPhaseMachineRunWithSkip(t *testing.T) {
	ctx := context.Background()

	executionOrder := []string{}

	phases := []PhaseDefinition{
		{
			ID:    "phase1",
			Label: "Phase 1",
			Handler: &mockPhaseHandler{
				executeFunc: func(ctx context.Context, mc PhaseMachineContext) (PhaseOutcome, error) {
					executionOrder = append(executionOrder, "phase1")
					return PhaseOutcome{Advance: true}, nil
				},
			},
			SkipWhen: func(state map[string]any, artifacts *ArtifactBundle) bool {
				return state["skip"] == true
			},
		},
		{
			ID:    "phase2",
			Label: "Phase 2",
			Handler: &mockPhaseHandler{
				executeFunc: func(ctx context.Context, mc PhaseMachineContext) (PhaseOutcome, error) {
					executionOrder = append(executionOrder, "phase2")
					return PhaseOutcome{Advance: true}, nil
				},
			},
		},
	}

	emitter := &testFrameEmitter{}
	cfg := PhaseMachineConfig{
		Mode:    "test",
		Phases:  phases,
		Emitter: emitter,
	}

	machine := NewPhaseMachine(cfg)
	machine.state["skip"] = true

	err := machine.Run(ctx)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// phase1 should be skipped, only phase2 executed
	if len(executionOrder) != 1 {
		t.Fatalf("Expected 1 phase executed (phase1 skipped), got %d", len(executionOrder))
	}
	if executionOrder[0] != "phase2" {
		t.Errorf("Expected phase2 executed, got %s", executionOrder[0])
	}

	skipped := machine.SkippedPhases()
	if len(skipped) != 1 || skipped[0] != "phase1" {
		t.Errorf("Expected skipped phases ['phase1'], got %v", skipped)
	}
}

func TestPhaseMachineRunWithJump(t *testing.T) {
	ctx := context.Background()

	executionOrder := []string{}

	phases := []PhaseDefinition{
		{
			ID:    "phase1",
			Label: "Phase 1",
			Handler: &mockPhaseHandler{
				executeFunc: func(ctx context.Context, mc PhaseMachineContext) (PhaseOutcome, error) {
					executionOrder = append(executionOrder, "phase1")
					return PhaseOutcome{JumpTo: "phase3"}, nil
				},
			},
		},
		{
			ID:    "phase2",
			Label: "Phase 2",
			Handler: &mockPhaseHandler{
				executeFunc: func(ctx context.Context, mc PhaseMachineContext) (PhaseOutcome, error) {
					executionOrder = append(executionOrder, "phase2")
					return PhaseOutcome{Advance: true}, nil
				},
			},
		},
		{
			ID:    "phase3",
			Label: "Phase 3",
			Handler: &mockPhaseHandler{
				executeFunc: func(ctx context.Context, mc PhaseMachineContext) (PhaseOutcome, error) {
					executionOrder = append(executionOrder, "phase3")
					return PhaseOutcome{Advance: true}, nil
				},
			},
		},
	}

	emitter := &testFrameEmitter{}
	cfg := PhaseMachineConfig{
		Mode:    "test",
		Phases:  phases,
		Emitter: emitter,
	}

	machine := NewPhaseMachine(cfg)

	err := machine.Run(ctx)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// phase1 should jump to phase3, skipping phase2
	if len(executionOrder) != 2 {
		t.Fatalf("Expected 2 phases executed, got %d", len(executionOrder))
	}
	if executionOrder[0] != "phase1" || executionOrder[1] != "phase3" {
		t.Errorf("Execution order incorrect: %v", executionOrder)
	}
}

func TestPhaseMachineRunWithTransition(t *testing.T) {
	ctx := context.Background()

	emitter := &testFrameEmitter{
		response: UserResponse{ActionID: "accept"},
	}

	phases := []PhaseDefinition{
		{
			ID:    "phase1",
			Label: "Phase 1",
			Handler: &mockPhaseHandler{
				executeFunc: func(ctx context.Context, mc PhaseMachineContext) (PhaseOutcome, error) {
					return PhaseOutcome{
						Advance:    true,
						Transition: "code",
					}, nil
				},
			},
		},
	}

	cfg := PhaseMachineConfig{
		Mode:    "chat",
		Phases:  phases,
		Emitter: emitter,
	}

	machine := NewPhaseMachine(cfg)

	err := machine.Run(ctx)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Check transition was recorded in state
	if machine.state["transition.accepted"] != "code" {
		t.Error("Transition should be recorded as accepted")
	}

	// Check emitter received transition frame
	if len(emitter.frames) == 0 {
		t.Error("Emitter should have received transition frame")
	}
}
