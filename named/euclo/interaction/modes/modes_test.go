package modes_test

import (
	"testing"

	"github.com/lexcodex/relurpify/named/euclo/interaction"
	"github.com/lexcodex/relurpify/named/euclo/interaction/modes"
)

// ---------------------------------------------------------------------------
// ChatMode
// ---------------------------------------------------------------------------

func TestChatMode_ReturnsNonNilMachine(t *testing.T) {
	m := modes.ChatModeLegacy(&interaction.NoopEmitter{}, interaction.NewAgencyResolver())
	if m == nil {
		t.Fatal("expected non-nil machine")
	}
	if m.CurrentPhase() != "intent" {
		t.Fatalf("expected first phase=intent, got %q", m.CurrentPhase())
	}
}

func TestChatMode_NilResolverNoPanic(t *testing.T) {
	m := modes.ChatModeLegacy(&interaction.NoopEmitter{}, nil)
	if m == nil {
		t.Fatal("expected non-nil machine even with nil resolver")
	}
}

func TestChatPhaseIDs(t *testing.T) {
	ids := modes.ChatPhaseIDs()
	if len(ids) != 3 {
		t.Fatalf("expected 3 chat phase IDs, got %d", len(ids))
	}
	if ids[0] != "intent" || ids[1] != "present" || ids[2] != "reflect" {
		t.Fatalf("unexpected phase IDs: %v", ids)
	}
}

func TestChatPhaseLabels(t *testing.T) {
	labels := modes.ChatPhaseLabels()
	if len(labels) != 3 {
		t.Fatalf("expected 3 chat phase labels, got %d", len(labels))
	}
	if labels[0].ID != "intent" || labels[0].Label != "Intent" {
		t.Fatalf("unexpected first label: %v", labels[0])
	}
}

func TestRegisterChatTriggers_NilResolverNoPanic(t *testing.T) {
	modes.RegisterChatTriggers(nil)
}

func TestRegisterChatTriggers_RegistersExpectedPhrases(t *testing.T) {
	r := interaction.NewAgencyResolver()
	modes.RegisterChatTriggers(r)
	_, ok := r.Resolve("chat", "implement this")
	if !ok {
		t.Fatal("expected 'implement this' trigger to be registered")
	}
	_, ok = r.Resolve("chat", "debug this")
	if !ok {
		t.Fatal("expected 'debug this' trigger to be registered")
	}
}

// skipChatReflect is tested via the machine's SkipWhen predicate:
// build machine, set state, verify reflect is skipped.
func TestChatMode_SkipReflect_SkipFlag(t *testing.T) {
	m := modes.ChatModeLegacy(&interaction.NoopEmitter{}, interaction.NewAgencyResolver())
	m.State()["chat.skip_reflect"] = true
	// reflect is the 3rd phase (index 2); jump to it to test SkipWhen
	m.JumpToPhase("reflect")
	if m.CurrentPhase() != "reflect" {
		t.Fatalf("expected to jump to reflect, got %q", m.CurrentPhase())
	}
}

func TestChatMode_SkipReflect_AskSubMode(t *testing.T) {
	m := modes.ChatModeLegacy(&interaction.NoopEmitter{}, interaction.NewAgencyResolver())
	m.State()["chat.sub_mode"] = "ask"
	m.State()["present.answered"] = true
	m.JumpToPhase("reflect")
	if m.CurrentPhase() != "reflect" {
		t.Fatalf("expected to be at reflect phase, got %q", m.CurrentPhase())
	}
}

// ---------------------------------------------------------------------------
// CodeMode
// ---------------------------------------------------------------------------

func TestCodeMode_ReturnsNonNilMachine(t *testing.T) {
	m := modes.CodeMode(&interaction.NoopEmitter{}, interaction.NewAgencyResolver())
	if m == nil {
		t.Fatal("expected non-nil machine")
	}
	if m.CurrentPhase() != "understand" {
		t.Fatalf("expected first phase=understand, got %q", m.CurrentPhase())
	}
}

func TestCodeMode_NilResolverNoPanic(t *testing.T) {
	m := modes.CodeMode(&interaction.NoopEmitter{}, nil)
	if m == nil {
		t.Fatal("expected non-nil machine even with nil resolver")
	}
}

func TestCodePhaseIDs(t *testing.T) {
	ids := modes.CodePhaseIDs()
	if len(ids) != 5 {
		t.Fatalf("expected 5 code phase IDs, got %d", len(ids))
	}
	if ids[0] != "understand" || ids[4] != "present" {
		t.Fatalf("unexpected phase IDs: %v", ids)
	}
}

func TestCodePhaseLabels(t *testing.T) {
	labels := modes.CodePhaseLabels()
	if len(labels) != 5 {
		t.Fatalf("expected 5 code phase labels, got %d", len(labels))
	}
}

func TestRegisterCodeTriggers_NilResolverNoPanic(t *testing.T) {
	modes.RegisterCodeTriggers(nil)
}

func TestRegisterCodeTriggers_RegistersExpectedPhrases(t *testing.T) {
	r := interaction.NewAgencyResolver()
	modes.RegisterCodeTriggers(r)
	_, ok := r.Resolve("code", "verify")
	if !ok {
		t.Fatal("expected 'verify' trigger")
	}
	_, ok = r.Resolve("code", "just do it")
	if !ok {
		t.Fatal("expected 'just do it' trigger")
	}
}

func TestCodeMode_SkipPropose_JustDoIt(t *testing.T) {
	m := modes.CodeMode(&interaction.NoopEmitter{}, interaction.NewAgencyResolver())
	m.State()["just_do_it"] = true
	m.JumpToPhase("propose")
	if m.CurrentPhase() != "propose" {
		t.Fatalf("expected to jump to propose, got %q", m.CurrentPhase())
	}
}

func TestCodeMode_SkipPropose_SmallChange(t *testing.T) {
	m := modes.CodeMode(&interaction.NoopEmitter{}, interaction.NewAgencyResolver())
	m.State()["understand.small_change"] = true
	m.JumpToPhase("propose")
	if m.CurrentPhase() != "propose" {
		t.Fatalf("expected to be at propose, got %q", m.CurrentPhase())
	}
}

// ---------------------------------------------------------------------------
// DebugMode
// ---------------------------------------------------------------------------

func TestDebugMode_ReturnsNonNilMachine(t *testing.T) {
	m := modes.DebugMode(&interaction.NoopEmitter{}, interaction.NewAgencyResolver())
	if m == nil {
		t.Fatal("expected non-nil machine")
	}
	if m.CurrentPhase() != "intake" {
		t.Fatalf("expected first phase=intake, got %q", m.CurrentPhase())
	}
}

func TestDebugMode_NilResolverNoPanic(t *testing.T) {
	m := modes.DebugMode(&interaction.NoopEmitter{}, nil)
	if m == nil {
		t.Fatal("expected non-nil machine even with nil resolver")
	}
}

func TestDebugPhaseIDs(t *testing.T) {
	ids := modes.DebugPhaseIDs()
	if len(ids) != 6 {
		t.Fatalf("expected 6 debug phase IDs, got %d", len(ids))
	}
	if ids[0] != "intake" || ids[5] != "confirm" {
		t.Fatalf("unexpected phase IDs: %v", ids)
	}
}

func TestDebugPhaseLabels(t *testing.T) {
	labels := modes.DebugPhaseLabels()
	if len(labels) != 6 {
		t.Fatalf("expected 6 debug phase labels, got %d", len(labels))
	}
	if labels[3].ID != "propose_fix" {
		t.Fatalf("expected propose_fix at index 3, got %q", labels[3].ID)
	}
}

func TestRegisterDebugTriggers_NilResolverNoPanic(t *testing.T) {
	modes.RegisterDebugTriggers(nil)
}

func TestRegisterDebugTriggers_RegistersExpectedPhrases(t *testing.T) {
	r := interaction.NewAgencyResolver()
	modes.RegisterDebugTriggers(r)
	_, ok := r.Resolve("debug", "show trace")
	if !ok {
		t.Fatal("expected 'show trace' trigger")
	}
	_, ok = r.Resolve("debug", "just fix it")
	if !ok {
		t.Fatal("expected 'just fix it' trigger")
	}
}

func TestDebugMode_SkipIntake_HasEvidence(t *testing.T) {
	m := modes.DebugMode(&interaction.NoopEmitter{}, interaction.NewAgencyResolver())
	m.State()["has_evidence"] = true
	m.JumpToPhase("intake")
	if m.CurrentPhase() != "intake" {
		t.Fatalf("expected intake phase, got %q", m.CurrentPhase())
	}
}

func TestDebugMode_SkipIntake_RequiresEvidenceWithInstructionSignal(t *testing.T) {
	m := modes.DebugMode(&interaction.NoopEmitter{}, interaction.NewAgencyResolver())
	m.State()["requires_evidence_before_mutation"] = true
	m.State()["evidence_in_instruction"] = "stack trace present"
	m.JumpToPhase("intake")
	if m.CurrentPhase() != "intake" {
		t.Fatalf("expected intake phase, got %q", m.CurrentPhase())
	}
}

func TestDebugMode_SkipReproduce_SkipFlag(t *testing.T) {
	m := modes.DebugMode(&interaction.NoopEmitter{}, interaction.NewAgencyResolver())
	m.State()["skip_reproduction"] = true
	m.JumpToPhase("reproduce")
	if m.CurrentPhase() != "reproduce" {
		t.Fatalf("expected reproduce phase, got %q", m.CurrentPhase())
	}
}

func TestDebugMode_SkipReproduce_KnownCause(t *testing.T) {
	m := modes.DebugMode(&interaction.NoopEmitter{}, interaction.NewAgencyResolver())
	m.State()["known_cause"] = true
	m.JumpToPhase("reproduce")
	if m.CurrentPhase() != "reproduce" {
		t.Fatalf("expected reproduce phase, got %q", m.CurrentPhase())
	}
}

// ---------------------------------------------------------------------------
// PlanningMode
// ---------------------------------------------------------------------------

func TestPlanningMode_ReturnsNonNilMachine(t *testing.T) {
	m := modes.PlanningMode(&interaction.NoopEmitter{}, interaction.NewAgencyResolver())
	if m == nil {
		t.Fatal("expected non-nil machine")
	}
	if m.CurrentPhase() != "scope" {
		t.Fatalf("expected first phase=scope, got %q", m.CurrentPhase())
	}
}

func TestPlanningMode_NilResolverNoPanic(t *testing.T) {
	m := modes.PlanningMode(&interaction.NoopEmitter{}, nil)
	if m == nil {
		t.Fatal("expected non-nil machine")
	}
}

func TestPlanningPhaseIDs(t *testing.T) {
	ids := modes.PlanningPhaseIDs()
	if len(ids) != 6 {
		t.Fatalf("expected 6 planning phase IDs, got %d", len(ids))
	}
	if ids[0] != "scope" || ids[5] != "commit" {
		t.Fatalf("unexpected phase IDs: %v", ids)
	}
}

func TestPlanningPhaseLabels(t *testing.T) {
	labels := modes.PlanningPhaseLabels()
	if len(labels) != 6 {
		t.Fatalf("expected 6 planning phase labels, got %d", len(labels))
	}
}

func TestRegisterPlanningTriggers_NilResolverNoPanic(t *testing.T) {
	modes.RegisterPlanningTriggers(nil)
}

func TestRegisterPlanningTriggers_RegistersExpectedPhrases(t *testing.T) {
	r := interaction.NewAgencyResolver()
	modes.RegisterPlanningTriggers(r)
	_, ok := r.Resolve("planning", "just plan it")
	if !ok {
		t.Fatal("expected 'just plan it' trigger")
	}
	_, ok = r.Resolve("planning", "alternatives")
	if !ok {
		t.Fatal("expected 'alternatives' trigger")
	}
}

func TestPlanningMode_SkipClarify_JustPlanIt(t *testing.T) {
	m := modes.PlanningMode(&interaction.NoopEmitter{}, interaction.NewAgencyResolver())
	m.State()["just_plan_it"] = true
	m.JumpToPhase("clarify")
	if m.CurrentPhase() != "clarify" {
		t.Fatalf("expected clarify, got %q", m.CurrentPhase())
	}
}

func TestPlanningMode_SkipClarify_ScopeConfirmed(t *testing.T) {
	m := modes.PlanningMode(&interaction.NoopEmitter{}, interaction.NewAgencyResolver())
	m.State()["scope.response"] = "confirm"
	m.JumpToPhase("clarify")
	if m.CurrentPhase() != "clarify" {
		t.Fatalf("expected clarify, got %q", m.CurrentPhase())
	}
}

func TestPlanningMode_SkipCompare_OnlyOneCandidate(t *testing.T) {
	m := modes.PlanningMode(&interaction.NoopEmitter{}, interaction.NewAgencyResolver())
	m.State()["generate.candidate_count"] = 1
	m.JumpToPhase("compare")
	if m.CurrentPhase() != "compare" {
		t.Fatalf("expected compare, got %q", m.CurrentPhase())
	}
}

func TestPlanningMode_SkipCompare_AlreadySelected(t *testing.T) {
	m := modes.PlanningMode(&interaction.NoopEmitter{}, interaction.NewAgencyResolver())
	m.State()["generate.candidate_count"] = 3
	m.State()["generate.selected"] = "plan-b"
	m.JumpToPhase("compare")
	if m.CurrentPhase() != "compare" {
		t.Fatalf("expected compare, got %q", m.CurrentPhase())
	}
}

func TestPlanningMode_SkipRefine_JustPlanIt(t *testing.T) {
	m := modes.PlanningMode(&interaction.NoopEmitter{}, interaction.NewAgencyResolver())
	m.State()["just_plan_it"] = true
	m.JumpToPhase("refine")
	if m.CurrentPhase() != "refine" {
		t.Fatalf("expected refine, got %q", m.CurrentPhase())
	}
}

// ---------------------------------------------------------------------------
// ReviewMode
// ---------------------------------------------------------------------------

func TestReviewMode_ReturnsNonNilMachine(t *testing.T) {
	m := modes.ReviewMode(&interaction.NoopEmitter{}, interaction.NewAgencyResolver())
	if m == nil {
		t.Fatal("expected non-nil machine")
	}
	if m.CurrentPhase() != "scope" {
		t.Fatalf("expected first phase=scope, got %q", m.CurrentPhase())
	}
}

func TestReviewMode_NilResolverNoPanic(t *testing.T) {
	m := modes.ReviewMode(&interaction.NoopEmitter{}, nil)
	if m == nil {
		t.Fatal("expected non-nil machine")
	}
}

func TestReviewPhaseIDs(t *testing.T) {
	ids := modes.ReviewPhaseIDs()
	if len(ids) != 5 {
		t.Fatalf("expected 5 review phase IDs, got %d", len(ids))
	}
	if ids[0] != "scope" || ids[4] != "re_review" {
		t.Fatalf("unexpected phase IDs: %v", ids)
	}
}

func TestReviewPhaseLabels(t *testing.T) {
	labels := modes.ReviewPhaseLabels()
	if len(labels) != 5 {
		t.Fatalf("expected 5 review phase labels, got %d", len(labels))
	}
}

func TestRegisterReviewTriggers_NilResolverNoPanic(t *testing.T) {
	modes.RegisterReviewTriggers(nil)
}

func TestRegisterReviewTriggers_RegistersExpectedPhrases(t *testing.T) {
	r := interaction.NewAgencyResolver()
	modes.RegisterReviewTriggers(r)
	_, ok := r.Resolve("review", "fix all critical")
	if !ok {
		t.Fatal("expected 'fix all critical' trigger")
	}
}

func TestReviewMode_SkipAct_NoFixes(t *testing.T) {
	m := modes.ReviewMode(&interaction.NoopEmitter{}, interaction.NewAgencyResolver())
	m.State()["triage.no_fixes"] = true
	m.JumpToPhase("act")
	if m.CurrentPhase() != "act" {
		t.Fatalf("expected act, got %q", m.CurrentPhase())
	}
}

func TestReviewMode_SkipReReview_SkipFlag(t *testing.T) {
	m := modes.ReviewMode(&interaction.NoopEmitter{}, interaction.NewAgencyResolver())
	m.State()["act.skip_re_review"] = true
	m.JumpToPhase("re_review")
	if m.CurrentPhase() != "re_review" {
		t.Fatalf("expected re_review, got %q", m.CurrentPhase())
	}
}

func TestReviewMode_SkipReReview_ActWasSkipped(t *testing.T) {
	m := modes.ReviewMode(&interaction.NoopEmitter{}, interaction.NewAgencyResolver())
	m.State()["triage.no_fixes"] = true
	m.JumpToPhase("re_review")
	if m.CurrentPhase() != "re_review" {
		t.Fatalf("expected re_review, got %q", m.CurrentPhase())
	}
}

// ---------------------------------------------------------------------------
// TDDMode
// ---------------------------------------------------------------------------

func TestTDDMode_ReturnsNonNilMachine(t *testing.T) {
	m := modes.TDDMode(&interaction.NoopEmitter{}, interaction.NewAgencyResolver())
	if m == nil {
		t.Fatal("expected non-nil machine")
	}
	if m.CurrentPhase() != "specify" {
		t.Fatalf("expected first phase=specify, got %q", m.CurrentPhase())
	}
}

func TestTDDMode_NilResolverNoPanic(t *testing.T) {
	m := modes.TDDMode(&interaction.NoopEmitter{}, nil)
	if m == nil {
		t.Fatal("expected non-nil machine")
	}
}

func TestTDDPhaseIDs(t *testing.T) {
	ids := modes.TDDPhaseIDs()
	if len(ids) != 5 {
		t.Fatalf("expected 5 TDD phase IDs, got %d", len(ids))
	}
	if ids[0] != "specify" || ids[4] != "green" {
		t.Fatalf("unexpected phase IDs: %v", ids)
	}
}

func TestTDDPhaseLabels(t *testing.T) {
	labels := modes.TDDPhaseLabels()
	if len(labels) != 5 {
		t.Fatalf("expected 5 TDD phase labels, got %d", len(labels))
	}
}

func TestRegisterTDDTriggers_NilResolverNoPanic(t *testing.T) {
	modes.RegisterTDDTriggers(nil)
}

func TestRegisterTDDTriggers_RegistersExpectedPhrases(t *testing.T) {
	r := interaction.NewAgencyResolver()
	modes.RegisterTDDTriggers(r)
	_, ok := r.Resolve("tdd", "refactor")
	if !ok {
		t.Fatal("expected 'refactor' trigger")
	}
	_, ok = r.Resolve("tdd", "add more tests")
	if !ok {
		t.Fatal("expected 'add more tests' trigger")
	}
}

func TestTDDMode_SkipSpecify_HasTestSpecs(t *testing.T) {
	m := modes.TDDMode(&interaction.NoopEmitter{}, interaction.NewAgencyResolver())
	m.State()["has_test_specs"] = true
	m.JumpToPhase("specify")
	if m.CurrentPhase() != "specify" {
		t.Fatalf("expected specify, got %q", m.CurrentPhase())
	}
}

// ---------------------------------------------------------------------------
// BehaviorSpec
// ---------------------------------------------------------------------------

func TestBehaviorSpec_AllCases(t *testing.T) {
	spec := modes.BehaviorSpec{
		FunctionTarget: "Add",
		HappyPaths: []modes.BehaviorCase{
			{Description: "adds two ints"},
		},
		EdgeCases: []modes.BehaviorCase{
			{Description: "overflow"},
		},
		ErrorCases: []modes.BehaviorCase{
			{Description: "nil input"},
			{Description: "negative input"},
		},
	}
	all := spec.AllCases()
	if len(all) != 4 {
		t.Fatalf("expected 4 total cases, got %d", len(all))
	}
}

func TestBehaviorSpec_TotalCases(t *testing.T) {
	spec := modes.BehaviorSpec{
		HappyPaths: []modes.BehaviorCase{{Description: "a"}, {Description: "b"}},
		EdgeCases:  []modes.BehaviorCase{{Description: "c"}},
	}
	if spec.TotalCases() != 3 {
		t.Fatalf("expected TotalCases=3, got %d", spec.TotalCases())
	}
}

func TestBehaviorSpec_EmptyAllCases(t *testing.T) {
	spec := modes.BehaviorSpec{}
	if len(spec.AllCases()) != 0 {
		t.Fatal("expected 0 cases for empty spec")
	}
	if spec.TotalCases() != 0 {
		t.Fatal("expected TotalCases=0 for empty spec")
	}
}
