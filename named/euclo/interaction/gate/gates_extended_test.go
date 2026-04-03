package gate_test

import (
	"testing"

	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/interaction/gate"
)

// ---------------------------------------------------------------------------
// ArtifactGate.IsEmpty
// ---------------------------------------------------------------------------

func TestArtifactGate_IsEmpty(t *testing.T) {
	empty := gate.ArtifactGate{}
	if !empty.IsEmpty() {
		t.Fatal("expected empty gate to be empty")
	}
	nonEmpty := gate.ArtifactGate{
		RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
	}
	if nonEmpty.IsEmpty() {
		t.Fatal("expected non-empty gate to not be empty")
	}
}

// ---------------------------------------------------------------------------
// EvaluateGate
// ---------------------------------------------------------------------------

func TestEvaluateGate_EmptyGatePasses(t *testing.T) {
	pg := gate.PhaseGate{
		From:        gate.PhaseExplore,
		To:          gate.PhasePlan,
		DefaultGate: gate.ArtifactGate{},
		OnFail:      gate.GateFailBlock,
	}
	artifacts := euclotypes.NewArtifactState(nil)
	eval := gate.EvaluateGate(pg, "code", artifacts)
	if !eval.Passed {
		t.Fatal("expected empty gate to pass")
	}
}

func TestEvaluateGate_RequiredKindPresent(t *testing.T) {
	pg := gate.PhaseGate{
		From: gate.PhaseExplore,
		To:   gate.PhasePlan,
		DefaultGate: gate.ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
		},
		OnFail: gate.GateFailBlock,
	}
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "e1", Kind: euclotypes.ArtifactKindExplore, Payload: map[string]any{"files": []any{"main.go"}}},
	})
	eval := gate.EvaluateGate(pg, "code", artifacts)
	if !eval.Passed {
		t.Fatalf("expected gate to pass, missing: %v", eval.Missing)
	}
	if eval.From != gate.PhaseExplore || eval.To != gate.PhasePlan {
		t.Fatalf("unexpected phase range: %v→%v", eval.From, eval.To)
	}
}

func TestEvaluateGate_RequiredKindMissing(t *testing.T) {
	pg := gate.PhaseGate{
		From: gate.PhaseExplore,
		To:   gate.PhasePlan,
		DefaultGate: gate.ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore, euclotypes.ArtifactKindAnalyze},
		},
		OnFail: gate.GateFailBlock,
	}
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "e1", Kind: euclotypes.ArtifactKindExplore},
	})
	eval := gate.EvaluateGate(pg, "code", artifacts)
	if eval.Passed {
		t.Fatal("expected gate to fail with missing analyze")
	}
	if len(eval.Missing) != 1 || eval.Missing[0] != euclotypes.ArtifactKindAnalyze {
		t.Fatalf("unexpected missing kinds: %v", eval.Missing)
	}
}

func TestEvaluateGate_AnyOfKindsSatisfied(t *testing.T) {
	pg := gate.PhaseGate{
		From: gate.PhaseLocalize,
		To:   gate.PhasePatch,
		DefaultGate: gate.ArtifactGate{
			AnyOfKinds: [][]euclotypes.ArtifactKind{
				{euclotypes.ArtifactKindAnalyze, euclotypes.ArtifactKindRegressionAnalysis},
			},
		},
		OnFail: gate.GateFailBlock,
	}
	// Only analyze present — satisfies AnyOf
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "a1", Kind: euclotypes.ArtifactKindAnalyze},
	})
	eval := gate.EvaluateGate(pg, "debug", artifacts)
	if !eval.Passed {
		t.Fatal("expected AnyOf gate to pass when at least one kind is present")
	}
}

func TestEvaluateGate_AnyOfKindsNoneSatisfied(t *testing.T) {
	pg := gate.PhaseGate{
		From: gate.PhaseLocalize,
		To:   gate.PhasePatch,
		DefaultGate: gate.ArtifactGate{
			AnyOfKinds: [][]euclotypes.ArtifactKind{
				{euclotypes.ArtifactKindAnalyze, euclotypes.ArtifactKindRegressionAnalysis},
			},
		},
		OnFail: gate.GateFailBlock,
	}
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "e1", Kind: euclotypes.ArtifactKindExplore},
	})
	eval := gate.EvaluateGate(pg, "code", artifacts)
	if eval.Passed {
		t.Fatal("expected AnyOf gate to fail when none present")
	}
	if len(eval.Missing) == 0 {
		t.Fatal("expected missing kinds to be populated")
	}
}

func TestEvaluateGate_ModeOverrideUsed(t *testing.T) {
	pg := gate.PhaseGate{
		From: gate.PhaseExplore,
		To:   gate.PhasePlan,
		DefaultGate: gate.ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
		},
		ModeGates: map[string]gate.ArtifactGate{
			"debug": {
				RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore, euclotypes.ArtifactKindAnalyze},
			},
		},
		OnFail: gate.GateFailBlock,
	}
	// Only explore present — passes default, fails debug mode
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "e1", Kind: euclotypes.ArtifactKindExplore},
	})
	evalDefault := gate.EvaluateGate(pg, "code", artifacts)
	if !evalDefault.Passed {
		t.Fatal("expected default mode gate to pass")
	}
	evalDebug := gate.EvaluateGate(pg, "debug", artifacts)
	if evalDebug.Passed {
		t.Fatal("expected debug mode gate to fail (needs analyze too)")
	}
}

// ---------------------------------------------------------------------------
// EvaluateGate with validators
// ---------------------------------------------------------------------------

func TestEvaluateGate_ValidatorNotEmpty_Passes(t *testing.T) {
	pg := gate.PhaseGate{
		From: gate.PhaseExplore,
		To:   gate.PhasePlan,
		DefaultGate: gate.ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
			Validators: []gate.ArtifactValidator{
				{Kind: euclotypes.ArtifactKindExplore, Check: gate.ValidatorCheckNotEmpty},
			},
		},
		OnFail: gate.GateFailBlock,
	}
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "e1", Kind: euclotypes.ArtifactKindExplore, Payload: map[string]any{"files": "main.go"}},
	})
	eval := gate.EvaluateGate(pg, "code", artifacts)
	if !eval.Passed {
		t.Fatalf("expected validator to pass, failures: %v", eval.FailedValidators)
	}
}

func TestEvaluateGate_ValidatorNotEmpty_Fails(t *testing.T) {
	pg := gate.PhaseGate{
		From: gate.PhaseExplore,
		To:   gate.PhasePlan,
		DefaultGate: gate.ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
			Validators: []gate.ArtifactValidator{
				{Kind: euclotypes.ArtifactKindExplore, Field: "summary", Check: gate.ValidatorCheckNotEmpty},
			},
		},
		OnFail: gate.GateFailBlock,
	}
	// Payload has no "summary" key → extractField returns nil → isEmpty=true
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "e1", Kind: euclotypes.ArtifactKindExplore, Payload: map[string]any{"other": "value"}},
	})
	eval := gate.EvaluateGate(pg, "code", artifacts)
	if eval.Passed {
		t.Fatal("expected validator to fail when field is missing/empty")
	}
	if len(eval.FailedValidators) == 0 {
		t.Fatal("expected failed validators to be populated")
	}
}

func TestEvaluateGate_ValidatorHasKey_Passes(t *testing.T) {
	pg := gate.PhaseGate{
		From: gate.PhaseExplore,
		To:   gate.PhasePlan,
		DefaultGate: gate.ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
			Validators: []gate.ArtifactValidator{
				{Kind: euclotypes.ArtifactKindExplore, Check: gate.ValidatorCheckHasKey, Value: "status"},
			},
		},
		OnFail: gate.GateFailBlock,
	}
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "e1", Kind: euclotypes.ArtifactKindExplore, Payload: map[string]any{"status": "done"}},
	})
	eval := gate.EvaluateGate(pg, "code", artifacts)
	if !eval.Passed {
		t.Fatalf("expected has_key validator to pass, failures: %v", eval.FailedValidators)
	}
}

func TestEvaluateGate_ValidatorHasKey_MissingKey(t *testing.T) {
	pg := gate.PhaseGate{
		From: gate.PhaseExplore,
		To:   gate.PhasePlan,
		DefaultGate: gate.ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
			Validators: []gate.ArtifactValidator{
				{Kind: euclotypes.ArtifactKindExplore, Check: gate.ValidatorCheckHasKey, Value: "required_field"},
			},
		},
		OnFail: gate.GateFailBlock,
	}
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "e1", Kind: euclotypes.ArtifactKindExplore, Payload: map[string]any{"other": "value"}},
	})
	eval := gate.EvaluateGate(pg, "code", artifacts)
	if eval.Passed {
		t.Fatal("expected has_key validator to fail for missing key")
	}
}

func TestEvaluateGate_ValidatorHasKey_EmptyKeyValue(t *testing.T) {
	pg := gate.PhaseGate{
		From: gate.PhaseExplore,
		To:   gate.PhasePlan,
		DefaultGate: gate.ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
			Validators: []gate.ArtifactValidator{
				{Kind: euclotypes.ArtifactKindExplore, Check: gate.ValidatorCheckHasKey, Value: ""},
			},
		},
		OnFail: gate.GateFailBlock,
	}
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "e1", Kind: euclotypes.ArtifactKindExplore, Payload: map[string]any{"a": "b"}},
	})
	eval := gate.EvaluateGate(pg, "code", artifacts)
	if eval.Passed {
		t.Fatal("expected has_key with empty key to fail")
	}
}

func TestEvaluateGate_ValidatorHasKey_NonMapPayload(t *testing.T) {
	pg := gate.PhaseGate{
		From: gate.PhaseExplore,
		To:   gate.PhasePlan,
		DefaultGate: gate.ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
			Validators: []gate.ArtifactValidator{
				{Kind: euclotypes.ArtifactKindExplore, Check: gate.ValidatorCheckHasKey, Value: "key"},
			},
		},
		OnFail: gate.GateFailBlock,
	}
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "e1", Kind: euclotypes.ArtifactKindExplore, Payload: "not a map"},
	})
	eval := gate.EvaluateGate(pg, "code", artifacts)
	if eval.Passed {
		t.Fatal("expected has_key to fail for non-map payload")
	}
}

func TestEvaluateGate_ValidatorMinCount_Passes(t *testing.T) {
	pg := gate.PhaseGate{
		From: gate.PhasePlan,
		To:   gate.PhaseEdit,
		DefaultGate: gate.ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindPlan},
			Validators: []gate.ArtifactValidator{
				{Kind: euclotypes.ArtifactKindPlan, Field: "steps", Check: gate.ValidatorCheckMinCount, Value: 2},
			},
		},
		OnFail: gate.GateFailBlock,
	}
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "p1", Kind: euclotypes.ArtifactKindPlan, Payload: map[string]any{
			"steps": []any{"step1", "step2", "step3"},
		}},
	})
	eval := gate.EvaluateGate(pg, "code", artifacts)
	if !eval.Passed {
		t.Fatalf("expected min_count to pass with 3 steps, failures: %v", eval.FailedValidators)
	}
}

func TestEvaluateGate_ValidatorMinCount_Fails(t *testing.T) {
	pg := gate.PhaseGate{
		From: gate.PhasePlan,
		To:   gate.PhaseEdit,
		DefaultGate: gate.ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindPlan},
			Validators: []gate.ArtifactValidator{
				{Kind: euclotypes.ArtifactKindPlan, Field: "steps", Check: gate.ValidatorCheckMinCount, Value: 3},
			},
		},
		OnFail: gate.GateFailBlock,
	}
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "p1", Kind: euclotypes.ArtifactKindPlan, Payload: map[string]any{
			"steps": []any{"step1"},
		}},
	})
	eval := gate.EvaluateGate(pg, "code", artifacts)
	if eval.Passed {
		t.Fatal("expected min_count to fail with only 1 step")
	}
}

func TestEvaluateGate_ValidatorMinCount_InvalidCount(t *testing.T) {
	pg := gate.PhaseGate{
		From: gate.PhasePlan,
		To:   gate.PhaseEdit,
		DefaultGate: gate.ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindPlan},
			Validators: []gate.ArtifactValidator{
				{Kind: euclotypes.ArtifactKindPlan, Check: gate.ValidatorCheckMinCount, Value: 0},
			},
		},
		OnFail: gate.GateFailBlock,
	}
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "p1", Kind: euclotypes.ArtifactKindPlan, Payload: []any{"x", "y"}},
	})
	eval := gate.EvaluateGate(pg, "code", artifacts)
	if eval.Passed {
		t.Fatal("expected min_count with Value=0 to fail (invalid count)")
	}
}

func TestEvaluateGate_ValidatorEquals_Passes(t *testing.T) {
	pg := gate.PhaseGate{
		From: gate.PhaseImplement,
		To:   gate.PhaseVerify,
		DefaultGate: gate.ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindTDDLifecycle},
			Validators: []gate.ArtifactValidator{
				{Kind: euclotypes.ArtifactKindTDDLifecycle, Field: "current_phase", Check: gate.ValidatorCheckEquals, Value: "complete"},
			},
		},
		OnFail: gate.GateFailBlock,
	}
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "tdd1", Kind: euclotypes.ArtifactKindTDDLifecycle, Payload: map[string]any{"current_phase": "complete"}},
	})
	eval := gate.EvaluateGate(pg, "tdd", artifacts)
	if !eval.Passed {
		t.Fatalf("expected equals validator to pass: %v", eval.FailedValidators)
	}
}

func TestEvaluateGate_ValidatorEquals_Fails(t *testing.T) {
	pg := gate.PhaseGate{
		From: gate.PhaseImplement,
		To:   gate.PhaseVerify,
		DefaultGate: gate.ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindTDDLifecycle},
			Validators: []gate.ArtifactValidator{
				{Kind: euclotypes.ArtifactKindTDDLifecycle, Field: "current_phase", Check: gate.ValidatorCheckEquals, Value: "complete"},
			},
		},
		OnFail: gate.GateFailBlock,
	}
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "tdd1", Kind: euclotypes.ArtifactKindTDDLifecycle, Payload: map[string]any{"current_phase": "in_progress"}},
	})
	eval := gate.EvaluateGate(pg, "tdd", artifacts)
	if eval.Passed {
		t.Fatal("expected equals validator to fail")
	}
}

func TestEvaluateGate_ValidatorUnknownCheck(t *testing.T) {
	pg := gate.PhaseGate{
		From: gate.PhaseExplore,
		To:   gate.PhasePlan,
		DefaultGate: gate.ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
			Validators: []gate.ArtifactValidator{
				{Kind: euclotypes.ArtifactKindExplore, Check: "unknown_check"},
			},
		},
		OnFail: gate.GateFailBlock,
	}
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "e1", Kind: euclotypes.ArtifactKindExplore, Payload: map[string]any{"x": "y"}},
	})
	eval := gate.EvaluateGate(pg, "code", artifacts)
	if eval.Passed {
		t.Fatal("expected unknown_check validator to fail")
	}
}

func TestEvaluateGate_ValidatorNoArtifactOfKind(t *testing.T) {
	pg := gate.PhaseGate{
		From: gate.PhaseExplore,
		To:   gate.PhasePlan,
		DefaultGate: gate.ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
			Validators: []gate.ArtifactValidator{
				{Kind: euclotypes.ArtifactKindAnalyze, Check: gate.ValidatorCheckNotEmpty},
			},
		},
		OnFail: gate.GateFailBlock,
	}
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "e1", Kind: euclotypes.ArtifactKindExplore, Payload: map[string]any{"x": "y"}},
	})
	eval := gate.EvaluateGate(pg, "code", artifacts)
	if eval.Passed {
		t.Fatal("expected validator to fail when no artifacts of required kind for validator")
	}
}

func TestEvaluateGate_ValidatorMinCountStringSlice(t *testing.T) {
	pg := gate.PhaseGate{
		From: gate.PhasePlan,
		To:   gate.PhaseEdit,
		DefaultGate: gate.ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindPlan},
			Validators: []gate.ArtifactValidator{
				{Kind: euclotypes.ArtifactKindPlan, Field: "files", Check: gate.ValidatorCheckMinCount, Value: 1},
			},
		},
		OnFail: gate.GateFailBlock,
	}
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "p1", Kind: euclotypes.ArtifactKindPlan, Payload: map[string]any{
			"files": []string{"main.go", "util.go"},
		}},
	})
	eval := gate.EvaluateGate(pg, "code", artifacts)
	if !eval.Passed {
		t.Fatalf("expected min_count to pass with []string: %v", eval.FailedValidators)
	}
}

func TestEvaluateGate_ValidatorMinCountMapSlice(t *testing.T) {
	pg := gate.PhaseGate{
		From: gate.PhasePlan,
		To:   gate.PhaseEdit,
		DefaultGate: gate.ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindPlan},
			Validators: []gate.ArtifactValidator{
				{Kind: euclotypes.ArtifactKindPlan, Field: "edits", Check: gate.ValidatorCheckMinCount, Value: 1},
			},
		},
		OnFail: gate.GateFailBlock,
	}
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "p1", Kind: euclotypes.ArtifactKindPlan, Payload: map[string]any{
			"edits": []map[string]any{{"file": "a.go"}, {"file": "b.go"}},
		}},
	})
	eval := gate.EvaluateGate(pg, "code", artifacts)
	if !eval.Passed {
		t.Fatalf("expected min_count to pass with []map[string]any: %v", eval.FailedValidators)
	}
}

func TestEvaluateGate_ValidatorNotEmpty_StringPayload(t *testing.T) {
	pg := gate.PhaseGate{
		From: gate.PhaseExplore,
		To:   gate.PhasePlan,
		DefaultGate: gate.ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
			Validators: []gate.ArtifactValidator{
				{Kind: euclotypes.ArtifactKindExplore, Check: gate.ValidatorCheckNotEmpty},
			},
		},
		OnFail: gate.GateFailBlock,
	}
	// Non-empty string
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "e1", Kind: euclotypes.ArtifactKindExplore, Payload: "non-empty"},
	})
	eval := gate.EvaluateGate(pg, "code", artifacts)
	if !eval.Passed {
		t.Fatalf("expected non-empty string to pass not_empty validator: %v", eval.FailedValidators)
	}
	// Empty string
	artifacts2 := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "e2", Kind: euclotypes.ArtifactKindExplore, Payload: "   "},
	})
	eval2 := gate.EvaluateGate(pg, "code", artifacts2)
	if eval2.Passed {
		t.Fatal("expected whitespace-only string to fail not_empty validator")
	}
}

func TestEvaluateGate_ValidatorNotEmpty_MapPayload(t *testing.T) {
	pg := gate.PhaseGate{
		From: gate.PhaseExplore,
		To:   gate.PhasePlan,
		DefaultGate: gate.ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
			Validators: []gate.ArtifactValidator{
				{Kind: euclotypes.ArtifactKindExplore, Check: gate.ValidatorCheckNotEmpty},
			},
		},
		OnFail: gate.GateFailBlock,
	}
	// Empty map
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "e1", Kind: euclotypes.ArtifactKindExplore, Payload: map[string]any{}},
	})
	eval := gate.EvaluateGate(pg, "code", artifacts)
	if eval.Passed {
		t.Fatal("expected empty map to fail not_empty validator")
	}
}

// ---------------------------------------------------------------------------
// EvaluateGateSequence
// ---------------------------------------------------------------------------

func TestEvaluateGateSequence_AllPass(t *testing.T) {
	gates := []gate.PhaseGate{
		{
			From: gate.PhaseExplore,
			To:   gate.PhasePlan,
			DefaultGate: gate.ArtifactGate{
				RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
			},
			OnFail: gate.GateFailBlock,
		},
		{
			From: gate.PhasePlan,
			To:   gate.PhaseEdit,
			DefaultGate: gate.ArtifactGate{
				RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindPlan},
			},
			OnFail: gate.GateFailBlock,
		},
	}
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "e1", Kind: euclotypes.ArtifactKindExplore},
		{ID: "p1", Kind: euclotypes.ArtifactKindPlan},
	})
	evals, allPassed := gate.EvaluateGateSequence(gates, "code", artifacts)
	if !allPassed {
		t.Fatalf("expected all gates to pass, evals: %+v", evals)
	}
	if len(evals) != 2 {
		t.Fatalf("expected 2 evaluations, got %d", len(evals))
	}
}

func TestEvaluateGateSequence_BlockOnFirstFail(t *testing.T) {
	gates := []gate.PhaseGate{
		{
			From: gate.PhaseExplore,
			To:   gate.PhasePlan,
			DefaultGate: gate.ArtifactGate{
				RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
			},
			OnFail: gate.GateFailBlock,
		},
		{
			From: gate.PhasePlan,
			To:   gate.PhaseEdit,
			DefaultGate: gate.ArtifactGate{
				RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindPlan},
			},
			OnFail: gate.GateFailBlock,
		},
	}
	// Neither artifact present
	artifacts := euclotypes.NewArtifactState(nil)
	evals, allPassed := gate.EvaluateGateSequence(gates, "code", artifacts)
	if allPassed {
		t.Fatal("expected sequence to fail")
	}
	// Should stop at first failure
	if len(evals) != 1 {
		t.Fatalf("expected sequence to stop after first block, got %d evals", len(evals))
	}
}

func TestEvaluateGateSequence_WarnContinues(t *testing.T) {
	gates := []gate.PhaseGate{
		{
			From: gate.PhaseReview,
			To:   gate.PhaseSummarize,
			DefaultGate: gate.ArtifactGate{
				RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindAnalyze},
			},
			OnFail: gate.GateFailWarn, // warn — continues even on failure
		},
		{
			From: gate.PhaseSummarize,
			To:   gate.PhaseReport,
			DefaultGate: gate.ArtifactGate{}, // empty gate always passes
			OnFail:      gate.GateFailBlock,
		},
	}
	// analyze missing — first gate fails with warn, second gate passes
	artifacts := euclotypes.NewArtifactState(nil)
	evals, allPassed := gate.EvaluateGateSequence(gates, "review", artifacts)
	if !allPassed {
		t.Fatalf("expected sequence to pass (warn policy continues), evals: %+v", evals)
	}
	if len(evals) != 2 {
		t.Fatalf("expected 2 evaluations (warn continues), got %d", len(evals))
	}
}

func TestEvaluateGateSequence_SkipContinues(t *testing.T) {
	gates := []gate.PhaseGate{
		{
			From: gate.PhaseExplore,
			To:   gate.PhasePlan,
			DefaultGate: gate.ArtifactGate{
				RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindAnalyze},
			},
			OnFail: gate.GateFailSkip,
		},
		{
			From: gate.PhasePlan,
			To:   gate.PhaseEdit,
			DefaultGate: gate.ArtifactGate{},
			OnFail:      gate.GateFailBlock,
		},
	}
	artifacts := euclotypes.NewArtifactState(nil)
	evals, allPassed := gate.EvaluateGateSequence(gates, "code", artifacts)
	if !allPassed {
		t.Fatalf("expected skip policy to continue, evals: %+v", evals)
	}
	if len(evals) != 2 {
		t.Fatalf("expected 2 evaluations (skip continues), got %d", len(evals))
	}
}

func TestEvaluateGateSequence_DefaultPolicyBlocks(t *testing.T) {
	// OnFail not set (zero value "") behaves like block
	gates := []gate.PhaseGate{
		{
			From: gate.PhaseExplore,
			To:   gate.PhasePlan,
			DefaultGate: gate.ArtifactGate{
				RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindAnalyze},
			},
			// OnFail is zero — treated as default block
		},
	}
	artifacts := euclotypes.NewArtifactState(nil)
	_, allPassed := gate.EvaluateGateSequence(gates, "code", artifacts)
	if allPassed {
		t.Fatal("expected zero OnFail policy to block")
	}
}

func TestEvaluateGateSequence_Empty(t *testing.T) {
	_, allPassed := gate.EvaluateGateSequence(nil, "code", euclotypes.NewArtifactState(nil))
	if !allPassed {
		t.Fatal("expected empty gate sequence to pass")
	}
}

// ---------------------------------------------------------------------------
// DefaultPhaseGates
// ---------------------------------------------------------------------------

func TestDefaultPhaseGates_ContainsAllProfiles(t *testing.T) {
	gates := gate.DefaultPhaseGates()
	profiles := []string{
		"edit_verify_repair",
		"reproduce_localize_patch",
		"test_driven_generation",
		"review_suggest_implement",
		"plan_stage_execute",
		"trace_execute_analyze",
	}
	for _, p := range profiles {
		if _, ok := gates[p]; !ok {
			t.Errorf("expected profile %q in DefaultPhaseGates", p)
		}
	}
}

func TestDefaultPhaseGates_EditVerifyRepair_PassesWithFullArtifacts(t *testing.T) {
	gates := gate.DefaultPhaseGates()["edit_verify_repair"]
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "e1", Kind: euclotypes.ArtifactKindExplore, Payload: map[string]any{"files": "main.go"}},
		{ID: "p1", Kind: euclotypes.ArtifactKindPlan},
		{ID: "ed1", Kind: euclotypes.ArtifactKindEditIntent},
	})
	for _, g := range gates {
		eval := gate.EvaluateGate(g, "code", artifacts)
		if !eval.Passed {
			t.Errorf("edit_verify_repair gate %v→%v failed: missing=%v", g.From, g.To, eval.Missing)
		}
	}
}

func TestDefaultPhaseGates_ReviewSuggestImplement_PassesWithFindings(t *testing.T) {
	gates := gate.DefaultPhaseGates()["review_suggest_implement"]
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "r1", Kind: euclotypes.ArtifactKindReviewFindings},
	})
	for _, g := range gates {
		eval := gate.EvaluateGate(g, "review", artifacts)
		if !eval.Passed {
			t.Errorf("review gate %v→%v failed: %v", g.From, g.To, eval.Missing)
		}
	}
}

func TestDefaultPhaseGates_PlanStageExecute_PassesWithPlan(t *testing.T) {
	gates := gate.DefaultPhaseGates()["plan_stage_execute"]
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "p1", Kind: euclotypes.ArtifactKindPlan},
		{ID: "ed1", Kind: euclotypes.ArtifactKindEditIntent},
	})
	for _, g := range gates {
		eval := gate.EvaluateGate(g, "planning", artifacts)
		if !eval.Passed {
			t.Errorf("plan_stage_execute gate %v→%v failed: %v", g.From, g.To, eval.Missing)
		}
	}
}

func TestDefaultPhaseGates_TraceExecuteAnalyze_PassesWithTrace(t *testing.T) {
	gates := gate.DefaultPhaseGates()["trace_execute_analyze"]
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "t1", Kind: euclotypes.ArtifactKindTrace},
	})
	for _, g := range gates {
		eval := gate.EvaluateGate(g, "debug", artifacts)
		if !eval.Passed {
			t.Errorf("trace_execute_analyze gate %v→%v failed: %v", g.From, g.To, eval.Missing)
		}
	}
}

// ---------------------------------------------------------------------------
// isEmpty edge cases: []any, []map[string]any, []euclotypes.Artifact, non-map extractField
// ---------------------------------------------------------------------------

func TestEvaluateGate_ValidatorNotEmpty_EmptyAnySlice(t *testing.T) {
	pg := gate.PhaseGate{
		From: gate.PhaseExplore,
		To:   gate.PhasePlan,
		DefaultGate: gate.ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
			Validators: []gate.ArtifactValidator{
				{Kind: euclotypes.ArtifactKindExplore, Field: "items", Check: gate.ValidatorCheckNotEmpty},
			},
		},
		OnFail: gate.GateFailBlock,
	}
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "e1", Kind: euclotypes.ArtifactKindExplore, Payload: map[string]any{"items": []any{}}},
	})
	eval := gate.EvaluateGate(pg, "code", artifacts)
	if eval.Passed {
		t.Fatal("expected empty []any to fail not_empty")
	}
}

func TestEvaluateGate_ValidatorNotEmpty_EmptyMapSlice(t *testing.T) {
	pg := gate.PhaseGate{
		From: gate.PhaseExplore,
		To:   gate.PhasePlan,
		DefaultGate: gate.ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
			Validators: []gate.ArtifactValidator{
				{Kind: euclotypes.ArtifactKindExplore, Field: "edits", Check: gate.ValidatorCheckNotEmpty},
			},
		},
		OnFail: gate.GateFailBlock,
	}
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "e1", Kind: euclotypes.ArtifactKindExplore, Payload: map[string]any{"edits": []map[string]any{}}},
	})
	eval := gate.EvaluateGate(pg, "code", artifacts)
	if eval.Passed {
		t.Fatal("expected empty []map[string]any to fail not_empty")
	}
}

func TestEvaluateGate_ValidatorNotEmpty_EmptyArtifactSlice(t *testing.T) {
	pg := gate.PhaseGate{
		From: gate.PhaseExplore,
		To:   gate.PhasePlan,
		DefaultGate: gate.ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
			Validators: []gate.ArtifactValidator{
				{Kind: euclotypes.ArtifactKindExplore, Field: "refs", Check: gate.ValidatorCheckNotEmpty},
			},
		},
		OnFail: gate.GateFailBlock,
	}
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "e1", Kind: euclotypes.ArtifactKindExplore, Payload: map[string]any{
			"refs": []euclotypes.Artifact{},
		}},
	})
	eval := gate.EvaluateGate(pg, "code", artifacts)
	if eval.Passed {
		t.Fatal("expected empty []euclotypes.Artifact to fail not_empty")
	}
}

func TestEvaluateGate_ValidatorExtractField_NonMapPayload(t *testing.T) {
	// extractField on a non-map payload returns nil → isEmpty(nil)=true
	pg := gate.PhaseGate{
		From: gate.PhaseExplore,
		To:   gate.PhasePlan,
		DefaultGate: gate.ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
			Validators: []gate.ArtifactValidator{
				{Kind: euclotypes.ArtifactKindExplore, Field: "status", Check: gate.ValidatorCheckNotEmpty},
			},
		},
		OnFail: gate.GateFailBlock,
	}
	// Payload is a string (not a map) — extractField returns nil
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "e1", Kind: euclotypes.ArtifactKindExplore, Payload: "plain string"},
	})
	eval := gate.EvaluateGate(pg, "code", artifacts)
	if eval.Passed {
		t.Fatal("expected not_empty to fail when extractField returns nil from non-map payload")
	}
}

func TestEvaluateGate_ValidatorMinCount_Int64Value(t *testing.T) {
	// toInt with int64 value
	pg := gate.PhaseGate{
		From: gate.PhasePlan,
		To:   gate.PhaseEdit,
		DefaultGate: gate.ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindPlan},
			Validators: []gate.ArtifactValidator{
				{Kind: euclotypes.ArtifactKindPlan, Field: "steps", Check: gate.ValidatorCheckMinCount, Value: int64(2)},
			},
		},
		OnFail: gate.GateFailBlock,
	}
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "p1", Kind: euclotypes.ArtifactKindPlan, Payload: map[string]any{
			"steps": []any{"a", "b", "c"},
		}},
	})
	eval := gate.EvaluateGate(pg, "code", artifacts)
	if !eval.Passed {
		t.Fatalf("expected min_count with int64(2) to pass: %v", eval.FailedValidators)
	}
}

func TestEvaluateGate_ValidatorMinCount_Float64Value(t *testing.T) {
	// toInt with float64 value
	pg := gate.PhaseGate{
		From: gate.PhasePlan,
		To:   gate.PhaseEdit,
		DefaultGate: gate.ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindPlan},
			Validators: []gate.ArtifactValidator{
				{Kind: euclotypes.ArtifactKindPlan, Field: "steps", Check: gate.ValidatorCheckMinCount, Value: float64(1)},
			},
		},
		OnFail: gate.GateFailBlock,
	}
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "p1", Kind: euclotypes.ArtifactKindPlan, Payload: map[string]any{
			"steps": []any{"x"},
		}},
	})
	eval := gate.EvaluateGate(pg, "code", artifacts)
	if !eval.Passed {
		t.Fatalf("expected min_count with float64(1) to pass: %v", eval.FailedValidators)
	}
}

func TestDefaultPhaseGates_ReproduceLocalizePatch_DebugModeValidator(t *testing.T) {
	gates := gate.DefaultPhaseGates()["reproduce_localize_patch"]
	// Debug mode requires non-empty explore and non-empty status field
	artifactsMissingStatus := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "e1", Kind: euclotypes.ArtifactKindExplore, Payload: map[string]any{"other": "value"}},
	})
	eval := gate.EvaluateGate(gates[0], "debug", artifactsMissingStatus)
	if eval.Passed {
		t.Fatal("expected debug mode gate to fail without status field in explore artifact")
	}

	artifactsWithStatus := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "e1", Kind: euclotypes.ArtifactKindExplore, Payload: map[string]any{"status": "reproduced"}},
	})
	eval2 := gate.EvaluateGate(gates[0], "debug", artifactsWithStatus)
	if !eval2.Passed {
		t.Fatalf("expected debug mode gate to pass with status field: %v", eval2.FailedValidators)
	}
}
