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
	// The plan/step/verify/summarize vocabularies were never consumed by the
	// planner runtime; per implement-or-delete they are retired, so the
	// contract declares no directive clauses. Any directive a recipe carries
	// in a planner run block is a load error, not a silent no-op.
	if got := c.DirectiveNames(); len(got) != 0 {
		t.Fatalf("planner directive names = %v, want none (retired no-op vocabularies)", got)
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
