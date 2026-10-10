package rewoo

import (
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
)

func TestRewooContractRegistered(t *testing.T) {
	c, ok := paradigm.Registry.Lookup("rewoo")
	if !ok || c == nil {
		t.Fatal("expected rewoo paradigm contract to be registered")
	}
	if c.Shape != paradigm.ShapeStagedPlan {
		t.Fatalf("rewoo shape = %q, want %q", c.Shape, paradigm.ShapeStagedPlan)
	}
	want := []string{"plan", "step", "synthesize"}
	got := c.DirectiveNames()
	if len(got) != len(want) {
		t.Fatalf("rewoo directive names = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rewoo directive names = %v, want %v", got, want)
		}
	}
	if c.OrderRule() == nil {
		t.Fatal("expected rewoo to declare an order rule")
	}
}

func TestContractCompleteness(t *testing.T) {
	c, ok := paradigm.Registry.Lookup("rewoo")
	if !ok || c == nil {
		t.Fatal("expected rewoo paradigm contract to be registered")
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
			t.Errorf("rewoo composes %q, which has no registered contract", composed)
		}
	}
}
