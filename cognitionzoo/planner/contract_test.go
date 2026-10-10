package planner

import (
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
)

// TestPlannerContractRegistered pins the contract present at load time.
func TestPlannerContractRegistered(t *testing.T) {
	c, ok := paradigm.Registry.Lookup("planner")
	if !ok || c == nil {
		t.Fatal("expected planner paradigm contract to be registered")
	}
	if c.Shape != paradigm.ShapePlanExecute {
		t.Fatalf("planner shape = %q, want %q", c.Shape, paradigm.ShapePlanExecute)
	}
	// The plan/step/verify/summarize vocabularies are restored as honored
	// runner semantics (Wave 3). `plan` is deliberately not Required so a
	// goal-only planner recipe keeps the library behavior (FR-9); `step`
	// Requires `plan` (D1 mixing rule).
	want := []string{"plan", "step", "verify", "summarize"}
	got := c.DirectiveNames()
	if len(got) != len(want) {
		t.Fatalf("planner directive names = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("planner directive names = %v, want %v", got, want)
		}
	}
	if c.OrderRule() == nil {
		t.Fatal("expected planner to declare an order rule")
	}
	stepSpec, ok := c.Directive("step")
	if !ok || len(stepSpec.Requires) != 1 || stepSpec.Requires[0] != "plan" {
		t.Fatalf("planner step spec Requires = %v, want [plan]", stepSpec.Requires)
	}
	if required := c.RequiredNames(); len(required) != 0 {
		t.Fatalf("planner required directives = %v, want none (goal-only recipes preserved)", required)
	}
}

// TestContractCompleteness asserts the contract declares exactly the directive
// set it pins: every directive has a conformance case, every case pins a
// declared directive, and every Composes entry resolves in the registry.
func TestContractCompleteness(t *testing.T) {
	c, ok := paradigm.Registry.Lookup("planner")
	if !ok || c == nil {
		t.Fatal("expected planner paradigm contract to be registered")
	}
	declared := map[string]bool{}
	for _, spec := range c.Directives {
		declared[spec.Name] = true
	}
	pinned := map[string]bool{}
	for _, cas := range c.Conformance {
		if strings.TrimSpace(cas.ID) == "" {
			t.Errorf("conformance case with empty ID")
		}
		if strings.TrimSpace(cas.Directive) == "" {
			continue
		}
		if !declared[cas.Directive] {
			t.Errorf("case %q pins undeclared directive %q", cas.ID, cas.Directive)
			continue
		}
		pinned[cas.Directive] = true
	}
	for _, spec := range c.Directives {
		if !pinned[spec.Name] {
			t.Errorf("directive %q has no conformance case", spec.Name)
		}
	}
	for _, composed := range c.Composes {
		if _, ok := paradigm.Registry.Lookup(composed); !ok {
			t.Errorf("planner composes %q, which has no registered contract", composed)
		}
	}
}
