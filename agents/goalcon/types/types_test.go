package types

import (
	"reflect"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
)

func TestWorldStateLifecycle(t *testing.T) {
	var nilState *WorldState
	nilState.Satisfy("noop")
	if nilState.IsSatisfied("noop") {
		t.Fatal("nil state should never satisfy predicates")
	}

	state := NewWorldState()
	if state == nil {
		t.Fatal("expected world state")
	}
	if state.IsSatisfied("ready") {
		t.Fatal("unexpected satisfied predicate")
	}

	state.Satisfy("ready")
	if !state.IsSatisfied("ready") {
		t.Fatal("expected predicate to be satisfied")
	}

	clone := state.Clone()
	if clone == state {
		t.Fatal("clone should be independent")
	}
	if !clone.IsSatisfied("ready") {
		t.Fatal("clone should preserve satisfied predicates")
	}
	clone.Satisfy("other")
	if state.IsSatisfied("other") {
		t.Fatal("changes to clone should not affect original")
	}
}

func TestBuildPlanAndHelpers(t *testing.T) {
	ops := []*Operator{
		{
			Name:          "inspect",
			Description:   "Inspect target",
			Effects:       []Predicate{"inspected"},
			DefaultParams: map[string]any{"dry_run": true},
		},
		{
			Name:          "fix",
			Description:   "Fix target",
			Preconditions: []Predicate{"inspected"},
			Effects:       []Predicate{"fixed"},
			DefaultParams: map[string]any{"force": false},
		},
	}

	plan := BuildPlan("repair the target", ops)
	if plan == nil {
		t.Fatal("expected plan")
	}
	if plan.Goal != "repair the target" {
		t.Fatalf("unexpected goal %q", plan.Goal)
	}
	if len(plan.Steps) != 2 {
		t.Fatalf("unexpected step count %d", len(plan.Steps))
	}
	if plan.Steps[0].ID != "goalcon_step_01_inspect" {
		t.Fatalf("unexpected first step id %q", plan.Steps[0].ID)
	}
	if plan.Steps[1].ID != "goalcon_step_02_fix" {
		t.Fatalf("unexpected second step id %q", plan.Steps[1].ID)
	}
	if got := plan.Dependencies[plan.Steps[1].ID]; len(got) != 1 || got[0] != plan.Steps[0].ID {
		t.Fatalf("unexpected dependencies: %+v", plan.Dependencies)
	}
	if !reflect.DeepEqual(plan.Steps[0].Params, map[string]any{"dry_run": true}) {
		t.Fatalf("unexpected first params: %+v", plan.Steps[0].Params)
	}
	if !reflect.DeepEqual(plan.Steps[1].Params, map[string]any{"force": false}) {
		t.Fatalf("unexpected second params: %+v", plan.Steps[1].Params)
	}

	if got := stepIDFor(9, &Operator{Name: "noop"}); got != "goalcon_step_10_noop" {
		t.Fatalf("unexpected step ID %q", got)
	}
	if got := stepIDFor(10, nil); got != "goalcon_step_11_op" {
		t.Fatalf("unexpected nil op step ID %q", got)
	}
	if got := twoDigit(1); got != "01" {
		t.Fatalf("unexpected twoDigit output %q", got)
	}
	if got := twoDigit(12); got != "12" {
		t.Fatalf("unexpected twoDigit output %q", got)
	}
	if got := operatorSatisfiesAny(nil, []Predicate{"x"}); got {
		t.Fatal("nil operator should not satisfy predicates")
	}
	if got := operatorSatisfiesAny(&Operator{Effects: []Predicate{"match"}}, []Predicate{"other", "match"}); !got {
		t.Fatal("expected operator to satisfy predicate")
	}
}

func TestOperatorRegistry(t *testing.T) {
	var nilRegistry *OperatorRegistry
	nilRegistry.Register(Operator{Name: "noop"})
	if nilRegistry.All() != nil {
		t.Fatal("nil registry should return nil list")
	}
	if nilRegistry.OperatorsSatisfying("x") != nil {
		t.Fatal("nil registry should return nil matches")
	}

	registry := NewOperatorRegistry()
	if registry == nil {
		t.Fatal("expected registry")
	}

	registry.Register(Operator{Name: "inspect", Effects: []Predicate{"inspected"}})
	registry.Register(Operator{Name: "fix", Effects: []Predicate{"fixed", "inspected"}})

	all := registry.All()
	if len(all) != 2 {
		t.Fatalf("unexpected registry size %d", len(all))
	}
	all = append(all, &Operator{Name: "extra"})
	if len(registry.All()) != 2 {
		t.Fatal("registry should return copy of slice, not shared backing array")
	}

	if got := registry.OperatorsSatisfying("inspected"); len(got) != 2 {
		t.Fatalf("unexpected operators satisfying inspected: %+v", got)
	}
	if got := registry.OperatorsSatisfying("fixed"); len(got) != 1 || got[0].Name != "fix" {
		t.Fatalf("unexpected fixed matches: %+v", got)
	}
	if got := registry.OperatorsSatisfying("missing"); len(got) != 0 {
		t.Fatalf("expected no matches, got %+v", got)
	}
}

func TestGoalConditionAndCoreCompatibility(t *testing.T) {
	cond := GoalCondition{Predicates: []Predicate{"a", "b"}, Description: "demo"}
	if len(cond.Predicates) != 2 || cond.Description != "demo" {
		t.Fatalf("unexpected goal condition: %+v", cond)
	}

	plan := &core.Plan{Goal: "x"}
	if plan.Goal != "x" {
		t.Fatal("expected plan compatibility")
	}
}
