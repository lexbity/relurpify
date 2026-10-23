package critic_test

import (
	"strings"
	"testing"

	// Blank-import react so its paradigm contract registers in this test
	// binary: the real runtime always links both packages (thoughtrecipes
	// imports every cognitionzoo paradigm), and Composes resolution is only
	// meaningful against the fully-populated registry.
	_ "codeburg.org/lexbit/relurpify/cognitionzoo/react"

	// Blank-import critic so its init registers the contract in this
	// binary too — the same mechanism the runtime relies on.
	_ "codeburg.org/lexbit/relurpify/cognitionzoo/critic"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
)

func TestCriticContractRegistered(t *testing.T) {
	c, ok := paradigm.Registry.Lookup("critic")
	if !ok || c == nil {
		t.Fatal("expected critic paradigm contract to be registered")
	}
	if c.Shape != paradigm.ShapeReviewLoop {
		t.Fatalf("critic shape = %q, want %q", c.Shape, paradigm.ShapeReviewLoop)
	}
	if len(c.Composes) != 1 || c.Composes[0] != "react" {
		t.Fatalf("critic composes = %v, want [react]", c.Composes)
	}
	want := []string{"critique", "revise"}
	got := c.DirectiveNames()
	if len(got) != len(want) {
		t.Fatalf("critic directive names = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("critic directive names = %v, want %v", got, want)
		}
	}
	reviseSpec, ok := c.Directive("revise")
	if !ok || !reviseSpec.Predicate || len(reviseSpec.Requires) != 1 || reviseSpec.Requires[0] != "critique" {
		t.Fatalf("critic revise spec = %#v, want predicate + Requires [critique]", reviseSpec)
	}
	if c.OrderRule() == nil {
		t.Fatal("expected critic to declare an order rule")
	}
	if required := c.RequiredNames(); len(required) != 0 {
		t.Fatalf("critic required directives = %v, want none (goal-only recipes preserved)", required)
	}
}

// TestContractCompleteness mirrors the shipped paradigms' own completeness
// tests: every declared directive is pinned by a conformance case, no case
// pins an undeclared directive, and every Composes entry resolves in the
// fully-populated registry.
func TestContractCompleteness(t *testing.T) {
	c, ok := paradigm.Registry.Lookup("critic")
	if !ok || c == nil {
		t.Fatal("expected critic paradigm contract to be registered")
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
			t.Errorf("critic composes %q, which has no registered contract", composed)
		}
	}
}

// TestRegistrationValidatesIntrinsics drives a scoped registry through the
// refusals the process-wide Register applies: the grammar as data is
// validated before it is trusted.
func TestRegistrationValidatesIntrinsics(t *testing.T) {
	base := paradigm.Contract{
		Paradigm: "critic-lab",
		Shape:    paradigm.ShapeReviewLoop,
		Directives: []paradigm.DirectiveSpec{
			{Name: "critique", Form: paradigm.FormLine, Text: paradigm.ArgOne},
		},
		Conformance: []paradigm.ConformanceCase{
			{ID: "lab/critique", Directive: "critique", Assertion: "verdict lands in scratch"},
		},
	}

	t.Run("unknown shape", func(t *testing.T) {
		bad := base
		bad.Shape = paradigm.ExecutionShape("Whirlpool")
		err := register(t, bad)
		if err == nil || !strings.Contains(err.Error(), "unknown execution shape") {
			t.Fatalf("err = %v, want unknown execution shape", err)
		}
	})
	t.Run("duplicate directive", func(t *testing.T) {
		bad := base
		bad.Directives = append(bad.Directives, bad.Directives[0])
		err := register(t, bad)
		if err == nil || !strings.Contains(err.Error(), "duplicate directive") {
			t.Fatalf("err = %v, want duplicate directive", err)
		}
	})
	t.Run("requires undeclared directive", func(t *testing.T) {
		bad := base
		bad.Directives = append(bad.Directives, paradigm.DirectiveSpec{
			Name: "revise", Form: paradigm.FormBlock, Text: paradigm.ArgNone,
			Requires: []string{"approve"},
		})
		err := register(t, bad)
		if err == nil || !strings.Contains(err.Error(), `requires undeclared directive "approve"`) {
			t.Fatalf("err = %v, want requires undeclared directive", err)
		}
	})
	t.Run("order rule references undeclared directive", func(t *testing.T) {
		bad := base
		bad.Order = &paradigm.OrderRule{Sequence: []string{"critique", "polish"}}
		err := register(t, bad)
		if err == nil || !strings.Contains(err.Error(), "order rule references undeclared directive") {
			t.Fatalf("err = %v, want order rule references undeclared directive", err)
		}
	})
	t.Run("case pins undeclared directive", func(t *testing.T) {
		bad := base
		bad.Conformance = append(bad.Conformance, paradigm.ConformanceCase{
			ID: "lab/ghost", Directive: "polish", Assertion: "nothing",
		})
		err := register(t, bad)
		if err == nil || !strings.Contains(err.Error(), "pins undeclared directive") {
			t.Fatalf("err = %v, want pins undeclared directive", err)
		}
	})
	t.Run("declared directive without a case", func(t *testing.T) {
		bad := base
		bad.Directives = append(bad.Directives, paradigm.DirectiveSpec{
			Name: "revise", Form: paradigm.FormBlock, Text: paradigm.ArgNone,
		})
		err := register(t, bad)
		if err == nil || !strings.Contains(err.Error(), `directive "revise" has no conformance case`) {
			t.Fatalf("err = %v, want has no conformance case", err)
		}
	})
	t.Run("duplicate registration", func(t *testing.T) {
		reg := paradigm.NewContractRegistry()
		if err := reg.Register(base); err != nil {
			t.Fatalf("first Register: %v", err)
		}
		err := reg.Register(base)
		if err == nil || !strings.Contains(err.Error(), "duplicate paradigm contract registration") {
			t.Fatalf("err = %v, want duplicate registration", err)
		}
	})
}

// register runs Register against a scoped registry so the process-wide
// collection is untouched and a panic-capable failure stays a test error.
func register(t *testing.T, c paradigm.Contract) error {
	t.Helper()
	return paradigm.NewContractRegistry().Register(c)
}
