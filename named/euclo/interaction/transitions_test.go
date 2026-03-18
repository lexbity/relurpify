package interaction

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

func TestTransitionRuleSet_Evaluate(t *testing.T) {
	rs := NewTransitionRuleSet()
	rs.Add(TransitionRule{
		FromMode:    "code",
		ToMode:      "debug",
		Trigger:     TriggerVerificationFailure,
		Description: "code to debug",
	})
	rs.Add(TransitionRule{
		FromMode:    "code",
		ToMode:      "planning",
		Trigger:     TriggerScopeExpansion,
		Description: "code to planning",
	})

	// Match.
	rule := rs.Evaluate("code", TriggerVerificationFailure, nil, nil)
	if rule == nil {
		t.Fatal("expected match")
	}
	if rule.ToMode != "debug" {
		t.Errorf("to: got %q", rule.ToMode)
	}

	// No match for wrong trigger.
	rule = rs.Evaluate("code", TriggerUserRequest, nil, nil)
	if rule != nil {
		t.Error("expected no match")
	}

	// No match for wrong mode.
	rule = rs.Evaluate("debug", TriggerVerificationFailure, nil, nil)
	if rule != nil {
		t.Error("expected no match for wrong mode")
	}
}

func TestTransitionRuleSet_WildcardMode(t *testing.T) {
	rs := NewTransitionRuleSet()
	rs.Add(TransitionRule{
		FromMode: "*",
		ToMode:   "planning",
		Trigger:  TriggerUserRequest,
	})

	rule := rs.Evaluate("tdd", TriggerUserRequest, nil, nil)
	if rule == nil {
		t.Fatal("expected wildcard match")
	}
	if rule.ToMode != "planning" {
		t.Errorf("to: got %q", rule.ToMode)
	}
}

func TestTransitionRuleSet_Condition(t *testing.T) {
	rs := NewTransitionRuleSet()
	rs.Add(TransitionRule{
		FromMode: "code",
		ToMode:   "debug",
		Trigger:  TriggerVerificationFailure,
		Condition: func(state map[string]any, _ *ArtifactBundle) bool {
			count, _ := state["verify.failure_count"].(int)
			return count >= 2
		},
	})

	// Below threshold.
	state := map[string]any{"verify.failure_count": 1}
	rule := rs.Evaluate("code", TriggerVerificationFailure, state, nil)
	if rule != nil {
		t.Error("expected no match below threshold")
	}

	// At threshold.
	state["verify.failure_count"] = 2
	rule = rs.Evaluate("code", TriggerVerificationFailure, state, nil)
	if rule == nil {
		t.Fatal("expected match at threshold")
	}
}

func TestTransitionRuleSet_RulesFrom(t *testing.T) {
	rs := DefaultTransitionRules()
	codeRules := rs.RulesFrom("code")
	if len(codeRules) == 0 {
		t.Error("expected rules from code")
	}
	for _, r := range codeRules {
		if r.FromMode != "code" && r.FromMode != "*" {
			t.Errorf("unexpected from: %q", r.FromMode)
		}
	}
}

func TestTransitionRuleSet_RulesTo(t *testing.T) {
	rs := DefaultTransitionRules()
	toCode := rs.RulesTo("code")
	if len(toCode) == 0 {
		t.Error("expected rules targeting code")
	}
}

func TestDefaultTransitionRules_Canonical(t *testing.T) {
	rs := DefaultTransitionRules()
	all := rs.All()
	if len(all) < 9 {
		t.Errorf("expected at least 9 canonical rules, got %d", len(all))
	}

	// Verify specific rules exist.
	tests := []struct {
		from, to string
		trigger  TransitionTrigger
	}{
		{"code", "debug", TriggerVerificationFailure},
		{"code", "planning", TriggerScopeExpansion},
		{"code", "planning", TriggerUserRequest},
		{"debug", "code", TriggerUserRequest},
		{"debug", "code", TriggerPhaseCompletion},
		{"planning", "code", TriggerPhaseCompletion},
		{"planning", "code", TriggerUserRequest},
		{"tdd", "code", TriggerUserRequest},
		{"review", "code", TriggerUserRequest},
		{"*", "planning", TriggerUserRequest},
	}
	for _, tt := range tests {
		found := false
		for _, r := range all {
			if r.FromMode == tt.from && r.ToMode == tt.to && r.Trigger == tt.trigger {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing rule: %s → %s (%s)", tt.from, tt.to, tt.trigger)
		}
	}
}

func TestDefaultTransitionRules_CodeToDebugCondition(t *testing.T) {
	rs := DefaultTransitionRules()

	// Below threshold.
	state := map[string]any{"verify.failure_count": 1}
	rule := rs.Evaluate("code", TriggerVerificationFailure, state, nil)
	if rule != nil {
		t.Error("should not fire below threshold")
	}

	// At threshold.
	state["verify.failure_count"] = 2
	rule = rs.Evaluate("code", TriggerVerificationFailure, state, nil)
	if rule == nil {
		t.Fatal("should fire at threshold")
	}
	if rule.ToMode != "debug" {
		t.Errorf("to: got %q", rule.ToMode)
	}
	if len(rule.ArtifactCarry) != 3 {
		t.Errorf("carry: got %d, want 3", len(rule.ArtifactCarry))
	}
}

func TestDefaultTransitionRules_ScopeExpansionCondition(t *testing.T) {
	rs := DefaultTransitionRules()

	// Below scope threshold.
	state := map[string]any{
		"understand.proposal": ProposalContent{Scope: []string{"a.go", "b.go"}},
	}
	rule := rs.Evaluate("code", TriggerScopeExpansion, state, nil)
	if rule != nil {
		t.Error("should not fire below scope threshold")
	}

	// Above scope threshold.
	state["understand.proposal"] = ProposalContent{
		Scope: []string{"a.go", "b.go", "c.go", "d.go", "e.go", "f.go"},
	}
	rule = rs.Evaluate("code", TriggerScopeExpansion, state, nil)
	if rule == nil {
		t.Fatal("should fire above scope threshold")
	}
	if rule.ToMode != "planning" {
		t.Errorf("to: got %q", rule.ToMode)
	}
}

// ──────────────────────────────────────────────────────────────
// Transition stack tests
// ──────────────────────────────────────────────────────────────

func TestTransitionStack_PushPop(t *testing.T) {
	ts := NewTransitionStack()
	if !ts.IsEmpty() {
		t.Error("should be empty initially")
	}

	ts.Push(TransitionFrame{
		Mode:  "code",
		Phase: "verify",
		InteractionState: map[string]any{
			"verify.failure_count": 2,
		},
		ReturnArtifacts: []euclotypes.ArtifactKind{euclotypes.ArtifactKindPlan},
	})

	if ts.IsEmpty() {
		t.Error("should not be empty after push")
	}
	if ts.Depth() != 1 {
		t.Errorf("depth: got %d", ts.Depth())
	}

	top := ts.Peek()
	if top == nil || top.Mode != "code" {
		t.Error("peek should return code frame")
	}
	if ts.Depth() != 1 {
		t.Error("peek should not pop")
	}

	popped := ts.Pop()
	if popped == nil || popped.Mode != "code" {
		t.Error("pop should return code frame")
	}
	if !ts.IsEmpty() {
		t.Error("should be empty after pop")
	}

	// Pop on empty.
	if ts.Pop() != nil {
		t.Error("pop on empty should return nil")
	}
}

func TestTransitionStack_CollectReturnArtifacts(t *testing.T) {
	ts := NewTransitionStack()
	ts.Push(TransitionFrame{
		Mode:            "code",
		Phase:           "verify",
		ReturnArtifacts: []euclotypes.ArtifactKind{euclotypes.ArtifactKindPlan, euclotypes.ArtifactKindAnalyze},
	})

	bundle := NewArtifactBundle()
	bundle.Add(euclotypes.Artifact{Kind: euclotypes.ArtifactKindPlan, Summary: "plan"})
	bundle.Add(euclotypes.Artifact{Kind: euclotypes.ArtifactKindExplore, Summary: "explore"})
	bundle.Add(euclotypes.Artifact{Kind: euclotypes.ArtifactKindAnalyze, Summary: "analyze"})

	carried := ts.CollectReturnArtifacts(bundle)
	if len(carried) != 2 {
		t.Errorf("carried: got %d, want 2", len(carried))
	}
}

func TestTransitionStack_CollectReturnArtifacts_Empty(t *testing.T) {
	ts := NewTransitionStack()
	carried := ts.CollectReturnArtifacts(NewArtifactBundle())
	if len(carried) != 0 {
		t.Errorf("empty stack: got %d, want 0", len(carried))
	}
}

func TestTransitionStack_LIFO(t *testing.T) {
	ts := NewTransitionStack()
	ts.Push(TransitionFrame{Mode: "code", Phase: "verify"})
	ts.Push(TransitionFrame{Mode: "planning", Phase: "commit"})

	if ts.Depth() != 2 {
		t.Errorf("depth: got %d", ts.Depth())
	}

	top := ts.Pop()
	if top.Mode != "planning" {
		t.Errorf("first pop: got %q, want planning", top.Mode)
	}

	top = ts.Pop()
	if top.Mode != "code" {
		t.Errorf("second pop: got %q, want code", top.Mode)
	}
}

// ──────────────────────────────────────────────────────────────
// Integration: transition round-trip through machine
// ──────────────────────────────────────────────────────────────

func TestTransitionRoundTrip_CodeDebugCode(t *testing.T) {
	// Simulate: code mode triggers debug transition, debug completes,
	// return stack resumes code mode.
	rs := DefaultTransitionRules()
	ts := NewTransitionStack()

	// Phase 1: code mode runs, verification fails twice.
	codeState := map[string]any{
		"verify.failure_count": 2,
	}
	codeBundle := NewArtifactBundle()
	codeBundle.Add(euclotypes.Artifact{Kind: euclotypes.ArtifactKindExplore, Summary: "explored"})
	codeBundle.Add(euclotypes.Artifact{Kind: euclotypes.ArtifactKindEditIntent, Summary: "edit"})

	// Evaluate transition rule.
	rule := rs.Evaluate("code", TriggerVerificationFailure, codeState, codeBundle)
	if rule == nil {
		t.Fatal("expected code→debug rule")
	}
	if rule.ToMode != "debug" {
		t.Fatalf("to: got %q", rule.ToMode)
	}

	// Save code state for return.
	ts.Push(TransitionFrame{
		Mode:             "code",
		Phase:            "verify",
		InteractionState: codeState,
		ReturnArtifacts:  []euclotypes.ArtifactKind{euclotypes.ArtifactKindAnalyze},
	})

	// Carry artifacts to debug.
	carried := CarryOverArtifacts(codeBundle, "code", "debug")
	if len(carried) != 2 {
		t.Errorf("carry-over: got %d, want 2 (explore + edit_intent)", len(carried))
	}

	// Phase 2: debug mode runs and completes with analysis artifact.
	debugBundle := NewArtifactBundle()
	for _, a := range carried {
		debugBundle.Add(a)
	}
	debugBundle.Add(euclotypes.Artifact{Kind: euclotypes.ArtifactKindAnalyze, Summary: "root cause found"})

	// Phase 3: debug completes, check return stack.
	if ts.IsEmpty() {
		t.Fatal("return stack should not be empty")
	}

	returnArtifacts := ts.CollectReturnArtifacts(debugBundle)
	if len(returnArtifacts) != 1 {
		t.Errorf("return artifacts: got %d, want 1 (analyze)", len(returnArtifacts))
	}
	if returnArtifacts[0].Kind != euclotypes.ArtifactKindAnalyze {
		t.Errorf("return artifact kind: got %q", returnArtifacts[0].Kind)
	}

	returnFrame := ts.Pop()
	if returnFrame.Mode != "code" {
		t.Errorf("return mode: got %q", returnFrame.Mode)
	}
	if returnFrame.Phase != "verify" {
		t.Errorf("return phase: got %q", returnFrame.Phase)
	}
}

func TestTransitionRoundTrip_CodePlanningCode(t *testing.T) {
	rs := DefaultTransitionRules()
	ts := NewTransitionStack()

	// User requests planning.
	rule := rs.Evaluate("code", TriggerUserRequest, nil, nil)
	if rule == nil {
		t.Fatal("expected code→planning rule")
	}
	if rule.ToMode != "planning" {
		t.Fatalf("to: got %q", rule.ToMode)
	}

	ts.Push(TransitionFrame{
		Mode:            "code",
		Phase:           "intent",
		ReturnArtifacts: []euclotypes.ArtifactKind{euclotypes.ArtifactKindPlan},
	})

	// Planning completes with plan artifact.
	planBundle := NewArtifactBundle()
	planBundle.Add(euclotypes.Artifact{Kind: euclotypes.ArtifactKindPlan, Summary: "plan committed"})

	// Check return.
	returnArtifacts := ts.CollectReturnArtifacts(planBundle)
	if len(returnArtifacts) != 1 {
		t.Errorf("return: got %d", len(returnArtifacts))
	}
	returnFrame := ts.Pop()
	if returnFrame.Mode != "code" {
		t.Errorf("return mode: got %q", returnFrame.Mode)
	}
}

func TestTransitionRoundTrip_MachineIntegration(t *testing.T) {
	// Full machine-level test: code mode triggers transition to debug,
	// verify the machine state captures the transition decision.
	emitter := &NoopEmitter{}

	// Build a code machine that triggers a transition at verify phase.
	machine := NewPhaseMachine(PhaseMachineConfig{
		Mode:    "code",
		Emitter: emitter,
		Phases: []PhaseDefinition{
			{ID: "intent", Label: "Intent", Handler: PhaseHandlerFunc(func(_ context.Context, mc PhaseMachineContext) (PhaseOutcome, error) {
				return PhaseOutcome{Advance: true}, nil
			})},
			{ID: "verify", Label: "Verify", Handler: PhaseHandlerFunc(func(_ context.Context, mc PhaseMachineContext) (PhaseOutcome, error) {
				return PhaseOutcome{
					Advance:    true,
					Transition: "debug",
					StateUpdates: map[string]any{
						"verify.failure_count": 2,
						"verify.escalated":     true,
					},
				}, nil
			})},
		},
	})

	if err := machine.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	result := ExtractInteractionResult(machine)
	if result.TransitionTo != "debug" {
		t.Errorf("transition: got %q, want debug", result.TransitionTo)
	}

	// Verify the transition rules would match this state.
	rs := DefaultTransitionRules()
	rule := rs.Evaluate("code", TriggerVerificationFailure, machine.State(), machine.Artifacts())
	if rule == nil {
		t.Fatal("expected matching rule")
	}
	if rule.ToMode != "debug" {
		t.Errorf("rule to: got %q", rule.ToMode)
	}
}
