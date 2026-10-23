package critic_test

import (
	"strings"
	"testing"

	// Blank-import react so the fully-populated registry mirrors the
	// runtime binary (thoughtrecipes links every paradigm).
	_ "codeburg.org/lexbit/relurpify/cognitionzoo/react"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	thoughtrecipe "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

// TestRecipeAddressesCritic is the Chapter 9 loader-level proof: the
// recipe text parses, validates against the contract registry (which the
// critic package's init populated), and lowers to a plan bound to the
// critic paradigm.
const criticLoopRecipe = `thoughtrecipe critic_loop
"Critique loop: a react draft, an independent critique phase, and a bounded conditional revision delegated to a react patcher."

trigger as capability:
  may read workspace

agent fixer uses critic
agent patcher uses react

run fixer:
  from input.prompt
  goal "Produce the work, then submit it for independent critique."
  critique "Check error handling and naming."
  revise when scratch.critic.verdict contains issues:
    delegate to patcher:
      goal "Apply every issue the critique named."
`

func TestRecipeAddressesCritic(t *testing.T) {
	doc, err := thoughtrecipe.ParseSource("critic_loop.erpe", criticLoopRecipe)
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}

	// Load-time validation against the process-wide registry: `uses critic`
	// resolves, and both directives are declared by the contract.
	if errs := thoughtrecipe.ValidateAgainstContracts(doc, paradigm.Registry); len(errs) != 0 {
		for _, err := range errs {
			t.Errorf("ValidateAgainstContracts: %v", err)
		}
		t.Fatal("recipe does not load against the populated registry")
	}

	plan, err := thoughtrecipe.LowerDocument(doc)
	if err != nil {
		t.Fatalf("LowerDocument: %v", err)
	}
	found := false
	for _, step := range plan.Steps {
		if strings.EqualFold(step.Paradigm, "critic") {
			found = true
			if len(step.Directives) != 2 {
				t.Fatalf("critic step directives = %d, want 2 (critique, revise)", len(step.Directives))
			}
			if len(step.ReviseBody) == 0 {
				t.Fatal("critic step revise body did not lower")
			}
		}
	}
	if !found {
		t.Fatal("lowered plan contains no critic step")
	}
}

// TestRecipeRejectsUndeclaredDirective is the parity mirror: the same
// loader that accepts `critique` rejects a directive the contract does not
// declare — with the valid set named.
func TestRecipeRejectsUndeclaredDirective(t *testing.T) {
	bogus := strings.Replace(criticLoopRecipe,
		`critique "Check error handling and naming."`,
		`polish "Check error handling and naming."`, 1)
	doc, err := thoughtrecipe.ParseSource("bogus.erpe", bogus)
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	errs := thoughtrecipe.ValidateAgainstContracts(doc, paradigm.Registry)
	if len(errs) == 0 {
		t.Fatal("expected a load error for the undeclared directive")
	}
	joined := ""
	for _, err := range errs {
		joined += err.Error() + "\n"
	}
	if !strings.Contains(joined, "polish") || !strings.Contains(joined, "critique") {
		t.Fatalf("errors = %q, want the directive named with the valid set", joined)
	}
}
