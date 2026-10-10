package blackboard

import (
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
)

func TestBlackboardContractRegistered(t *testing.T) {
	c, ok := paradigm.Registry.Lookup("blackboard")
	if !ok || c == nil {
		t.Fatal("expected blackboard paradigm contract to be registered")
	}
	if c.Shape != paradigm.ShapeBlackboard {
		t.Fatalf("blackboard shape = %q, want %q", c.Shape, paradigm.ShapeBlackboard)
	}
	if got := c.DirectiveNames(); len(got) != 1 || got[0] != "source" {
		t.Fatalf("blackboard directive names = %v, want [source] (restored D6)", got)
	}
	if spec, ok := c.Directive("source"); !ok || !spec.Repeatable || spec.Required {
		t.Fatalf("source spec = %+v, want repeatable and not required (FR-9: built-in specialist loop stays the default)", spec)
	}
}

func TestContractCompleteness(t *testing.T) {
	c, ok := paradigm.Registry.Lookup("blackboard")
	if !ok || c == nil {
		t.Fatal("expected blackboard paradigm contract to be registered")
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
			t.Errorf("blackboard composes %q, which has no registered contract", composed)
		}
	}
}
