package pipeline

import (
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
)

func TestPipelineContractRegistered(t *testing.T) {
	c, ok := paradigm.Registry.Lookup("pipeline")
	if !ok || c == nil {
		t.Fatal("expected pipeline paradigm contract to be registered")
	}
	if c.Shape != paradigm.ShapeLinear {
		t.Fatalf("pipeline shape = %q, want %q", c.Shape, paradigm.ShapeLinear)
	}
	if got := c.DirectiveNames(); len(got) != 0 {
		t.Fatalf("pipeline directive names = %v, want none (stage is grammar structure, not a directive)", got)
	}
	if len(c.Conformance) != 1 || c.Conformance[0].ID != "pipeline/stages_execute_in_order" {
		t.Fatalf("pipeline conformance cases = %#v, want the stages_execute_in_order shape case", c.Conformance)
	}
}

func TestContractCompleteness(t *testing.T) {
	c, ok := paradigm.Registry.Lookup("pipeline")
	if !ok || c == nil {
		t.Fatal("expected pipeline paradigm contract to be registered")
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
			t.Errorf("pipeline composes %q, which has no registered contract", composed)
		}
	}
}
