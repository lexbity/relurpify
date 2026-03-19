package orchestrate

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

// simpleHandler is a test PhaseHandler that advances immediately.
type simpleHandler struct {
	stateKey   string
	stateValue any
	artifacts  []euclotypes.Artifact
	called     *int
}

func (h *simpleHandler) Execute(_ context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	if h.called != nil {
		*h.called = *h.called + 1
	}
	updates := map[string]any{}
	if h.stateKey != "" {
		updates[h.stateKey] = h.stateValue
	}
	return interaction.PhaseOutcome{
		Advance:      true,
		Artifacts:    h.artifacts,
		StateUpdates: updates,
	}, nil
}

// transitionHandler proposes a transition to another mode.
type transitionHandler struct {
	targetMode string
}

func (h *transitionHandler) Execute(_ context.Context, _ interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	return interaction.PhaseOutcome{
		Advance:    true,
		Transition: h.targetMode,
	}, nil
}

func buildTestRegistry() *interaction.ModeMachineRegistry {
	reg := interaction.NewModeMachineRegistry()
	reg.Register("code", func(emitter interaction.FrameEmitter, resolver *interaction.AgencyResolver) *interaction.PhaseMachine {
		return interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
			Mode:     "code",
			Emitter:  emitter,
			Resolver: resolver,
			Phases: []interaction.PhaseDefinition{
				{ID: "intent", Label: "Intent", Handler: &simpleHandler{stateKey: "intent.done", stateValue: true}},
				{ID: "execute", Label: "Execute", Handler: &simpleHandler{
					stateKey:   "execute.done",
					stateValue: true,
					artifacts: []euclotypes.Artifact{
						{Kind: euclotypes.ArtifactKindEditIntent, Summary: "edited"},
					},
				}},
				{ID: "verify", Label: "Verify", Handler: &simpleHandler{stateKey: "verify.done", stateValue: true}},
			},
		})
	})
	reg.Register("debug", func(emitter interaction.FrameEmitter, resolver *interaction.AgencyResolver) *interaction.PhaseMachine {
		return interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
			Mode:     "debug",
			Emitter:  emitter,
			Resolver: resolver,
			Phases: []interaction.PhaseDefinition{
				{ID: "localize", Label: "Localize", Handler: &simpleHandler{stateKey: "localize.done", stateValue: true}},
				{ID: "fix", Label: "Fix", Handler: &simpleHandler{stateKey: "fix.done", stateValue: true}},
			},
		})
	})
	return reg
}

func TestExecuteInteractive_Basic(t *testing.T) {
	pc := &ProfileController{}
	registry := buildTestRegistry()
	emitter := &interaction.NoopEmitter{}

	mode := euclotypes.ModeResolution{ModeID: "code", Source: "test"}
	env := euclotypes.ExecutionEnvelope{
		Task:    &core.Task{ID: "t1", Instruction: "test task"},
		Mode:    mode,
		Profile: euclotypes.ExecutionProfileSelection{ProfileID: "default"},
		State:   core.NewContext(),
	}

	result, icResult, err := pc.ExecuteInteractive(ctx(), registry, mode, env, emitter)
	if err != nil {
		t.Fatalf("ExecuteInteractive: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	if len(icResult.PhasesExecuted) != 3 {
		t.Errorf("phases: got %d, want 3", len(icResult.PhasesExecuted))
	}
	if len(icResult.Artifacts) != 1 {
		t.Errorf("artifacts: got %d, want 1", len(icResult.Artifacts))
	}
	if icResult.TransitionTo != "" {
		t.Errorf("unexpected transition: %q", icResult.TransitionTo)
	}
}

func TestExecuteInteractive_UnregisteredMode(t *testing.T) {
	pc := &ProfileController{}
	registry := interaction.NewModeMachineRegistry()
	emitter := &interaction.NoopEmitter{}

	mode := euclotypes.ModeResolution{ModeID: "unknown", Source: "test"}
	env := euclotypes.ExecutionEnvelope{
		Task: &core.Task{ID: "t1"},
		Mode: mode,
	}

	_, _, err := pc.ExecuteInteractive(ctx(), registry, mode, env, emitter)
	if err == nil {
		t.Error("expected error for unregistered mode")
	}
}

func TestExecuteInteractive_NilRegistry(t *testing.T) {
	pc := &ProfileController{}
	emitter := &interaction.NoopEmitter{}

	mode := euclotypes.ModeResolution{ModeID: "code"}
	env := euclotypes.ExecutionEnvelope{Task: &core.Task{ID: "t1"}, Mode: mode}

	_, _, err := pc.ExecuteInteractive(ctx(), nil, mode, env, emitter)
	if err == nil {
		t.Error("expected error for nil registry")
	}
}

func TestExecuteInteractive_PersistsState(t *testing.T) {
	pc := &ProfileController{}
	registry := buildTestRegistry()
	emitter := &interaction.NoopEmitter{}

	mode := euclotypes.ModeResolution{ModeID: "code", Source: "test"}
	state := core.NewContext()
	env := euclotypes.ExecutionEnvelope{
		Task:    &core.Task{ID: "t1"},
		Mode:    mode,
		Profile: euclotypes.ExecutionProfileSelection{ProfileID: "default"},
		State:   state,
	}

	_, icResult, err := pc.ExecuteInteractive(ctx(), registry, mode, env, emitter)
	if err != nil {
		t.Fatalf("ExecuteInteractive: %v", err)
	}

	// Verify interaction state was persisted.
	if icResult.InteractionState.Mode != "code" {
		t.Errorf("state mode: got %q", icResult.InteractionState.Mode)
	}

	// Verify state is in execution context.
	raw, ok := state.Get("euclo.interaction_state")
	if !ok || raw == nil {
		t.Error("expected interaction state in execution context")
	}
	recording, ok := state.Get("euclo.interaction_recording")
	if !ok || recording == nil {
		t.Error("expected lossy interaction recording in execution context")
	}
	records, ok := state.Get("euclo.interaction_records")
	if !ok || records == nil {
		t.Error("expected full interaction records in execution context")
	}
}

func TestExecuteInteractive_PersistsProposalAsPlan(t *testing.T) {
	pc := &ProfileController{}
	emitter := &interaction.NoopEmitter{}

	reg := interaction.NewModeMachineRegistry()
	reg.Register("code", func(emitter interaction.FrameEmitter, resolver *interaction.AgencyResolver) *interaction.PhaseMachine {
		return interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
			Mode:     "code",
			Emitter:  emitter,
			Resolver: resolver,
			Phases: []interaction.PhaseDefinition{
				{
					ID:    "propose",
					Label: "Propose",
					Handler: &simpleHandler{
						stateKey: "propose.items",
						stateValue: []map[string]any{{
							"id":      "edit-1",
							"content": "Add logging",
						}},
					},
				},
			},
		})
	})

	state := core.NewContext()
	mode := euclotypes.ModeResolution{ModeID: "code", Source: "test"}
	env := euclotypes.ExecutionEnvelope{
		Task:    &core.Task{ID: "t1"},
		Mode:    mode,
		Profile: euclotypes.ExecutionProfileSelection{ProfileID: "default"},
		State:   state,
	}

	_, _, err := pc.ExecuteInteractive(ctx(), reg, mode, env, emitter)
	if err != nil {
		t.Fatalf("ExecuteInteractive: %v", err)
	}

	raw, ok := state.Get("pipeline.plan")
	if !ok || raw == nil {
		t.Fatal("expected pipeline.plan in execution context")
	}
	payload, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("pipeline.plan type: got %T", raw)
	}
	if payload["source"] != "interaction.propose" {
		t.Fatalf("pipeline.plan source: got %v", payload["source"])
	}
}

func TestExecuteInteractive_ResumesFromPersistedPhase(t *testing.T) {
	pc := &ProfileController{}
	emitter := interaction.NewTestFrameEmitter(interaction.ScriptedResponse{
		Kind:     string(interaction.FrameSessionResume),
		ActionID: "resume",
	})

	var scopeCalls, generateCalls, commitCalls int
	reg := interaction.NewModeMachineRegistry()
	reg.Register("planning", func(emitter interaction.FrameEmitter, resolver *interaction.AgencyResolver) *interaction.PhaseMachine {
		return interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
			Mode:     "planning",
			Emitter:  emitter,
			Resolver: resolver,
			Phases: []interaction.PhaseDefinition{
				{ID: "scope", Label: "Scope", Handler: &simpleHandler{stateKey: "scope.done", stateValue: true, called: &scopeCalls}},
				{ID: "clarify", Label: "Clarify", Handler: &simpleHandler{stateKey: "clarify.done", stateValue: true}},
				{ID: "generate", Label: "Generate", Handler: &simpleHandler{stateKey: "generate.done", stateValue: true, called: &generateCalls}},
				{ID: "commit", Label: "Commit", Handler: &simpleHandler{stateKey: "commit.done", stateValue: true, called: &commitCalls}},
			},
		})
	})

	state := core.NewContext()
	state.Set("euclo.interaction_state", map[string]any{
		"mode":            "planning",
		"current_phase":   "generate",
		"phases_executed": []any{"scope", "clarify"},
		"phase_states": map[string]any{
			"scope.done":   true,
			"clarify.done": true,
		},
	})

	mode := euclotypes.ModeResolution{ModeID: "planning", Source: "test"}
	env := euclotypes.ExecutionEnvelope{
		Task:    &core.Task{ID: "resume-1", Instruction: "plan and implement rate limiting"},
		Mode:    mode,
		Profile: euclotypes.ExecutionProfileSelection{ProfileID: "default"},
		State:   state,
	}

	_, icResult, err := pc.ExecuteInteractive(ctx(), reg, mode, env, emitter)
	if err != nil {
		t.Fatalf("ExecuteInteractive: %v", err)
	}
	if scopeCalls != 0 {
		t.Fatalf("expected scope not to rerun, got %d calls", scopeCalls)
	}
	if generateCalls != 1 || commitCalls != 1 {
		t.Fatalf("expected generate/commit to run once, got generate=%d commit=%d", generateCalls, commitCalls)
	}
	if len(icResult.PhasesExecuted) != 2 || icResult.PhasesExecuted[0] != "generate" || icResult.PhasesExecuted[1] != "commit" {
		t.Fatalf("expected resumed phases [generate commit], got %v", icResult.PhasesExecuted)
	}
	resumed, ok := icResult.InteractionState.PhaseStates["session.resumed"].(bool)
	if !ok || !resumed {
		t.Fatal("expected resumed session flag in interaction state")
	}
	if got := emitter.Frames(); len(got) != 1 || got[0].Kind != interaction.FrameSessionResume {
		t.Fatalf("expected one session_resume frame, got %v", got)
	}
}

func TestExecuteInteractive_SeedsTaskDescription(t *testing.T) {
	pc := &ProfileController{}
	emitter := &interaction.NoopEmitter{}

	var capturedState map[string]any
	reg := interaction.NewModeMachineRegistry()
	reg.Register("code", func(emitter interaction.FrameEmitter, resolver *interaction.AgencyResolver) *interaction.PhaseMachine {
		m := interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
			Mode:    "code",
			Emitter: emitter,
			Phases: []interaction.PhaseDefinition{
				{ID: "a", Label: "A", Handler: &captureStateHandler{captured: &capturedState}},
			},
		})
		return m
	})

	mode := euclotypes.ModeResolution{ModeID: "code"}
	env := euclotypes.ExecutionEnvelope{
		Task:    &core.Task{ID: "t1", Instruction: "fix the bug"},
		Mode:    mode,
		Profile: euclotypes.ExecutionProfileSelection{ProfileID: "default"},
		State:   core.NewContext(),
	}

	_, _, err := pc.ExecuteInteractive(ctx(), reg, mode, env, emitter)
	if err != nil {
		t.Fatalf("ExecuteInteractive: %v", err)
	}

	if capturedState["task.instruction"] != "fix the bug" {
		t.Errorf("task.instruction: got %v", capturedState["task.instruction"])
	}
}

// captureStateHandler captures the machine state for inspection.
type captureStateHandler struct {
	captured *map[string]any
}

func (h *captureStateHandler) Execute(_ context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	*h.captured = make(map[string]any)
	for k, v := range mc.State {
		(*h.captured)[k] = v
	}
	return interaction.PhaseOutcome{Advance: true}, nil
}

func TestExecuteInteractiveWithTransitions(t *testing.T) {
	pc := &ProfileController{}
	emitter := &interaction.NoopEmitter{}

	reg := interaction.NewModeMachineRegistry()
	reg.Register("code", func(emitter interaction.FrameEmitter, resolver *interaction.AgencyResolver) *interaction.PhaseMachine {
		return interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
			Mode:    "code",
			Emitter: emitter,
			Phases: []interaction.PhaseDefinition{
				{ID: "intent", Label: "Intent", Handler: &transitionHandler{targetMode: "debug"}},
			},
		})
	})
	reg.Register("debug", func(emitter interaction.FrameEmitter, resolver *interaction.AgencyResolver) *interaction.PhaseMachine {
		return interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
			Mode:    "debug",
			Emitter: emitter,
			Phases: []interaction.PhaseDefinition{
				{ID: "localize", Label: "Localize", Handler: &simpleHandler{stateKey: "localize.done", stateValue: true}},
			},
		})
	})

	mode := euclotypes.ModeResolution{ModeID: "code", Source: "test"}
	env := euclotypes.ExecutionEnvelope{
		Task:    &core.Task{ID: "t1"},
		Mode:    mode,
		Profile: euclotypes.ExecutionProfileSelection{ProfileID: "default"},
		State:   core.NewContext(),
	}

	result, icResult, err := pc.ExecuteInteractiveWithTransitions(ctx(), reg, mode, env, emitter, 3)
	if err != nil {
		t.Fatalf("ExecuteInteractiveWithTransitions: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	// Should have executed phases from both modes.
	if len(icResult.PhasesExecuted) < 2 {
		t.Errorf("phases: got %d, want >= 2", len(icResult.PhasesExecuted))
	}
}

func TestExecuteInteractiveWithTransitions_MaxExceeded(t *testing.T) {
	pc := &ProfileController{}
	emitter := &interaction.NoopEmitter{}

	// Both modes transition to each other, creating an infinite loop.
	reg := interaction.NewModeMachineRegistry()
	reg.Register("code", func(emitter interaction.FrameEmitter, resolver *interaction.AgencyResolver) *interaction.PhaseMachine {
		return interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
			Mode:    "code",
			Emitter: emitter,
			Phases: []interaction.PhaseDefinition{
				{ID: "a", Label: "A", Handler: &transitionHandler{targetMode: "debug"}},
			},
		})
	})
	reg.Register("debug", func(emitter interaction.FrameEmitter, resolver *interaction.AgencyResolver) *interaction.PhaseMachine {
		return interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
			Mode:    "debug",
			Emitter: emitter,
			Phases: []interaction.PhaseDefinition{
				{ID: "a", Label: "A", Handler: &transitionHandler{targetMode: "code"}},
			},
		})
	})

	mode := euclotypes.ModeResolution{ModeID: "code"}
	env := euclotypes.ExecutionEnvelope{
		Task:    &core.Task{ID: "t1"},
		Mode:    mode,
		Profile: euclotypes.ExecutionProfileSelection{ProfileID: "default"},
		State:   core.NewContext(),
	}

	_, _, err := pc.ExecuteInteractiveWithTransitions(ctx(), reg, mode, env, emitter, 2)
	if err == nil {
		t.Error("expected error for max transitions exceeded")
	}
}

func TestExecuteInteractive_NoopBackwardCompat(t *testing.T) {
	// Verify that NoopEmitter produces deterministic auto-advancing behavior
	// identical to what batch execution would produce.
	pc := &ProfileController{}
	registry := buildTestRegistry()
	emitter := &interaction.NoopEmitter{}

	mode := euclotypes.ModeResolution{ModeID: "code", Source: "batch"}
	env := euclotypes.ExecutionEnvelope{
		Task:    &core.Task{ID: "batch-1", Instruction: "batch task"},
		Mode:    mode,
		Profile: euclotypes.ExecutionProfileSelection{ProfileID: "default"},
		State:   core.NewContext(),
	}

	result, icResult, err := pc.ExecuteInteractive(ctx(), registry, mode, env, emitter)
	if err != nil {
		t.Fatalf("NoopEmitter execution: %v", err)
	}
	if !result.Success {
		t.Error("expected success with NoopEmitter")
	}
	// All phases should have executed (no interactive blocking).
	if len(icResult.PhasesExecuted) != 3 {
		t.Errorf("phases: got %d, want 3 (all phases auto-advanced)", len(icResult.PhasesExecuted))
	}
	// No frames should have blocked (NoopEmitter records but doesn't block).
	if icResult.TransitionTo != "" {
		t.Error("NoopEmitter should not trigger transitions")
	}
}

func ctx() context.Context {
	return context.Background()
}
