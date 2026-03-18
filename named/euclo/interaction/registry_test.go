package interaction

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

func TestModeMachineRegistry_Build(t *testing.T) {
	reg := NewModeMachineRegistry()
	built := false
	reg.Register("code", func(emitter FrameEmitter, resolver *AgencyResolver) *PhaseMachine {
		built = true
		return NewPhaseMachine(PhaseMachineConfig{
			Mode:    "code",
			Emitter: emitter,
		})
	})

	if !reg.Has("code") {
		t.Error("expected Has(code) to be true")
	}
	if reg.Has("debug") {
		t.Error("expected Has(debug) to be false")
	}

	emitter := &NoopEmitter{}
	m := reg.Build("code", emitter, nil)
	if m == nil {
		t.Fatal("expected machine")
	}
	if !built {
		t.Error("factory should have been called")
	}
}

func TestModeMachineRegistry_BuildUnregistered(t *testing.T) {
	reg := NewModeMachineRegistry()
	m := reg.Build("code", &NoopEmitter{}, nil)
	if m != nil {
		t.Error("expected nil for unregistered mode")
	}
}

func TestModeMachineRegistry_Modes(t *testing.T) {
	reg := NewModeMachineRegistry()
	reg.Register("code", func(FrameEmitter, *AgencyResolver) *PhaseMachine { return nil })
	reg.Register("debug", func(FrameEmitter, *AgencyResolver) *PhaseMachine { return nil })

	modes := reg.Modes()
	if len(modes) != 2 {
		t.Errorf("modes: got %d, want 2", len(modes))
	}
}

func TestInteractionState_Extract(t *testing.T) {
	emitter := &NoopEmitter{}
	m := NewPhaseMachine(PhaseMachineConfig{
		Mode:    "code",
		Emitter: emitter,
		Phases: []PhaseDefinition{
			{ID: "a", Label: "A", Handler: PhaseHandlerFunc(func(_ context.Context, mc PhaseMachineContext) (PhaseOutcome, error) {
				return PhaseOutcome{Advance: true, StateUpdates: map[string]any{"a.done": true}}, nil
			})},
			{ID: "b", Label: "B", Handler: PhaseHandlerFunc(func(_ context.Context, mc PhaseMachineContext) (PhaseOutcome, error) {
				return PhaseOutcome{Advance: true}, nil
			})},
		},
	})

	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	state := ExtractInteractionState(m)
	if state.Mode != "code" {
		t.Errorf("mode: got %q", state.Mode)
	}
	if v, _ := state.PhaseStates["a.done"].(bool); !v {
		t.Error("expected a.done in phase states")
	}
}

func TestInteractionResult_Extract(t *testing.T) {
	emitter := &NoopEmitter{}
	m := NewPhaseMachine(PhaseMachineConfig{
		Mode:    "code",
		Emitter: emitter,
		Phases: []PhaseDefinition{
			{ID: "a", Label: "A", Handler: PhaseHandlerFunc(func(_ context.Context, mc PhaseMachineContext) (PhaseOutcome, error) {
				return PhaseOutcome{
					Advance: true,
					Artifacts: []euclotypes.Artifact{
						{Kind: euclotypes.ArtifactKindExplore, Summary: "explored"},
					},
				}, nil
			})},
		},
	})

	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	result := ExtractInteractionResult(m)
	if len(result.Artifacts) != 1 {
		t.Errorf("artifacts: got %d, want 1", len(result.Artifacts))
	}
	if len(result.PhasesExecuted) != 1 {
		t.Errorf("phases: got %d, want 1", len(result.PhasesExecuted))
	}
}

func TestInteractionResult_Transition(t *testing.T) {
	emitter := &NoopEmitter{}
	m := NewPhaseMachine(PhaseMachineConfig{
		Mode:    "code",
		Emitter: emitter,
		Phases: []PhaseDefinition{
			{ID: "a", Label: "A", Handler: PhaseHandlerFunc(func(_ context.Context, mc PhaseMachineContext) (PhaseOutcome, error) {
				return PhaseOutcome{Advance: true, Transition: "debug"}, nil
			})},
		},
	})

	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	result := ExtractInteractionResult(m)
	// NoopEmitter auto-selects default "accept" for transitions.
	if result.TransitionTo != "debug" {
		t.Errorf("transition: got %q, want debug", result.TransitionTo)
	}
}

func TestCarryOverArtifacts(t *testing.T) {
	bundle := NewArtifactBundle()
	bundle.Add(euclotypes.Artifact{Kind: euclotypes.ArtifactKindExplore, Summary: "explore"})
	bundle.Add(euclotypes.Artifact{Kind: euclotypes.ArtifactKindEditIntent, Summary: "edit"})
	bundle.Add(euclotypes.Artifact{Kind: euclotypes.ArtifactKindPlan, Summary: "plan"})

	// code → debug should carry explore and edit_intent.
	carried := CarryOverArtifacts(bundle, "code", "debug")
	if len(carried) != 2 {
		t.Errorf("code→debug: got %d, want 2", len(carried))
	}

	// code → planning should carry only explore.
	carried = CarryOverArtifacts(bundle, "code", "planning")
	if len(carried) != 1 {
		t.Errorf("code→planning: got %d, want 1", len(carried))
	}

	// unknown transition carries nothing.
	carried = CarryOverArtifacts(bundle, "code", "unknown")
	if len(carried) != 0 {
		t.Errorf("code→unknown: got %d, want 0", len(carried))
	}

	// nil bundle carries nothing.
	carried = CarryOverArtifacts(nil, "code", "debug")
	if len(carried) != 0 {
		t.Errorf("nil bundle: got %d, want 0", len(carried))
	}
}
