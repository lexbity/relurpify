package thoughtrecipe

import (
	"testing"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
)

// TestLowerTypedDirectivesPinsShape pins the typed lowering of directive
// clauses and blocks: name, text arguments in source order, span, predicate
// (block form), and nested block directives in the typed Body. Non-directive
// children (do/capture/run) are structural and must NOT leak into the typed
// directive tree.
func TestLowerTypedDirectivesPinsShape(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe typed_directives
"Typed directive lowering."

agent reviewer uses planner

run reviewer:
  plan "Review the code."
  step:
    do relurpic:code_review
    summarize state.findings
  verify state.outcome
`)
	run := findRunDecl(t, doc)
	directives, err := lowerDirectivesForPlanTest(run.Items)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if len(directives) != 3 {
		t.Fatalf("directive count = %d, want 3", len(directives))
	}

	plan := directives[0]
	if plan.Name != "plan" {
		t.Fatalf("directive[0].Name = %q, want plan", plan.Name)
	}
	if len(plan.TextArgs) != 1 || plan.TextArgs[0] != `"Review the code."` {
		t.Fatalf("directive[0].TextArgs = %#v, want [\"Review the code.\"]", plan.TextArgs)
	}
	if plan.Predicate != nil {
		t.Fatal("directive[0] must not carry a predicate")
	}
	if len(plan.Body) != 0 {
		t.Fatalf("directive[0].Body = %#v, want empty (line form)", plan.Body)
	}

	step := directives[1]
	if step.Name != "step" {
		t.Fatalf("directive[1].Name = %q, want step", step.Name)
	}
	if len(step.Body) != 2 {
		t.Fatalf("directive[1].Body = %#v, want the nested do and summarize items", step.Body)
	}
	if step.Body[0].Name != "do" {
		t.Fatalf("first nested item name = %q, want do", step.Body[0].Name)
	}
	if step.Body[1].Name != "summarize" {
		t.Fatalf("second nested item name = %q, want summarize", step.Body[1].Name)
	}
	if len(step.Body[1].TextArgs) != 1 || step.Body[1].TextArgs[0] != "state.findings" {
		t.Fatalf("nested directive text args = %#v, want [state.findings]", step.Body[1].TextArgs)
	}

	verify := directives[2]
	if verify.Name != "verify" {
		t.Fatalf("directive[2].Name = %q, want verify", verify.Name)
	}
	if len(verify.TextArgs) != 1 || verify.TextArgs[0] != "state.outcome" {
		t.Fatalf("directive[2].TextArgs = %#v, want [state.outcome]", verify.TextArgs)
	}
}

// TestLowerTypedDirectiveSpansPinSource: every lowered directive carries the
// source span of its clause, and nested directives carry their own spans.
func TestLowerTypedDirectiveSpansPinSource(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe typed_spans
"Span pinning."

agent reviewer uses planner

run reviewer:
  plan "Review."
  step:
    summarize state.findings
`)
	run := findRunDecl(t, doc)
	directives, err := lowerDirectivesForPlanTest(run.Items)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if directives[0].Span.Start.File != "demo.euclo" || directives[0].Span.Start.Line != 7 {
		t.Fatalf("plan directive span = %+v, want demo.euclo:7", directives[0].Span)
	}
	if directives[1].Span.Start.Line != 8 {
		t.Fatalf("step directive span line = %d, want 8", directives[1].Span.Start.Line)
	}
	if directives[1].Body[0].Span.Start.Line != 9 {
		t.Fatalf("nested summarize span line = %d, want 9", directives[1].Body[0].Span.Start.Line)
	}
}

// TestLowerTypedDirectiveNotFoundReturnsEmpty pins the fail-closed behavior of
// lowering a non-directive execution item: the typed form is the zero value.
func TestLowerTypedDirectiveNotFoundReturnsEmpty(t *testing.T) {
	got := lowerTypedDirective(&GoalClause{})
	if got.Name != "" || len(got.TextArgs) != 0 || got.Predicate != nil || len(got.Body) != 0 {
		t.Fatalf("lowerTypedDirective(non-directive) = %#v, want zero value", got)
	}
}

// TestDirectivePayloadAccessors pins the only allowed directive accessors.
func TestDirectivePayloadAccessors(t *testing.T) {
	directives := []TypedDirective{
		{Name: "plan", TextArgs: []string{`"Review."`}},
		{Name: "step", Body: []TypedDirective{{Name: "summarize", TextArgs: []string{"state.x"}}}},
	}
	if !Has(directives, "plan") || !Has(directives, "step") {
		t.Fatal("Has must report present directives")
	}
	if Has(directives, "nope") {
		t.Fatal("Has must report absent directives")
	}
	if got := DirectiveText(directives, "plan"); len(got) != 1 || got[0] != `"Review."` {
		t.Fatalf("DirectiveText(plan) = %#v", got)
	}
	if got := DirectiveText(directives, "nope"); got != nil {
		t.Fatalf("DirectiveText(nope) = %#v, want nil", got)
	}
	if got := DirectivePredicate(directives, "plan"); got != nil {
		t.Fatalf("DirectivePredicate(plan) = %#v, want nil", got)
	}
	if got := DirectivePredicate(directives, "nope"); got != nil {
		t.Fatalf("DirectivePredicate(nope) = %#v, want nil", got)
	}
}

// TestDirectiveNamesTopLevelOnly: the flat name projection is the top-level
// directive surface (matching the pre-typed execution_directives shape);
// nested block directives are reached through their parent's Body.
func TestDirectiveNamesTopLevelOnly(t *testing.T) {
	directives := []TypedDirective{
		{Name: "plan"},
		{Name: "step", Body: []TypedDirective{{Name: "summarize"}, {Name: "verify"}}},
	}
	got := DirectiveNames(directives)
	if len(got) != 2 || got[0] != "plan" || got[1] != "step" {
		t.Fatalf("DirectiveNames = %#v, want [plan step]", got)
	}
	if got := DirectiveNames(nil); len(got) != 0 {
		t.Fatalf("DirectiveNames(nil) = %#v, want empty", got)
	}
}

// TestLowerTypedDirectivesCoverContractRegistry ties lowering coverage to the
// contract registry: every directive a paradigm contract declares must lower
// to a typed directive that preserves its name. A directive added to a
// contract without lowering coverage fails here.
func TestLowerTypedDirectivesCoverContractRegistry(t *testing.T) {
	contracts := paradigm.Registry.All()
	if len(contracts) == 0 {
		t.Fatal("expected registered paradigm contracts")
	}
	for _, contract := range contracts {
		for _, spec := range contract.Directives {
			clause := &DirectiveClause{
				positioned: positioned{Span: NewSpan("test", 1, 1, 1, 2)},
				Name:       Identifier{positioned: positioned{Span: NewSpan("test", 1, 1, 1, 2)}, Value: spec.Name},
			}
			got := lowerTypedDirective(clause)
			if got.Name != spec.Name {
				t.Errorf("%s: lowerTypedDirective(%q).Name = %q", contract.Paradigm, spec.Name, got.Name)
			}
			if got.Span.Start.Line != 1 || got.Span.Start.File != "test" {
				t.Errorf("%s: directive %q span not preserved: %+v", contract.Paradigm, spec.Name, got.Span)
			}
		}
	}
}

// findRunDecl returns the first top-level run declaration in the document.
func findRunDecl(t *testing.T, doc *ThoughtRecipeDocument) *RunDecl {
	t.Helper()
	for _, decl := range doc.Declarations {
		if run, ok := decl.(*RunDecl); ok {
			return run
		}
	}
	t.Fatal("no run declaration found")
	return nil
}

// lowerDirectivesForPlanTest lowers the run items and returns the typed
// directives, failing on any lower error.
func lowerDirectivesForPlanTest(items []ExecutionItem) ([]TypedDirective, error) {
	_, _, directives, _, _, _, _, _, _, err := lowerRunItems(items)
	return directives, err
}
