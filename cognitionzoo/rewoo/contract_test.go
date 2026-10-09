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
	if got := c.DirectiveNames(); len(got) != 0 {
		t.Fatalf("rewoo directive names = %v, want none (retired no-op vocabularies)", got)
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
