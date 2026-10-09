package thoughtrecipe

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
)

// The Phase 1 pilot registers exactly the react and planner contracts. These
// fixtures pin the load-time rejection and acceptance behavior; the remaining
// six paradigm contracts land in the Phase 3 slice.

func contractErrors(t *testing.T, src string) []error {
	t.Helper()
	doc := mustParseDoc(t, src)
	return ValidateAgainstContracts(doc, paradigm.Registry)
}

func contractErrorText(t *testing.T, src string) string {
	t.Helper()
	errs := contractErrors(t, src)
	if len(errs) == 0 {
		t.Fatalf("expected contract validation errors, got none")
	}
	var parts []string
	for _, err := range errs {
		parts = append(parts, err.Error())
	}
	return strings.Join(parts, "\n")
}

// AC-1: an unknown paradigm fails validation with a span and the valid names.
func TestValidateContractsRejectsUnknownParadigm(t *testing.T) {
	msg := contractErrorText(t, `thoughtrecipe demo
"Demo."

agent reviewer uses nosuchparadigm

run reviewer:
  goal "Review."
`)
	for _, want := range []string{
		"demo.euclo:4",
		"unsupported agent paradigm",
		"nosuchparadigm",
		"react",
		"planner",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error text %q missing %q", msg, want)
		}
	}
}

// AC-2: a directive the bound paradigm does not declare is a load error.
func TestValidateContractsRejectsDirectiveOutsideContract(t *testing.T) {
	msg := contractErrorText(t, `thoughtrecipe demo
"Demo."

agent reviewer uses planner

run reviewer:
  until 3
`)
	for _, want := range []string{
		"until",
		"not declared by the planner paradigm contract",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error text %q missing %q", msg, want)
		}
	}
}

// A declared directive on the matching paradigm validates clean.
func TestValidateContractsAcceptsDeclaredDirective(t *testing.T) {
	src := `thoughtrecipe demo
"Demo."

agent reviewer uses react

run reviewer:
  goal "Review."
  until 3
`
	if errs := contractErrors(t, src); len(errs) != 0 {
		t.Fatalf("expected no contract violations, got %v", errs)
	}
}

// A required directive that never appears is a load error. The fixture uses a
// scoped registry so the requirement is contract-declared, not a
// paradigm-internal assumption.
func TestValidateContractsRejectsMissingRequiredDirective(t *testing.T) {
	scoped := paradigm.NewContractRegistry()
	if err := scoped.Register(paradigm.Contract{
		Paradigm: "alpha",
		Summary:  "scoped test paradigm",
		Shape:    paradigm.ShapeLinear,
		Directives: []paradigm.DirectiveSpec{
			{Name: "mode", Form: paradigm.FormLine, Text: paradigm.ArgOne, Required: true},
		},
		Conformance: []paradigm.ConformanceCase{
			{ID: "alpha/mode", Directive: "mode", Assertion: "mode feeds the step instruction"},
		},
	}); err != nil {
		t.Fatalf("register scoped paradigm: %v", err)
	}

	doc := mustParseDoc(t, `thoughtrecipe demo
"Demo."

agent worker uses alpha

run worker:
  goal "Do work."
`)
	errs := ValidateAgainstContracts(doc, scoped)
	if len(errs) == 0 {
		t.Fatal("expected required-directive violation")
	}
	if !strings.Contains(errs[0].Error(), `required directive "mode" is missing`) {
		t.Fatalf("error %v missing required-directive message", errs[0])
	}
}

// A nested body item outside the directive's declared Body set is rejected.
// The fixture uses the chainer `link` block, whose declared Body permits
// from/goal/capture/stream/directive but not capability steps.
func TestValidateContractsRejectsNestedItemOutsideBody(t *testing.T) {
	msg := contractErrorText(t, `thoughtrecipe demo
"Demo."

agent summarizer uses chainer

run summarizer:
  link summarize:
    do relurpic:not_allowed
`)
	for _, want := range []string{
		`nested item do is not allowed in a link block`,
		"chainer paradigm contract",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error text %q missing %q", msg, want)
		}
	}
}

// A nested body item inside the declared Body set validates clean.
func TestValidateContractsAcceptsNestedBodyItems(t *testing.T) {
	src := `thoughtrecipe demo
"Demo."

agent summarizer uses chainer

run summarizer:
  link summarize:
    from state.findings
    prompt "Summarize the findings."
    capture result -> state.summary
`
	if errs := contractErrors(t, src); len(errs) != 0 {
		t.Fatalf("expected no contract violations, got %v", errs)
	}
}

// The semantic pass surfaces the same contract rejection through Resolve.
func TestSymbolTableResolveRejectsUnknownParadigm(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe demo
"Demo."

trigger as capability:
  may read workspace

agent reviewer uses nosuchparadigm

run reviewer:
  goal "Review."
`)
	err := NewSymbolTable(doc).Resolve()
	if err == nil {
		t.Fatal("expected unknown paradigm rejection at Resolve")
	}
	if !strings.Contains(err.Error(), "unsupported agent paradigm") {
		t.Fatalf("got %v, want unsupported agent paradigm error", err)
	}
}

// The loader rejects an unknown paradigm at load time with the recipe path.
func TestLoaderRejectsUnknownParadigmAtLoad(t *testing.T) {
	workspace := t.TempDir()
	sourceRoot := filepath.Join(workspace, ThoughtRecipeSourceRoot)
	if err := os.MkdirAll(sourceRoot, 0o700); err != nil { // public: test fixture dir
		t.Fatalf("mkdir source root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "bad.euclo"), []byte(`thoughtrecipe bad
"Bad."

trigger as capability:
  may read workspace

agent reviewer uses nosuchparadigm

run reviewer:
  goal "Review."
`), 0o600); err != nil { // public: test fixture
		t.Fatalf("write recipe: %v", err)
	}

	_, err := NewLoader().LoadWorkspace(workspace)
	if err == nil {
		t.Fatal("expected load failure for unknown paradigm")
	}
	if !strings.Contains(err.Error(), "unsupported agent paradigm") {
		t.Fatalf("load error %v missing unsupported paradigm text", err)
	}
	if !strings.Contains(err.Error(), "nosuchparadigm") {
		t.Fatalf("load error %v missing paradigm name", err)
	}
}

func TestRegisterCompiledRejectsUnknownParadigmPlan(t *testing.T) {
	recipe := &surface.ThoughtRecipe{
		ID:   "bad-plan",
		Name: "bad-plan",
		Metadata: surface.ThoughtRecipeMetadata{
			Name: "bad-plan",
		},
	}
	plan := &ExecutionPlan{
		ThoughtRecipe: recipe,
		Steps: []ExecutionStep{{
			ID:       "bad-plan.step0",
			Kind:     StepKindRun,
			Paradigm: "nosuchparadigm",
			Prompt:   "Continue.",
			Goal:     "Continue.",
			Config:   map[string]any{},
		}},
	}
	reg := NewThoughtRecipeRegistry()
	err := reg.RegisterCompiled(recipe, plan, "test")
	if err == nil {
		t.Fatal("expected RegisterCompiled rejection of unknown paradigm")
	}
	if !strings.Contains(err.Error(), "unsupported agent paradigm") {
		t.Fatalf("registration error %v missing unsupported paradigm text", err)
	}
}

func TestRegisterCompiledAcceptsContractBoundPlan(t *testing.T) {
	recipe := &surface.ThoughtRecipe{
		ID:   "ok-plan",
		Name: "ok-plan",
		Metadata: surface.ThoughtRecipeMetadata{
			Name: "ok-plan",
		},
	}
	plan := &ExecutionPlan{
		ThoughtRecipe: recipe,
		Steps: []ExecutionStep{{
			ID:       "ok-plan.step0",
			Kind:     StepKindRun,
			Paradigm: "react",
			Prompt:   "Continue.",
			Goal:     "Continue.",
			Config:   map[string]any{},
		}},
	}
	reg := NewThoughtRecipeRegistry()
	if err := reg.RegisterCompiled(recipe, plan, "test"); err != nil {
		t.Fatalf("RegisterCompiled failed for react plan: %v", err)
	}
}

func TestValidatePlanContractsRejectsUndeclaredDirective(t *testing.T) {
	plan := &ExecutionPlan{
		Steps: []ExecutionStep{{
			ID:         "plan.step0",
			Kind:       StepKindRun,
			Paradigm:   "react",
			Directives: []TypedDirective{{Name: "summarize"}},
		}},
	}
	err := ValidatePlanContracts(plan, paradigm.Registry)
	if err == nil {
		t.Fatal("expected plan-level rejection of undeclared react directive")
	}
	if !strings.Contains(err.Error(), "summarize") || !strings.Contains(err.Error(), "react paradigm contract") {
		t.Fatalf("plan validation error %v missing directive/contract text", err)
	}
}

func TestValidatePlanContractsScopesToRunAndDelegateSteps(t *testing.T) {
	plan := &ExecutionPlan{
		Steps: []ExecutionStep{
			{
				ID:           "cap.step0",
				Kind:         StepKindCapability,
				Paradigm:     "euclo",
				CapabilityID: "euclo:cap.x",
				Config:       map[string]any{},
			},
			{
				ID:       "ask.step0",
				Kind:     StepKindAsk,
				Paradigm: "euclo",
				Config:   map[string]any{},
			},
		},
	}
	if err := ValidatePlanContracts(plan, paradigm.Registry); err != nil {
		t.Fatalf("structural steps must not be contract-validated: %v", err)
	}
}

func TestScopedParadigmRegistryWiresThroughSymbolTable(t *testing.T) {
	scoped := paradigm.NewContractRegistry()
	if err := scoped.Register(paradigm.Contract{
		Paradigm: "alpha",
		Summary:  "scoped test paradigm",
		Shape:    paradigm.ShapeLinear,
		Directives: []paradigm.DirectiveSpec{
			{Name: "mode", Form: paradigm.FormLine, Text: paradigm.ArgOne, Required: true},
		},
		Conformance: []paradigm.ConformanceCase{
			{ID: "alpha/mode", Directive: "mode", Assertion: "mode feeds the step instruction"},
		},
	}); err != nil {
		t.Fatalf("register scoped paradigm: %v", err)
	}

	doc := mustParseDoc(t, `thoughtrecipe demo
"Demo."

trigger as capability:
  may read workspace

agent worker uses alpha

run worker:
  goal "Do work."
  mode "fast"
`)
	// Global registry does not know "alpha": default Resolve rejects it.
	st := NewSymbolTable(doc)
	if err := st.Resolve(); err == nil {
		t.Fatal("expected global registry to reject scoped-only paradigm")
	}
	// The scoped registry validates the same document cleanly.
	st = NewSymbolTable(doc).WithParadigmRegistry(scoped)
	if err := st.Resolve(); err != nil {
		t.Fatalf("scoped registry should accept the document: %v", err)
	}
}

func TestValidateAgainstContractsNilRegistryDefaultsGlobal(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe demo
"Demo."

agent reviewer uses react

run reviewer:
  goal "Review."
`)
	if errs := ValidateAgainstContracts(doc, nil); len(errs) != 0 {
		t.Fatalf("nil registry should default to the global registry, got %v", errs)
	}
}

// AC-3 defensive half: a paradigm outside the registry is unreachable from any
// .erpe-reachable path (the loader rejects it), and the runtime guard that
// remains returns the typed contract-violation error, not a plain-text
// "unsupported paradigm".
func TestBuildAgentReturnsContractViolationGuard(t *testing.T) {
	step := ExecutionStep{
		ID:       "guard.step",
		Kind:     StepKindRun,
		Paradigm: "nosuchparadigm",
		Prompt:   "Continue.",
		Goal:     "Continue.",
		Config:   map[string]any{},
	}
	node := NewRunNode("guard.step", &paradigm.Deps{}, step)
	_, err := node.Execute(context.Background(), contextdata.NewEnvelope("task-guard", "session-guard"))
	if err == nil {
		t.Fatal("expected the contract-violation guard to error")
	}
	var violation *paradigm.ErrContractViolation
	if !errors.As(err, &violation) {
		t.Fatalf("expected ErrContractViolation, got %T: %v", err, err)
	}
	if violation.Paradigm != "nosuchparadigm" {
		t.Fatalf("guard paradigm = %q, want nosuchparadigm", violation.Paradigm)
	}
	if strings.Contains(err.Error(), "unsupported paradigm") {
		t.Fatalf("guard error must not carry the retired plain-text wording: %q", err.Error())
	}
}
