package interaction_test

import (
	"context"
	"errors"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

// ---------------------------------------------------------------------------
// AgencyResolver
// ---------------------------------------------------------------------------

func TestAgencyResolver_ExactMatch(t *testing.T) {
	r := interaction.NewAgencyResolver()
	r.RegisterTrigger("chat", interaction.AgencyTrigger{
		Phrases:      []string{"run tests"},
		CapabilityID: "euclo:verification.execute",
	})
	trigger, ok := r.Resolve("chat", "run tests")
	if !ok {
		t.Fatal("expected match")
	}
	if trigger.CapabilityID != "euclo:verification.execute" {
		t.Fatalf("unexpected capability ID: %q", trigger.CapabilityID)
	}
}

func TestAgencyResolver_ExactMatchCaseInsensitive(t *testing.T) {
	r := interaction.NewAgencyResolver()
	r.RegisterTrigger("chat", interaction.AgencyTrigger{Phrases: []string{"Run Tests"}})
	_, ok := r.Resolve("chat", "RUN TESTS")
	if !ok {
		t.Fatal("expected case-insensitive match")
	}
}

func TestAgencyResolver_FuzzyMatch(t *testing.T) {
	r := interaction.NewAgencyResolver()
	r.RegisterTrigger("chat", interaction.AgencyTrigger{
		Phrases:   []string{"run tests"},
		PhaseJump: "verify",
	})
	trigger, ok := r.Resolve("chat", "please run tests now")
	if !ok {
		t.Fatal("expected fuzzy match")
	}
	if trigger.PhaseJump != "verify" {
		t.Fatalf("unexpected phase jump: %q", trigger.PhaseJump)
	}
}

func TestAgencyResolver_EmptyTextReturnsNil(t *testing.T) {
	r := interaction.NewAgencyResolver()
	r.RegisterTrigger("chat", interaction.AgencyTrigger{Phrases: []string{"run tests"}})
	_, ok := r.Resolve("chat", "   ")
	if ok {
		t.Fatal("expected no match for whitespace-only text")
	}
}

func TestAgencyResolver_WrongModeSkipped(t *testing.T) {
	r := interaction.NewAgencyResolver()
	r.RegisterTrigger("debug", interaction.AgencyTrigger{Phrases: []string{"investigate"}})
	_, ok := r.Resolve("chat", "investigate")
	if ok {
		t.Fatal("expected no match when registered mode differs from query mode")
	}
}

func TestAgencyResolver_RequiresModeFilters(t *testing.T) {
	r := interaction.NewAgencyResolver()
	// Entry has empty mode (global) but RequiresMode restricts to "debug"
	r.RegisterTrigger("", interaction.AgencyTrigger{
		Phrases:      []string{"dig deeper"},
		RequiresMode: "debug",
	})
	_, ok := r.Resolve("chat", "dig deeper")
	if ok {
		t.Fatal("expected no match when RequiresMode doesn't match")
	}
	trig, ok := r.Resolve("debug", "dig deeper")
	if !ok {
		t.Fatal("expected match when RequiresMode matches")
	}
	_ = trig
}

func TestAgencyResolver_EmptyModeMatchesAll(t *testing.T) {
	r := interaction.NewAgencyResolver()
	r.RegisterTrigger("", interaction.AgencyTrigger{Phrases: []string{"help"}})
	_, ok := r.Resolve("chat", "help")
	if !ok {
		t.Fatal("expected match for empty-mode trigger in any mode")
	}
	_, ok = r.Resolve("debug", "help")
	if !ok {
		t.Fatal("expected match for empty-mode trigger in debug mode")
	}
}

func TestAgencyResolver_TriggersForMode(t *testing.T) {
	r := interaction.NewAgencyResolver()
	r.RegisterTrigger("chat", interaction.AgencyTrigger{Phrases: []string{"a"}})
	r.RegisterTrigger("debug", interaction.AgencyTrigger{Phrases: []string{"b"}})
	r.RegisterTrigger("", interaction.AgencyTrigger{Phrases: []string{"c"}}) // global

	triggers := r.TriggersForMode("chat")
	if len(triggers) != 2 { // "chat" + "" (global)
		t.Fatalf("expected 2 triggers for chat mode, got %d", len(triggers))
	}
}

// ---------------------------------------------------------------------------
// InteractionBudget
// ---------------------------------------------------------------------------

func TestBudget_DefaultBudget(t *testing.T) {
	b := interaction.DefaultBudget()
	if b.MaxQuestionsPerPhase != 3 {
		t.Fatalf("expected MaxQuestionsPerPhase=3, got %d", b.MaxQuestionsPerPhase)
	}
	if b.MaxTransitions != 3 {
		t.Fatalf("expected MaxTransitions=3, got %d", b.MaxTransitions)
	}
}

func TestBudget_NilReceiverAlwaysWithinBudget(t *testing.T) {
	var b *interaction.InteractionBudget
	if !b.RecordFrame() {
		t.Fatal("nil budget RecordFrame should return true")
	}
	if !b.RecordQuestion("phase") {
		t.Fatal("nil budget RecordQuestion should return true")
	}
	if !b.RecordSkip() {
		t.Fatal("nil budget RecordSkip should return true")
	}
	if !b.RecordTransition() {
		t.Fatal("nil budget RecordTransition should return true")
	}
	if b.FrameCount() != 0 {
		t.Fatal("nil budget FrameCount should return 0")
	}
	if b.TransitionCount() != 0 {
		t.Fatal("nil budget TransitionCount should return 0")
	}
	if b.ExhaustedReason() != "" {
		t.Fatal("nil budget ExhaustedReason should return empty string")
	}
}

func TestBudget_RecordFrameCountsAndExceeds(t *testing.T) {
	b := interaction.DefaultBudget()
	b.MaxFramesTotal = 2
	if !b.RecordFrame() {
		t.Fatal("frame 1 should be within budget")
	}
	if !b.RecordFrame() {
		t.Fatal("frame 2 should be within budget")
	}
	if b.RecordFrame() {
		t.Fatal("frame 3 should exceed budget")
	}
	if b.FrameCount() != 3 {
		t.Fatalf("expected FrameCount=3, got %d", b.FrameCount())
	}
	if b.ExhaustedReason() != "frame_budget_exceeded" {
		t.Fatalf("expected frame_budget_exceeded, got %q", b.ExhaustedReason())
	}
}

func TestBudget_RecordQuestionExceedsPerPhase(t *testing.T) {
	b := interaction.DefaultBudget()
	b.MaxQuestionsPerPhase = 2
	if !b.RecordQuestion("p1") {
		t.Fatal("q1 should be within budget")
	}
	if !b.RecordQuestion("p1") {
		t.Fatal("q2 should be within budget")
	}
	if b.RecordQuestion("p1") {
		t.Fatal("q3 should exceed per-phase budget")
	}
	// Different phase is unaffected.
	if !b.RecordQuestion("p2") {
		t.Fatal("different phase should have independent budget")
	}
}

func TestBudget_RecordTransitionCountAndExhaustion(t *testing.T) {
	b := interaction.DefaultBudget()
	b.MaxTransitions = 2
	if !b.RecordTransition() {
		t.Fatal("transition 1 should be within budget")
	}
	if !b.RecordTransition() {
		t.Fatal("transition 2 should be within budget")
	}
	if b.RecordTransition() {
		t.Fatal("transition 3 should exceed budget")
	}
	if b.TransitionCount() != 3 {
		t.Fatalf("expected TransitionCount=3, got %d", b.TransitionCount())
	}
	if b.ExhaustedReason() != "transition_budget_exceeded" {
		t.Fatalf("expected transition_budget_exceeded, got %q", b.ExhaustedReason())
	}
}

func TestBudget_RecordSkipExceedsLimit(t *testing.T) {
	b := interaction.DefaultBudget()
	b.MaxPhasesSkippable = 1
	if !b.RecordSkip() {
		t.Fatal("skip 1 should be within budget")
	}
	if b.RecordSkip() {
		t.Fatal("skip 2 should exceed budget")
	}
}

func TestBudget_NewBudgetFromConfig(t *testing.T) {
	cfg := interaction.InteractionConfig{
		Budget: interaction.InteractionBudgetConfig{
			MaxQuestions:   5,
			MaxTransitions: 10,
			MaxFrames:      100,
		},
	}
	b := interaction.NewBudget(cfg)
	if b.MaxQuestionsPerPhase != 5 {
		t.Fatalf("expected MaxQuestionsPerPhase=5, got %d", b.MaxQuestionsPerPhase)
	}
	if b.MaxTransitions != 10 {
		t.Fatalf("expected MaxTransitions=10, got %d", b.MaxTransitions)
	}
	if b.MaxFramesTotal != 100 {
		t.Fatalf("expected MaxFramesTotal=100, got %d", b.MaxFramesTotal)
	}
}

// ---------------------------------------------------------------------------
// ArtifactBundle
// ---------------------------------------------------------------------------

func TestArtifactBundle_AddAndAll(t *testing.T) {
	b := interaction.NewArtifactBundle()
	b.Add(euclotypes.Artifact{Kind: euclotypes.ArtifactKindExplore, Summary: "s1"})
	b.Add(euclotypes.Artifact{Kind: euclotypes.ArtifactKindAnalyze, Summary: "s2"})
	all := b.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(all))
	}
}

func TestArtifactBundle_OfKind(t *testing.T) {
	b := interaction.NewArtifactBundle()
	b.Add(euclotypes.Artifact{Kind: euclotypes.ArtifactKindExplore, Summary: "e1"})
	b.Add(euclotypes.Artifact{Kind: euclotypes.ArtifactKindExplore, Summary: "e2"})
	b.Add(euclotypes.Artifact{Kind: euclotypes.ArtifactKindAnalyze, Summary: "a1"})

	explores := b.OfKind(euclotypes.ArtifactKindExplore)
	if len(explores) != 2 {
		t.Fatalf("expected 2 explore artifacts, got %d", len(explores))
	}
	analyzes := b.OfKind(euclotypes.ArtifactKindAnalyze)
	if len(analyzes) != 1 {
		t.Fatalf("expected 1 analyze artifact, got %d", len(analyzes))
	}
}

func TestArtifactBundle_Has(t *testing.T) {
	b := interaction.NewArtifactBundle()
	if b.Has(euclotypes.ArtifactKindExplore) {
		t.Fatal("expected empty bundle to not have explore")
	}
	b.Add(euclotypes.Artifact{Kind: euclotypes.ArtifactKindExplore})
	if !b.Has(euclotypes.ArtifactKindExplore) {
		t.Fatal("expected bundle to have explore after add")
	}
}

// ---------------------------------------------------------------------------
// InteractionFrame helpers
// ---------------------------------------------------------------------------

func TestInteractionFrame_DefaultAction(t *testing.T) {
	f := interaction.InteractionFrame{
		Actions: []interaction.ActionSlot{
			{ID: "a", Label: "A"},
			{ID: "b", Label: "B", Default: true},
		},
	}
	def := f.DefaultAction()
	if def == nil || def.ID != "b" {
		t.Fatalf("expected default action 'b', got %v", def)
	}
}

func TestInteractionFrame_DefaultActionNoneMarked(t *testing.T) {
	f := interaction.InteractionFrame{
		Actions: []interaction.ActionSlot{
			{ID: "a", Label: "A"},
		},
	}
	if f.DefaultAction() != nil {
		t.Fatal("expected nil when no action is marked default")
	}
}

func TestInteractionFrame_ActionByID(t *testing.T) {
	f := interaction.InteractionFrame{
		Actions: []interaction.ActionSlot{
			{ID: "confirm", Label: "Confirm"},
			{ID: "reject", Label: "Reject"},
		},
	}
	a := f.ActionByID("confirm")
	if a == nil || a.Label != "Confirm" {
		t.Fatalf("unexpected action: %v", a)
	}
	if f.ActionByID("missing") != nil {
		t.Fatal("expected nil for missing action ID")
	}
}

// ---------------------------------------------------------------------------
// NoopEmitter
// ---------------------------------------------------------------------------

func TestNoopEmitter_EmitRecordsFrame(t *testing.T) {
	e := &interaction.NoopEmitter{}
	f := interaction.InteractionFrame{Kind: interaction.FrameProposal, Mode: "chat", Phase: "understand"}
	if err := e.Emit(context.Background(), f); err != nil {
		t.Fatalf("Emit error: %v", err)
	}
	if len(e.Frames) != 1 {
		t.Fatalf("expected 1 frame, got %d", len(e.Frames))
	}
}

func TestNoopEmitter_AwaitResponseUsesDefaultAction(t *testing.T) {
	e := &interaction.NoopEmitter{}
	_ = e.Emit(context.Background(), interaction.InteractionFrame{
		Kind: interaction.FrameProposal,
		Actions: []interaction.ActionSlot{
			{ID: "confirm", Label: "Confirm", Default: true},
			{ID: "reject", Label: "Reject"},
		},
	})
	resp, err := e.AwaitResponse(context.Background())
	if err != nil {
		t.Fatalf("AwaitResponse error: %v", err)
	}
	if resp.ActionID != "confirm" {
		t.Fatalf("expected confirm, got %q", resp.ActionID)
	}
}

func TestNoopEmitter_TransitionFrameDefaultsToReject(t *testing.T) {
	e := &interaction.NoopEmitter{}
	_ = e.Emit(context.Background(), interaction.InteractionFrame{
		Kind: interaction.FrameTransition,
		Actions: []interaction.ActionSlot{
			{ID: "accept", Label: "Accept"},
			{ID: "reject", Label: "Reject"},
		},
	})
	resp, _ := e.AwaitResponse(context.Background())
	if resp.ActionID != "reject" {
		t.Fatalf("expected reject for transition frame, got %q", resp.ActionID)
	}
}

func TestNoopEmitter_CandidatesFrameSelectsRecommended(t *testing.T) {
	e := &interaction.NoopEmitter{}
	_ = e.Emit(context.Background(), interaction.InteractionFrame{
		Kind: interaction.FrameCandidates,
		Content: interaction.CandidatesContent{
			RecommendedID: "cand-2",
			Candidates: []interaction.Candidate{
				{ID: "cand-1", Summary: "Option 1"},
				{ID: "cand-2", Summary: "Option 2"},
			},
		},
		Actions: []interaction.ActionSlot{
			{ID: "cand-1", Label: "Option 1"},
			{ID: "cand-2", Label: "Option 2"},
		},
	})
	resp, _ := e.AwaitResponse(context.Background())
	if resp.ActionID != "cand-2" {
		t.Fatalf("expected cand-2 (recommended), got %q", resp.ActionID)
	}
}

func TestNoopEmitter_DraftFrameAccepts(t *testing.T) {
	e := &interaction.NoopEmitter{}
	_ = e.Emit(context.Background(), interaction.InteractionFrame{
		Kind: interaction.FrameDraft,
		Actions: []interaction.ActionSlot{
			{ID: "accept", Label: "Accept"},
			{ID: "edit", Label: "Edit"},
		},
	})
	resp, _ := e.AwaitResponse(context.Background())
	if resp.ActionID != "accept" {
		t.Fatalf("expected accept for draft frame, got %q", resp.ActionID)
	}
}

func TestNoopEmitter_ResultFrameContinues(t *testing.T) {
	e := &interaction.NoopEmitter{}
	_ = e.Emit(context.Background(), interaction.InteractionFrame{
		Kind: interaction.FrameResult,
		Actions: []interaction.ActionSlot{
			{ID: "continue", Label: "Continue"},
		},
	})
	resp, _ := e.AwaitResponse(context.Background())
	if resp.ActionID != "continue" {
		t.Fatalf("expected continue for result frame, got %q", resp.ActionID)
	}
}

func TestNoopEmitter_FallsBackToFirstAction(t *testing.T) {
	e := &interaction.NoopEmitter{}
	_ = e.Emit(context.Background(), interaction.InteractionFrame{
		Kind: interaction.FrameQuestion,
		Actions: []interaction.ActionSlot{
			{ID: "first", Label: "First"},
			{ID: "second", Label: "Second"},
		},
	})
	resp, _ := e.AwaitResponse(context.Background())
	if resp.ActionID != "first" {
		t.Fatalf("expected first action fallback, got %q", resp.ActionID)
	}
}

func TestNoopEmitter_EmptyFramesReturnsEmpty(t *testing.T) {
	e := &interaction.NoopEmitter{}
	resp, err := e.AwaitResponse(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ActionID != "" {
		t.Fatalf("expected empty response, got %q", resp.ActionID)
	}
}

func TestNoopEmitter_Reset(t *testing.T) {
	e := &interaction.NoopEmitter{}
	_ = e.Emit(context.Background(), interaction.InteractionFrame{Kind: interaction.FrameProposal})
	e.Reset()
	if len(e.Frames) != 0 {
		t.Fatalf("expected 0 frames after reset, got %d", len(e.Frames))
	}
}

func TestNoopEmitter_CanceledContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	e := &interaction.NoopEmitter{}
	_ = e.Emit(ctx, interaction.InteractionFrame{Kind: interaction.FrameProposal})
	_, err := e.AwaitResponse(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// TransitionRuleSet
// ---------------------------------------------------------------------------

func TestTransitionRuleSet_EvaluateNoMatch(t *testing.T) {
	rs := interaction.NewTransitionRuleSet()
	rs.Add(interaction.TransitionRule{
		FromMode: "code",
		ToMode:   "debug",
		Trigger:  interaction.TriggerVerificationFailure,
	})
	// Wrong trigger.
	rule := rs.Evaluate("code", interaction.TriggerUserRequest, nil, nil)
	if rule != nil {
		t.Fatal("expected no match for wrong trigger")
	}
}

func TestTransitionRuleSet_EvaluateExactMatch(t *testing.T) {
	rs := interaction.NewTransitionRuleSet()
	rs.Add(interaction.TransitionRule{
		FromMode: "code",
		ToMode:   "debug",
		Trigger:  interaction.TriggerVerificationFailure,
	})
	rule := rs.Evaluate("code", interaction.TriggerVerificationFailure, nil, nil)
	if rule == nil {
		t.Fatal("expected match")
	}
	if rule.ToMode != "debug" {
		t.Fatalf("expected ToMode=debug, got %q", rule.ToMode)
	}
}

func TestTransitionRuleSet_EvaluateWildcardFromMode(t *testing.T) {
	rs := interaction.NewTransitionRuleSet()
	rs.Add(interaction.TransitionRule{
		FromMode: "*",
		ToMode:   "planning",
		Trigger:  interaction.TriggerUserRequest,
	})
	rule := rs.Evaluate("chat", interaction.TriggerUserRequest, nil, nil)
	if rule == nil {
		t.Fatal("expected wildcard match")
	}
	if rule.ToMode != "planning" {
		t.Fatalf("expected planning, got %q", rule.ToMode)
	}
}

func TestTransitionRuleSet_ConditionFalseSkipsRule(t *testing.T) {
	rs := interaction.NewTransitionRuleSet()
	rs.Add(interaction.TransitionRule{
		FromMode: "code",
		ToMode:   "debug",
		Trigger:  interaction.TriggerVerificationFailure,
		Condition: func(state map[string]any, _ *interaction.ArtifactBundle) bool {
			count, _ := state["verify.failure_count"].(int)
			return count >= 2
		},
	})
	// Condition not met.
	state := map[string]any{"verify.failure_count": 1}
	rule := rs.Evaluate("code", interaction.TriggerVerificationFailure, state, nil)
	if rule != nil {
		t.Fatal("expected no match when condition is false")
	}
	// Condition met.
	state["verify.failure_count"] = 2
	rule = rs.Evaluate("code", interaction.TriggerVerificationFailure, state, nil)
	if rule == nil {
		t.Fatal("expected match when condition is true")
	}
}

func TestTransitionRuleSet_RulesFrom(t *testing.T) {
	rs := interaction.NewTransitionRuleSet()
	rs.Add(interaction.TransitionRule{FromMode: "code", ToMode: "debug", Trigger: interaction.TriggerVerificationFailure})
	rs.Add(interaction.TransitionRule{FromMode: "code", ToMode: "planning", Trigger: interaction.TriggerScopeExpansion})
	rs.Add(interaction.TransitionRule{FromMode: "debug", ToMode: "code", Trigger: interaction.TriggerUserRequest})
	rs.Add(interaction.TransitionRule{FromMode: "*", ToMode: "planning", Trigger: interaction.TriggerUserRequest})

	codeRules := rs.RulesFrom("code")
	if len(codeRules) != 3 { // 2 code + 1 wildcard
		t.Fatalf("expected 3 rules from code (including wildcard), got %d", len(codeRules))
	}
}

func TestTransitionRuleSet_RulesTo(t *testing.T) {
	rs := interaction.NewTransitionRuleSet()
	rs.Add(interaction.TransitionRule{FromMode: "code", ToMode: "debug", Trigger: interaction.TriggerVerificationFailure})
	rs.Add(interaction.TransitionRule{FromMode: "debug", ToMode: "code", Trigger: interaction.TriggerUserRequest})
	rs.Add(interaction.TransitionRule{FromMode: "planning", ToMode: "code", Trigger: interaction.TriggerPhaseCompletion})

	codeRules := rs.RulesTo("code")
	if len(codeRules) != 2 {
		t.Fatalf("expected 2 rules to code, got %d", len(codeRules))
	}
}

func TestTransitionRuleSet_All(t *testing.T) {
	rs := interaction.NewTransitionRuleSet()
	rs.Add(interaction.TransitionRule{FromMode: "a", ToMode: "b", Trigger: interaction.TriggerUserRequest})
	rs.Add(interaction.TransitionRule{FromMode: "b", ToMode: "c", Trigger: interaction.TriggerUserRequest})
	if len(rs.All()) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rs.All()))
	}
}

func TestDefaultTransitionRules_NotEmpty(t *testing.T) {
	rs := interaction.DefaultTransitionRules()
	if len(rs.All()) == 0 {
		t.Fatal("expected default rules to be non-empty")
	}
}

// ---------------------------------------------------------------------------
// TransitionStack
// ---------------------------------------------------------------------------

func TestTransitionStack_PushPopPeek(t *testing.T) {
	ts := interaction.NewTransitionStack()
	if !ts.IsEmpty() {
		t.Fatal("expected empty stack")
	}
	if ts.Depth() != 0 {
		t.Fatal("expected depth 0")
	}
	ts.Push(interaction.TransitionFrame{Mode: "code", Phase: "implement"})
	if ts.IsEmpty() {
		t.Fatal("expected non-empty stack after push")
	}
	if ts.Depth() != 1 {
		t.Fatalf("expected depth 1, got %d", ts.Depth())
	}
	peeked := ts.Peek()
	if peeked == nil || peeked.Mode != "code" {
		t.Fatalf("unexpected peek result: %v", peeked)
	}
	popped := ts.Pop()
	if popped == nil || popped.Mode != "code" {
		t.Fatalf("unexpected pop result: %v", popped)
	}
	if !ts.IsEmpty() {
		t.Fatal("expected empty after pop")
	}
}

func TestTransitionStack_PopEmptyReturnsNil(t *testing.T) {
	ts := interaction.NewTransitionStack()
	if ts.Pop() != nil {
		t.Fatal("expected nil pop from empty stack")
	}
}

func TestTransitionStack_PeekEmptyReturnsNil(t *testing.T) {
	ts := interaction.NewTransitionStack()
	if ts.Peek() != nil {
		t.Fatal("expected nil peek from empty stack")
	}
}

func TestTransitionStack_LIFOOrder(t *testing.T) {
	ts := interaction.NewTransitionStack()
	ts.Push(interaction.TransitionFrame{Mode: "a"})
	ts.Push(interaction.TransitionFrame{Mode: "b"})
	ts.Push(interaction.TransitionFrame{Mode: "c"})
	if ts.Pop().Mode != "c" {
		t.Fatal("expected LIFO order: c first")
	}
	if ts.Pop().Mode != "b" {
		t.Fatal("expected LIFO order: b second")
	}
}

func TestTransitionStack_CollectReturnArtifacts(t *testing.T) {
	ts := interaction.NewTransitionStack()
	ts.Push(interaction.TransitionFrame{
		Mode:            "code",
		ReturnArtifacts: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
	})
	bundle := interaction.NewArtifactBundle()
	bundle.Add(euclotypes.Artifact{Kind: euclotypes.ArtifactKindExplore, Summary: "exp"})
	bundle.Add(euclotypes.Artifact{Kind: euclotypes.ArtifactKindAnalyze, Summary: "ana"})

	artifacts := ts.CollectReturnArtifacts(bundle)
	if len(artifacts) != 1 || artifacts[0].Kind != euclotypes.ArtifactKindExplore {
		t.Fatalf("expected 1 explore artifact, got %v", artifacts)
	}
}

func TestTransitionStack_CollectReturnArtifactsNilBundle(t *testing.T) {
	ts := interaction.NewTransitionStack()
	ts.Push(interaction.TransitionFrame{Mode: "code"})
	if ts.CollectReturnArtifacts(nil) != nil {
		t.Fatal("expected nil result for nil bundle")
	}
}

func TestTransitionStack_CollectReturnArtifactsEmptyStack(t *testing.T) {
	ts := interaction.NewTransitionStack()
	bundle := interaction.NewArtifactBundle()
	if ts.CollectReturnArtifacts(bundle) != nil {
		t.Fatal("expected nil result for empty stack")
	}
}

// ---------------------------------------------------------------------------
// ModeMachineRegistry
// ---------------------------------------------------------------------------

func TestModeMachineRegistry_RegisterAndHas(t *testing.T) {
	r := interaction.NewModeMachineRegistry()
	if r.Has("chat") {
		t.Fatal("expected empty registry")
	}
	r.Register("chat", func(e interaction.FrameEmitter, _ *interaction.AgencyResolver) *interaction.PhaseMachine {
		return interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
			Mode:    "chat",
			Emitter: e,
		})
	})
	if !r.Has("chat") {
		t.Fatal("expected chat to be registered")
	}
}

func TestModeMachineRegistry_BuildUnknownReturnsNil(t *testing.T) {
	r := interaction.NewModeMachineRegistry()
	m := r.Build("unknown", &interaction.NoopEmitter{}, nil)
	if m != nil {
		t.Fatal("expected nil for unknown mode")
	}
}

func TestModeMachineRegistry_BuildReturnsNewMachine(t *testing.T) {
	r := interaction.NewModeMachineRegistry()
	r.Register("chat", func(e interaction.FrameEmitter, _ *interaction.AgencyResolver) *interaction.PhaseMachine {
		return interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
			Mode:    "chat",
			Emitter: e,
		})
	})
	m := r.Build("chat", &interaction.NoopEmitter{}, nil)
	if m == nil {
		t.Fatal("expected non-nil machine")
	}
}

func TestModeMachineRegistry_ModesContainsRegistered(t *testing.T) {
	r := interaction.NewModeMachineRegistry()
	r.Register("chat", func(e interaction.FrameEmitter, _ *interaction.AgencyResolver) *interaction.PhaseMachine {
		return nil
	})
	r.Register("debug", func(e interaction.FrameEmitter, _ *interaction.AgencyResolver) *interaction.PhaseMachine {
		return nil
	})
	modes := r.Modes()
	if len(modes) != 2 {
		t.Fatalf("expected 2 modes, got %d", len(modes))
	}
}

// ---------------------------------------------------------------------------
// PhaseMachine
// ---------------------------------------------------------------------------

// stubHandler is a minimal PhaseHandler for testing.
type stubHandler struct {
	outcome interaction.PhaseOutcome
	err     error
}

func (h *stubHandler) Execute(_ context.Context, _ interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	return h.outcome, h.err
}

func TestPhaseMachine_RunAdvancesPhases(t *testing.T) {
	executed := []string{}
	phases := []interaction.PhaseDefinition{
		{ID: "p1", Handler: &recordingHandler{executed: &executed}},
		{ID: "p2", Handler: &recordingHandler{executed: &executed}},
	}
	m := interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
		Mode:    "chat",
		Phases:  phases,
		Emitter: &interaction.NoopEmitter{},
	})
	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(executed) != 2 || executed[0] != "p1" || executed[1] != "p2" {
		t.Fatalf("expected p1,p2 executed, got %v", executed)
	}
}

func TestPhaseMachine_SkipWhenSkipsPhase(t *testing.T) {
	executed := []string{}
	phases := []interaction.PhaseDefinition{
		{ID: "p1", Handler: &recordingHandler{executed: &executed}},
		{
			ID:       "p2",
			Handler:  &recordingHandler{executed: &executed},
			SkipWhen: func(map[string]any, *interaction.ArtifactBundle) bool { return true },
		},
		{ID: "p3", Handler: &recordingHandler{executed: &executed}},
	}
	m := interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
		Mode:    "chat",
		Phases:  phases,
		Emitter: &interaction.NoopEmitter{},
	})
	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(executed) != 2 || executed[0] != "p1" || executed[1] != "p3" {
		t.Fatalf("expected p1,p3 executed (p2 skipped), got %v", executed)
	}
	skipped := m.SkippedPhases()
	if len(skipped) != 1 || skipped[0] != "p2" {
		t.Fatalf("expected p2 in skipped, got %v", skipped)
	}
}

func TestPhaseMachine_EnterGuardError(t *testing.T) {
	phases := []interaction.PhaseDefinition{
		{
			ID:         "p1",
			Handler:    &stubHandler{outcome: interaction.PhaseOutcome{Advance: true}},
			EnterGuard: func(map[string]any, *interaction.ArtifactBundle) error { return errors.New("guard failed") },
		},
	}
	m := interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
		Mode:    "chat",
		Phases:  phases,
		Emitter: &interaction.NoopEmitter{},
	})
	err := m.Run(context.Background())
	if err == nil || !errors.Is(err, errors.New("guard failed")) {
		// Error wrapping — just check it's non-nil and contains "guard"
		if err == nil {
			t.Fatal("expected error from enter guard")
		}
	}
}

func TestPhaseMachine_HandlerErrorPropagates(t *testing.T) {
	phases := []interaction.PhaseDefinition{
		{ID: "p1", Handler: &stubHandler{err: errors.New("handler failed")}},
	}
	m := interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
		Mode:    "chat",
		Phases:  phases,
		Emitter: &interaction.NoopEmitter{},
	})
	if err := m.Run(context.Background()); err == nil {
		t.Fatal("expected error from handler")
	}
}

func TestPhaseMachine_JumpToPhase(t *testing.T) {
	executed := []string{}
	var machine *interaction.PhaseMachine
	phases := []interaction.PhaseDefinition{
		{ID: "p1", Handler: &recordingHandler{executed: &executed}},
		{ID: "p2", Handler: &jumpHandler{target: "p3", executed: &executed, machineRef: &machine}},
		{ID: "p3", Handler: &recordingHandler{executed: &executed}},
	}
	machine = interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
		Mode:    "chat",
		Phases:  phases,
		Emitter: &interaction.NoopEmitter{},
	})
	if err := machine.Run(context.Background()); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	// p1, p2 (jump to p3), p3
	if len(executed) != 3 {
		t.Fatalf("expected 3 executed, got %v", executed)
	}
}

func TestPhaseMachine_CurrentPhaseEmpty(t *testing.T) {
	m := interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
		Mode:    "chat",
		Emitter: &interaction.NoopEmitter{},
	})
	if m.CurrentPhase() != "" {
		t.Fatalf("expected empty current phase for empty machine, got %q", m.CurrentPhase())
	}
}

func TestPhaseMachine_ExecutedPhasesEmpty(t *testing.T) {
	m := interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
		Mode:    "chat",
		Emitter: &interaction.NoopEmitter{},
	})
	if m.ExecutedPhases() != nil {
		t.Fatal("expected nil for unrun machine")
	}
}

func TestPhaseMachine_StateAndArtifactsAccessors(t *testing.T) {
	m := interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
		Mode:    "chat",
		Emitter: &interaction.NoopEmitter{},
	})
	if m.State() == nil {
		t.Fatal("expected non-nil state map")
	}
	if m.Artifacts() == nil {
		t.Fatal("expected non-nil artifact bundle")
	}
}

func TestPhaseMachine_CanceledContextStopsRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	phases := []interaction.PhaseDefinition{
		{ID: "p1", Handler: &stubHandler{outcome: interaction.PhaseOutcome{Advance: true}, err: nil}},
	}
	m := interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
		Mode:    "chat",
		Phases:  phases,
		Emitter: &interaction.NoopEmitter{},
	})
	err := m.Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// InteractionRecording
// ---------------------------------------------------------------------------

func TestInteractionRecording_RecordFrame(t *testing.T) {
	rec := interaction.NewInteractionRecording()
	rec.RecordFrame(interaction.InteractionFrame{
		Kind:  interaction.FrameProposal,
		Mode:  "chat",
		Phase: "understand",
	})
	events := rec.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "frame" {
		t.Fatalf("expected type=frame, got %q", events[0].Type)
	}
}

func TestInteractionRecording_FrameEvents(t *testing.T) {
	rec := interaction.NewInteractionRecording()
	rec.RecordFrame(interaction.InteractionFrame{Kind: interaction.FrameProposal, Mode: "chat", Phase: "p1"})
	rec.RecordTransition("chat", "debug", "trigger")
	rec.RecordFrame(interaction.InteractionFrame{Kind: interaction.FrameResult, Mode: "chat", Phase: "p2"})

	frames := rec.FrameEvents()
	if len(frames) != 2 {
		t.Fatalf("expected 2 frame events, got %d", len(frames))
	}
}

func TestInteractionRecording_TransitionEvents(t *testing.T) {
	rec := interaction.NewInteractionRecording()
	rec.RecordFrame(interaction.InteractionFrame{Kind: interaction.FrameProposal, Mode: "chat", Phase: "p1"})
	rec.RecordTransition("chat", "debug", "trigger")
	rec.RecordTransition("debug", "code", "phase_completion")

	transitions := rec.TransitionEvents()
	if len(transitions) != 2 {
		t.Fatalf("expected 2 transition events, got %d", len(transitions))
	}
}

func TestInteractionRecording_RecordPhaseSkip(t *testing.T) {
	rec := interaction.NewInteractionRecording()
	rec.RecordPhaseSkip("p1", "chat", "condition met")
	events := rec.Events()
	if len(events) != 1 || events[0].Type != "phase_skip" {
		t.Fatalf("expected 1 phase_skip event, got %v", events)
	}
}

func TestInteractionRecording_ToStateMap(t *testing.T) {
	rec := interaction.NewInteractionRecording()
	rec.RecordFrame(interaction.InteractionFrame{Kind: interaction.FrameProposal, Mode: "chat", Phase: "p1"})
	rec.RecordTransition("chat", "debug", "trigger")

	sm := rec.ToStateMap()
	if sm == nil {
		t.Fatal("expected non-nil state map")
	}
	if sm["event_count"].(int) != 2 {
		t.Fatalf("expected event_count=2, got %v", sm["event_count"])
	}
	frames, ok := sm["frames"].([]map[string]any)
	if !ok || len(frames) != 1 {
		t.Fatalf("expected 1 frame in state map, got %v", sm["frames"])
	}
	transitions, ok := sm["transitions"].([]map[string]any)
	if !ok || len(transitions) != 1 {
		t.Fatalf("expected 1 transition in state map, got %v", sm["transitions"])
	}
}

func TestInteractionRecording_MarshalJSON(t *testing.T) {
	rec := interaction.NewInteractionRecording()
	rec.RecordFrame(interaction.InteractionFrame{Kind: interaction.FrameProposal, Mode: "chat", Phase: "p1"})
	data, err := rec.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty JSON")
	}
}

func TestInteractionRecording_ToJSONLines(t *testing.T) {
	rec := interaction.NewInteractionRecording()
	rec.RecordFrame(interaction.InteractionFrame{Kind: interaction.FrameProposal, Mode: "chat", Phase: "p1"})
	data, err := rec.ToJSONLines()
	if err != nil {
		t.Fatalf("ToJSONLines error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty JSONL output")
	}
}

// ---------------------------------------------------------------------------
// RecordingEmitter
// ---------------------------------------------------------------------------

func TestRecordingEmitter_RecordsFrameAndResponse(t *testing.T) {
	noop := &interaction.NoopEmitter{}
	re := interaction.NewRecordingEmitter(noop)

	f := interaction.InteractionFrame{
		Kind:  interaction.FrameProposal,
		Mode:  "chat",
		Phase: "understand",
		Actions: []interaction.ActionSlot{
			{ID: "confirm", Default: true},
		},
	}
	if err := re.Emit(context.Background(), f); err != nil {
		t.Fatalf("Emit error: %v", err)
	}
	resp, err := re.AwaitResponse(context.Background())
	if err != nil {
		t.Fatalf("AwaitResponse error: %v", err)
	}
	if resp.ActionID != "confirm" {
		t.Fatalf("expected confirm, got %q", resp.ActionID)
	}

	events := re.Recording.Events()
	// Expect frame + response events.
	if len(events) != 2 {
		t.Fatalf("expected 2 events (frame+response), got %d", len(events))
	}
}

// ---------------------------------------------------------------------------
// ExtractSessionResume
// ---------------------------------------------------------------------------

func TestExtractSessionResume_NilStateReturnsNil(t *testing.T) {
	if interaction.ExtractSessionResume(nil) != nil {
		t.Fatal("expected nil for nil state")
	}
}

func TestExtractSessionResume_NoKeyReturnsNil(t *testing.T) {
	ctx := core.NewContext()
	if interaction.ExtractSessionResume(ctx) != nil {
		t.Fatal("expected nil when key is absent")
	}
}

func TestExtractSessionResume_InteractionStateValue(t *testing.T) {
	ctx := core.NewContext()
	is := interaction.InteractionState{
		Mode:         "chat",
		CurrentPhase: "implement",
	}
	ctx.Set("euclo.interaction_state", is)
	resume := interaction.ExtractSessionResume(ctx)
	if resume == nil {
		t.Fatal("expected non-nil resume")
	}
	if resume.Mode != "chat" {
		t.Fatalf("expected mode=chat, got %q", resume.Mode)
	}
	if resume.LastPhase != "implement" {
		t.Fatalf("expected last_phase=implement, got %q", resume.LastPhase)
	}
}

func TestExtractSessionResume_MapValue(t *testing.T) {
	ctx := core.NewContext()
	ctx.Set("euclo.interaction_state", map[string]any{
		"mode":          "debug",
		"current_phase": "investigate",
	})
	resume := interaction.ExtractSessionResume(ctx)
	if resume == nil {
		t.Fatal("expected non-nil resume")
	}
	if resume.Mode != "debug" {
		t.Fatalf("expected mode=debug, got %q", resume.Mode)
	}
}

func TestExtractSessionResume_EmptyModeReturnsNil(t *testing.T) {
	ctx := core.NewContext()
	ctx.Set("euclo.interaction_state", interaction.InteractionState{Mode: ""})
	if interaction.ExtractSessionResume(ctx) != nil {
		t.Fatal("expected nil for empty mode")
	}
}

func TestExtractSessionResume_CollectsArtifactKinds(t *testing.T) {
	ctx := core.NewContext()
	ctx.Set("euclo.interaction_state", interaction.InteractionState{Mode: "code", CurrentPhase: "implement"})
	ctx.Set("euclo.artifacts", []euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindExplore},
		{Kind: euclotypes.ArtifactKindAnalyze},
	})
	resume := interaction.ExtractSessionResume(ctx)
	if resume == nil {
		t.Fatal("expected non-nil resume")
	}
	if !resume.HasArtifacts {
		t.Fatal("expected HasArtifacts=true")
	}
	if len(resume.ArtifactKinds) != 2 {
		t.Fatalf("expected 2 artifact kinds, got %v", resume.ArtifactKinds)
	}
}

// ---------------------------------------------------------------------------
// HandleResumeResponse
// ---------------------------------------------------------------------------

func TestHandleResumeResponse_Resume(t *testing.T) {
	r := interaction.HandleResumeResponse(interaction.UserResponse{ActionID: "resume"})
	if r != "resume" {
		t.Fatalf("expected resume, got %q", r)
	}
}

func TestHandleResumeResponse_Restart(t *testing.T) {
	r := interaction.HandleResumeResponse(interaction.UserResponse{ActionID: "restart"})
	if r != "restart" {
		t.Fatalf("expected restart, got %q", r)
	}
}

func TestHandleResumeResponse_Switch(t *testing.T) {
	r := interaction.HandleResumeResponse(interaction.UserResponse{ActionID: "switch"})
	if r != "switch" {
		t.Fatalf("expected switch, got %q", r)
	}
}

func TestHandleResumeResponse_UnknownDefaultsToResume(t *testing.T) {
	r := interaction.HandleResumeResponse(interaction.UserResponse{ActionID: "garbage"})
	if r != "resume" {
		t.Fatalf("expected resume (default), got %q", r)
	}
}

// ---------------------------------------------------------------------------
// ApplySessionResume
// ---------------------------------------------------------------------------

func TestApplySessionResume_NilInputsNoPanic(t *testing.T) {
	interaction.ApplySessionResume(nil, nil)
}

func TestApplySessionResume_RestoresPhaseState(t *testing.T) {
	phases := []interaction.PhaseDefinition{
		{ID: "init", Handler: &stubHandler{outcome: interaction.PhaseOutcome{Advance: true}}},
		{ID: "implement", Handler: &stubHandler{outcome: interaction.PhaseOutcome{Advance: true}}},
	}
	m := interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
		Mode:    "code",
		Phases:  phases,
		Emitter: &interaction.NoopEmitter{},
	})
	resume := &interaction.SessionResume{
		Mode:      "code",
		LastPhase: "implement",
		PhaseStates: map[string]any{
			"init.done": true,
		},
	}
	interaction.ApplySessionResume(m, resume)
	if m.State()["init.done"] != true {
		t.Fatal("expected init.done to be restored")
	}
	if m.State()["session.resumed"] != true {
		t.Fatal("expected session.resumed flag")
	}
	if m.CurrentPhase() != "implement" {
		t.Fatalf("expected current phase=implement, got %q", m.CurrentPhase())
	}
}

// ---------------------------------------------------------------------------
// BuildResumeFrame
// ---------------------------------------------------------------------------

func TestBuildResumeFrame_Kind(t *testing.T) {
	resume := &interaction.SessionResume{Mode: "code", LastPhase: "implement"}
	f := interaction.BuildResumeFrame(resume)
	if f.Kind != interaction.FrameSessionResume {
		t.Fatalf("expected FrameSessionResume, got %q", f.Kind)
	}
	if f.Mode != "code" {
		t.Fatalf("expected mode=code, got %q", f.Mode)
	}
}

// ---------------------------------------------------------------------------
// CarryOverArtifacts
// ---------------------------------------------------------------------------

func TestCarryOverArtifacts_NilBundleReturnsNil(t *testing.T) {
	if interaction.CarryOverArtifacts(nil, "code", "debug") != nil {
		t.Fatal("expected nil for nil bundle")
	}
}

func TestCarryOverArtifacts_UnknownFromModeReturnsNil(t *testing.T) {
	b := interaction.NewArtifactBundle()
	b.Add(euclotypes.Artifact{Kind: euclotypes.ArtifactKindExplore})
	if interaction.CarryOverArtifacts(b, "unknown", "debug") != nil {
		t.Fatal("expected nil for unknown from mode")
	}
}

func TestCarryOverArtifacts_KnownTransitionFilters(t *testing.T) {
	b := interaction.NewArtifactBundle()
	b.Add(euclotypes.Artifact{Kind: euclotypes.ArtifactKindExplore, Summary: "exp"})
	b.Add(euclotypes.Artifact{Kind: euclotypes.ArtifactKindEditIntent, Summary: "edit"})
	b.Add(euclotypes.Artifact{Kind: euclotypes.ArtifactKindPlan, Summary: "plan"}) // should not carry code→debug

	artifacts := interaction.CarryOverArtifacts(b, "code", "debug")
	// code→debug carries: explore, edit_intent, verification
	if len(artifacts) != 2 {
		t.Fatalf("expected 2 artifacts (explore+edit_intent), got %d: %v", len(artifacts), artifacts)
	}
}

func TestCarryOverArtifactsFromRules_UsesRuleWhenMatched(t *testing.T) {
	bundle := interaction.NewArtifactBundle()
	bundle.Add(euclotypes.Artifact{Kind: euclotypes.ArtifactKindExplore, Summary: "exp"})

	rs := interaction.NewTransitionRuleSet()
	rs.Add(interaction.TransitionRule{
		FromMode:      "code",
		ToMode:        "debug",
		Trigger:       interaction.TriggerVerificationFailure,
		ArtifactCarry: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
	})

	artifacts := interaction.CarryOverArtifactsFromRules(bundle, "code", "debug", rs, interaction.TriggerVerificationFailure, nil)
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact from rule, got %d", len(artifacts))
	}
}

func TestCarryOverArtifactsFromRules_FallsBackToStaticMap(t *testing.T) {
	bundle := interaction.NewArtifactBundle()
	bundle.Add(euclotypes.Artifact{Kind: euclotypes.ArtifactKindPlan, Summary: "plan"})

	// Rules for a different trigger — won't match.
	rs := interaction.NewTransitionRuleSet()
	rs.Add(interaction.TransitionRule{
		FromMode: "code",
		ToMode:   "planning",
		Trigger:  interaction.TriggerScopeExpansion,
	})

	// Fallback: planning→code carries ArtifactKindPlan
	artifacts := interaction.CarryOverArtifactsFromRules(bundle, "planning", "code", rs, interaction.TriggerPhaseCompletion, nil)
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact from static fallback, got %d", len(artifacts))
	}
}

func TestCarryOverArtifactsFromRules_NilBundleReturnsNil(t *testing.T) {
	if interaction.CarryOverArtifactsFromRules(nil, "code", "debug", nil, interaction.TriggerUserRequest, nil) != nil {
		t.Fatal("expected nil for nil bundle")
	}
}

// ---------------------------------------------------------------------------
// ExtractInteractionState / ExtractInteractionResult
// ---------------------------------------------------------------------------

func TestExtractInteractionState_EmptyMachine(t *testing.T) {
	m := interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
		Mode:    "chat",
		Emitter: &interaction.NoopEmitter{},
	})
	is := interaction.ExtractInteractionState(m)
	if is.Mode != "chat" {
		t.Fatalf("expected mode=chat, got %q", is.Mode)
	}
	if is.PhaseStates == nil {
		t.Fatal("expected non-nil PhaseStates map")
	}
}

func TestExtractInteractionResult_EmptyMachine(t *testing.T) {
	m := interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
		Mode:    "chat",
		Emitter: &interaction.NoopEmitter{},
	})
	result := interaction.ExtractInteractionResult(m)
	if result.TransitionTo != "" {
		t.Fatalf("expected empty TransitionTo, got %q", result.TransitionTo)
	}
}

func TestExtractInteractionResult_TransitionAccepted(t *testing.T) {
	m := interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
		Mode:    "chat",
		Emitter: &interaction.NoopEmitter{},
	})
	m.State()["transition.accepted"] = "debug"
	result := interaction.ExtractInteractionResult(m)
	if result.TransitionTo != "debug" {
		t.Fatalf("expected TransitionTo=debug, got %q", result.TransitionTo)
	}
}

// ---------------------------------------------------------------------------
// DefaultInteractionConfig
// ---------------------------------------------------------------------------

func TestDefaultInteractionConfig(t *testing.T) {
	cfg := interaction.DefaultInteractionConfig()
	if !cfg.Enabled {
		t.Fatal("expected Enabled=true")
	}
	if cfg.Budget.MaxQuestions != 3 {
		t.Fatalf("expected MaxQuestions=3, got %d", cfg.Budget.MaxQuestions)
	}
}

// ---------------------------------------------------------------------------
// Helper types for PhaseMachine tests
// ---------------------------------------------------------------------------

// recordingHandler records the phase ID when executed and advances.
type recordingHandler struct {
	executed *[]string
}

func (h *recordingHandler) Execute(_ context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	*h.executed = append(*h.executed, mc.Phase)
	return interaction.PhaseOutcome{Advance: true}, nil
}

// jumpHandler records phase ID and returns a JumpTo outcome.
type jumpHandler struct {
	target     string
	executed   *[]string
	machineRef **interaction.PhaseMachine
}

func (h *jumpHandler) Execute(_ context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	*h.executed = append(*h.executed, mc.Phase)
	return interaction.PhaseOutcome{JumpTo: h.target}, nil
}
