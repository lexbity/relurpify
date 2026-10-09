package react

import (
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
)

// TestReactContractRegistered pins the pilot contract present at load time.
func TestReactContractRegistered(t *testing.T) {
	c, ok := paradigm.Registry.Lookup("react")
	if !ok || c == nil {
		t.Fatal("expected react paradigm contract to be registered")
	}
	if c.Shape != paradigm.ShapeIterative {
		t.Fatalf("react shape = %q, want %q", c.Shape, paradigm.ShapeIterative)
	}
	if got := c.DirectiveNames(); len(got) != 1 || got[0] != "until" {
		t.Fatalf("react directive names = %v, want [until]", got)
	}
}

// TestContractCompleteness asserts the contract declares exactly the directive
// set it pins: every directive has a conformance case, every case pins a
// declared directive, and every Composes entry resolves in the registry. A
// directive added without a case, or a case pinning an undeclared directive,
// fails here — this is the drift detector for the contract itself.
func TestContractCompleteness(t *testing.T) {
	c, ok := paradigm.Registry.Lookup("react")
	if !ok || c == nil {
		t.Fatal("expected react paradigm contract to be registered")
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
			t.Errorf("react composes %q, which has no registered contract", composed)
		}
	}
}
