package htn

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

func TestHTNContractRegistered(t *testing.T) {
	c, ok := paradigm.Registry.Lookup("htn")
	if !ok || c == nil {
		t.Fatal("expected htn paradigm contract to be registered")
	}
	if c.Shape != paradigm.ShapeDecomposition {
		t.Fatalf("htn shape = %q, want %q", c.Shape, paradigm.ShapeDecomposition)
	}
	if len(c.Composes) != 1 || c.Composes[0] != "react" {
		t.Fatalf("htn composes = %v, want [react]", c.Composes)
	}
	if got := c.DirectiveNames(); len(got) != 0 {
		t.Fatalf("htn directive names = %v, want none (retired no-op vocabularies)", got)
	}
}

func TestContractCompleteness(t *testing.T) {
	c, ok := paradigm.Registry.Lookup("htn")
	if !ok || c == nil {
		t.Fatal("expected htn paradigm contract to be registered")
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
			t.Errorf("htn composes %q, which has no registered contract", composed)
		}
	}
}
