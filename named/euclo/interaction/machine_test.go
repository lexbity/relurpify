package interaction

import (
	"context"
	"errors"
	"testing"

	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

// stubHandler is a test PhaseHandler that records calls and returns a configured outcome.
type stubHandler struct {
	called  int
	outcome PhaseOutcome
	err     error
}

func (h *stubHandler) Execute(_ context.Context, _ PhaseMachineContext) (PhaseOutcome, error) {
	h.called++
	return h.outcome, h.err
}

type cancelOnEmitEmitter struct {
	inner  FrameEmitter
	cancel context.CancelFunc
}

func (e *cancelOnEmitEmitter) Emit(ctx context.Context, frame InteractionFrame) error {
	if err := e.inner.Emit(ctx, frame); err != nil {
		return err
	}
	e.cancel()
	return nil
}

func (e *cancelOnEmitEmitter) AwaitResponse(ctx context.Context) (UserResponse, error) {
	return e.inner.AwaitResponse(ctx)
}

func TestPhaseMachine_LinearAdvancement(t *testing.T) {
	h1 := &stubHandler{outcome: PhaseOutcome{Advance: true}}
	h2 := &stubHandler{outcome: PhaseOutcome{Advance: true}}
	h3 := &stubHandler{outcome: PhaseOutcome{Advance: true}}

	m := NewPhaseMachine(PhaseMachineConfig{
		Mode: "test",
		Phases: []PhaseDefinition{
			{ID: "a", Label: "Phase A", Handler: h1},
			{ID: "b", Label: "Phase B", Handler: h2},
			{ID: "c", Label: "Phase C", Handler: h3},
		},
		Emitter: &NoopEmitter{},
	})

	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if h1.called != 1 || h2.called != 1 || h3.called != 1 {
		t.Errorf("calls: h1=%d h2=%d h3=%d, want 1,1,1", h1.called, h2.called, h3.called)
	}
}

func TestPhaseMachine_StopOnNoAdvance(t *testing.T) {
	h1 := &stubHandler{outcome: PhaseOutcome{Advance: true}}
	h2 := &stubHandler{outcome: PhaseOutcome{Advance: false}} // terminal
	h3 := &stubHandler{outcome: PhaseOutcome{Advance: true}}

	m := NewPhaseMachine(PhaseMachineConfig{
		Mode: "test",
		Phases: []PhaseDefinition{
			{ID: "a", Handler: h1},
			{ID: "b", Handler: h2},
			{ID: "c", Handler: h3},
		},
		Emitter: &NoopEmitter{},
	})

	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if h3.called != 0 {
		t.Error("phase C should not have been called")
	}
}

func TestPhaseMachine_SkipWhen(t *testing.T) {
	h1 := &stubHandler{outcome: PhaseOutcome{Advance: true}}
	h2 := &stubHandler{outcome: PhaseOutcome{Advance: true}}
	h3 := &stubHandler{outcome: PhaseOutcome{Advance: true}}

	m := NewPhaseMachine(PhaseMachineConfig{
		Mode: "test",
		Phases: []PhaseDefinition{
			{ID: "a", Handler: h1},
			{ID: "b", Handler: h2, Skippable: true, SkipWhen: func(map[string]any, *ArtifactBundle) bool {
				return true // always skip
			}},
			{ID: "c", Handler: h3},
		},
		Emitter: &NoopEmitter{},
	})

	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if h2.called != 0 {
		t.Error("phase B should have been skipped")
	}
	if h1.called != 1 || h3.called != 1 {
		t.Error("phases A and C should have been called")
	}
}

func TestPhaseMachine_SkipWhen_StateBased(t *testing.T) {
	h1 := &stubHandler{outcome: PhaseOutcome{
		Advance:      true,
		StateUpdates: map[string]any{"skip_b": true},
	}}
	h2 := &stubHandler{outcome: PhaseOutcome{Advance: true}}
	h3 := &stubHandler{outcome: PhaseOutcome{Advance: true}}

	m := NewPhaseMachine(PhaseMachineConfig{
		Mode: "test",
		Phases: []PhaseDefinition{
			{ID: "a", Handler: h1},
			{ID: "b", Handler: h2, SkipWhen: func(state map[string]any, _ *ArtifactBundle) bool {
				v, _ := state["skip_b"].(bool)
				return v
			}},
			{ID: "c", Handler: h3},
		},
		Emitter: &NoopEmitter{},
	})

	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if h2.called != 0 {
		t.Error("phase B should have been skipped based on state from A")
	}
}

func TestPhaseMachine_EnterGuard_Blocks(t *testing.T) {
	h1 := &stubHandler{outcome: PhaseOutcome{Advance: true}}
	h2 := &stubHandler{outcome: PhaseOutcome{Advance: true}}

	guardErr := errors.New("precondition failed")

	m := NewPhaseMachine(PhaseMachineConfig{
		Mode: "test",
		Phases: []PhaseDefinition{
			{ID: "a", Handler: h1},
			{ID: "b", Handler: h2, EnterGuard: func(map[string]any, *ArtifactBundle) error {
				return guardErr
			}},
		},
		Emitter: &NoopEmitter{},
	})

	err := m.Run(context.Background())
	if err == nil {
		t.Fatal("expected error from guard")
	}
	if !errors.Is(err, guardErr) {
		t.Errorf("expected guard error, got: %v", err)
	}
	if h2.called != 0 {
		t.Error("phase B handler should not have been called")
	}
}

func TestPhaseMachine_EnterGuard_Passes(t *testing.T) {
	h1 := &stubHandler{outcome: PhaseOutcome{Advance: true}}

	m := NewPhaseMachine(PhaseMachineConfig{
		Mode: "test",
		Phases: []PhaseDefinition{
			{ID: "a", Handler: h1, EnterGuard: func(map[string]any, *ArtifactBundle) error {
				return nil
			}},
		},
		Emitter: &NoopEmitter{},
	})

	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if h1.called != 1 {
		t.Error("phase A should have been called")
	}
}

func TestPhaseMachine_JumpTo(t *testing.T) {
	h1 := &stubHandler{outcome: PhaseOutcome{Advance: true, JumpTo: "c"}}
	h2 := &stubHandler{outcome: PhaseOutcome{Advance: true}}
	h3 := &stubHandler{outcome: PhaseOutcome{Advance: true}}

	m := NewPhaseMachine(PhaseMachineConfig{
		Mode: "test",
		Phases: []PhaseDefinition{
			{ID: "a", Handler: h1},
			{ID: "b", Handler: h2},
			{ID: "c", Handler: h3},
		},
		Emitter: &NoopEmitter{},
	})

	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if h2.called != 0 {
		t.Error("phase B should have been skipped by jump")
	}
	if h3.called != 1 {
		t.Error("phase C should have been called after jump")
	}
}

func TestPhaseMachine_JumpTo_InvalidTarget(t *testing.T) {
	h1 := &stubHandler{outcome: PhaseOutcome{Advance: true, JumpTo: "nonexistent"}}

	m := NewPhaseMachine(PhaseMachineConfig{
		Mode: "test",
		Phases: []PhaseDefinition{
			{ID: "a", Handler: h1},
		},
		Emitter: &NoopEmitter{},
	})

	err := m.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid jump target")
	}
}

func TestPhaseMachine_JumpTo_Backward(t *testing.T) {
	callCount := 0
	// Phase A jumps to itself once, then advances.
	hA := &stubHandler{}
	hA.outcome = PhaseOutcome{Advance: true, JumpTo: "a"}

	m := NewPhaseMachine(PhaseMachineConfig{
		Mode: "test",
		Phases: []PhaseDefinition{
			{ID: "a", Handler: PhaseHandlerFunc(func(_ context.Context, _ PhaseMachineContext) (PhaseOutcome, error) {
				callCount++
				if callCount >= 3 {
					return PhaseOutcome{Advance: true}, nil // stop looping
				}
				return PhaseOutcome{Advance: true, JumpTo: "a"}, nil
			})},
			{ID: "b", Handler: &stubHandler{outcome: PhaseOutcome{Advance: true}}},
		},
		Emitter: &NoopEmitter{},
	})

	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if callCount != 3 {
		t.Errorf("phase A called %d times, want 3", callCount)
	}
}

func TestPhaseMachine_Transition(t *testing.T) {
	h1 := &stubHandler{outcome: PhaseOutcome{Advance: true, Transition: "debug"}}
	h2 := &stubHandler{outcome: PhaseOutcome{Advance: true}}

	emitter := &NoopEmitter{}
	m := NewPhaseMachine(PhaseMachineConfig{
		Mode: "code",
		Phases: []PhaseDefinition{
			{ID: "verify", Handler: h1},
			{ID: "present", Handler: h2},
		},
		Emitter: emitter,
	})

	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// NoopEmitter accepts default (accept), so transition should be recorded.
	if m.State()["transition.accepted"] != "debug" {
		t.Errorf("expected transition.accepted=debug, got %v", m.State()["transition.accepted"])
	}

	// Should have emitted a transition frame.
	var foundTransition bool
	for _, f := range emitter.Frames {
		if f.Kind == FrameTransition {
			foundTransition = true
			break
		}
	}
	if !foundTransition {
		t.Error("expected a transition frame to be emitted")
	}
}

func TestPhaseMachine_ArtifactMerge(t *testing.T) {
	h1 := &stubHandler{outcome: PhaseOutcome{
		Advance: true,
		Artifacts: []euclotypes.Artifact{
			{ID: "art-1", Kind: euclotypes.ArtifactKindExplore, Summary: "explored files"},
		},
	}}
	h2 := &stubHandler{outcome: PhaseOutcome{
		Advance: true,
		Artifacts: []euclotypes.Artifact{
			{ID: "art-2", Kind: euclotypes.ArtifactKindPlan, Summary: "plan created"},
		},
	}}

	m := NewPhaseMachine(PhaseMachineConfig{
		Mode: "test",
		Phases: []PhaseDefinition{
			{ID: "a", Handler: h1},
			{ID: "b", Handler: h2},
		},
		Emitter: &NoopEmitter{},
	})

	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	all := m.Artifacts().All()
	if len(all) != 2 {
		t.Fatalf("artifacts: got %d, want 2", len(all))
	}
	if !m.Artifacts().Has(euclotypes.ArtifactKindExplore) {
		t.Error("missing explore artifact")
	}
	if !m.Artifacts().Has(euclotypes.ArtifactKindPlan) {
		t.Error("missing plan artifact")
	}
}

func TestPhaseMachine_StateMerge(t *testing.T) {
	h1 := &stubHandler{outcome: PhaseOutcome{
		Advance:      true,
		StateUpdates: map[string]any{"key1": "value1"},
	}}
	h2 := &stubHandler{outcome: PhaseOutcome{
		Advance:      true,
		StateUpdates: map[string]any{"key2": "value2"},
	}}

	m := NewPhaseMachine(PhaseMachineConfig{
		Mode: "test",
		Phases: []PhaseDefinition{
			{ID: "a", Handler: h1},
			{ID: "b", Handler: h2},
		},
		Emitter: &NoopEmitter{},
	})

	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if m.State()["key1"] != "value1" {
		t.Errorf("key1: got %v, want value1", m.State()["key1"])
	}
	if m.State()["key2"] != "value2" {
		t.Errorf("key2: got %v, want value2", m.State()["key2"])
	}
}

func TestPhaseMachine_HandlerError(t *testing.T) {
	handlerErr := errors.New("handler failed")
	h1 := &stubHandler{outcome: PhaseOutcome{Advance: true}, err: handlerErr}

	m := NewPhaseMachine(PhaseMachineConfig{
		Mode: "test",
		Phases: []PhaseDefinition{
			{ID: "a", Handler: h1},
		},
		Emitter: &NoopEmitter{},
	})

	err := m.Run(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, handlerErr) {
		t.Errorf("expected handler error, got: %v", err)
	}
}

func TestPhaseMachine_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	h1 := &stubHandler{outcome: PhaseOutcome{Advance: true}}

	m := NewPhaseMachine(PhaseMachineConfig{
		Mode: "test",
		Phases: []PhaseDefinition{
			{ID: "a", Handler: h1},
		},
		Emitter: &NoopEmitter{},
	})

	err := m.Run(ctx)
	if err == nil {
		t.Fatal("expected context error")
	}
	if h1.called != 0 {
		t.Error("handler should not have been called after cancellation")
	}
}

func TestPhaseMachine_TransitionRespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	h1 := &stubHandler{outcome: PhaseOutcome{Advance: true, Transition: "debug"}}
	m := NewPhaseMachine(PhaseMachineConfig{
		Mode: "code",
		Phases: []PhaseDefinition{
			{ID: "verify", Handler: h1},
		},
		Emitter: &cancelOnEmitEmitter{
			inner:  &NoopEmitter{},
			cancel: cancel,
		},
	})

	err := m.Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if _, ok := m.State()["transition.accepted"]; ok {
		t.Fatal("transition should not be accepted after context cancellation")
	}
	if h1.called != 1 {
		t.Fatalf("expected transition handler to run once, got %d", h1.called)
	}
}

func TestPhaseMachine_CurrentPhase(t *testing.T) {
	m := NewPhaseMachine(PhaseMachineConfig{
		Mode: "test",
		Phases: []PhaseDefinition{
			{ID: "first", Handler: &stubHandler{outcome: PhaseOutcome{Advance: true}}},
			{ID: "second", Handler: &stubHandler{outcome: PhaseOutcome{Advance: true}}},
		},
		Emitter: &NoopEmitter{},
	})

	if m.CurrentPhase() != "first" {
		t.Errorf("before run: got %q, want 'first'", m.CurrentPhase())
	}

	_ = m.Run(context.Background())

	if m.CurrentPhase() != "" {
		t.Errorf("after run: got %q, want empty", m.CurrentPhase())
	}
}

func TestPhaseMachine_EmptyPhases(t *testing.T) {
	m := NewPhaseMachine(PhaseMachineConfig{
		Mode:    "test",
		Phases:  nil,
		Emitter: &NoopEmitter{},
	})

	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestArtifactBundle(t *testing.T) {
	b := NewArtifactBundle()

	if b.Has(euclotypes.ArtifactKindExplore) {
		t.Error("empty bundle should not have explore")
	}
	if len(b.All()) != 0 {
		t.Error("empty bundle should have 0 artifacts")
	}

	b.Add(euclotypes.Artifact{ID: "1", Kind: euclotypes.ArtifactKindExplore})
	b.Add(euclotypes.Artifact{ID: "2", Kind: euclotypes.ArtifactKindPlan})
	b.Add(euclotypes.Artifact{ID: "3", Kind: euclotypes.ArtifactKindExplore})

	if len(b.All()) != 3 {
		t.Errorf("all: got %d, want 3", len(b.All()))
	}
	if !b.Has(euclotypes.ArtifactKindExplore) {
		t.Error("should have explore")
	}
	if !b.Has(euclotypes.ArtifactKindPlan) {
		t.Error("should have plan")
	}
	if b.Has(euclotypes.ArtifactKindVerification) {
		t.Error("should not have verification")
	}

	explores := b.OfKind(euclotypes.ArtifactKindExplore)
	if len(explores) != 2 {
		t.Errorf("explore artifacts: got %d, want 2", len(explores))
	}
}

// PhaseHandlerFunc adapts a function to the PhaseHandler interface.
type PhaseHandlerFunc func(ctx context.Context, mc PhaseMachineContext) (PhaseOutcome, error)

func (f PhaseHandlerFunc) Execute(ctx context.Context, mc PhaseMachineContext) (PhaseOutcome, error) {
	return f(ctx, mc)
}
