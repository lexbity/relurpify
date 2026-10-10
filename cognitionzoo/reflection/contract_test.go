package reflection

import (
	"strings"
	"testing"

	// Blank-import react so its paradigm contract registers in this test
	// binary: the real runtime always links both packages (thoughtrecipes
	// imports every cognitionzoo paradigm), and Composes resolution is only
	// meaningful against the fully-populated registry.
	_ "codeburg.org/lexbit/relurpify/cognitionzoo/react"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
)

func TestReflectionContractRegistered(t *testing.T) {
	c, ok := paradigm.Registry.Lookup("reflection")
	if !ok || c == nil {
		t.Fatal("expected reflection paradigm contract to be registered")
	}
	if c.Shape != paradigm.ShapeReviewLoop {
		t.Fatalf("reflection shape = %q, want %q", c.Shape, paradigm.ShapeReviewLoop)
	}
	if len(c.Composes) != 1 || c.Composes[0] != "react" {
		t.Fatalf("reflection composes = %v, want [react]", c.Composes)
	}
	want := []string{"review", "revise"}
	got := c.DirectiveNames()
	if len(got) != len(want) {
		t.Fatalf("reflection directive names = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("reflection directive names = %v, want %v", got, want)
		}
	}
	reviseSpec, ok := c.Directive("revise")
	if !ok || !reviseSpec.Predicate || len(reviseSpec.Requires) != 1 || reviseSpec.Requires[0] != "review" {
		t.Fatalf("reflection revise spec = %#v, want predicate + Requires [review]", reviseSpec)
	}
	if c.OrderRule() == nil {
		t.Fatal("expected reflection to declare an order rule")
	}
	if required := c.RequiredNames(); len(required) != 0 {
		t.Fatalf("reflection required directives = %v, want none (goal-only recipes preserved)", required)
	}
}

func TestContractCompleteness(t *testing.T) {
	c, ok := paradigm.Registry.Lookup("reflection")
	if !ok || c == nil {
		t.Fatal("expected reflection paradigm contract to be registered")
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
			t.Errorf("reflection composes %q, which has no registered contract", composed)
		}
	}
}
