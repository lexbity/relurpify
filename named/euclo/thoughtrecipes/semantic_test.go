package thoughtrecipe

import (
	"strings"
	"testing"

	ecap "codeburg.org/lexbit/relurpify/named/euclo/capabilities"
)

func TestSymbolTableResolvesValidDocument(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe code_review
"Review the workspace."

trigger as capability:
  may read workspace
  may write workspace

input workspace: "**/*"
input prompt: user.request

type ReviewFindings:
  summary: Markdown
  risks: list<Text>
  complexity: low | medium | high

agent reviewer uses react

run reviewer:
  from input.workspace
  from input.prompt
  goal "Review the code."
  do relurpic:code_review on input.workspace
  capture:
    findings: ReviewFindings -> state.findings
    result: Markdown -> output.result
`)

	st := NewSymbolTable(doc).WithCapabilityRegistry(ecap.NewRegistry())
	if err := st.Resolve(); err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
}

func TestSymbolTableRejectsUnknownAgent(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe demo
"Demo."

trigger as capability:
  may read workspace

input workspace: "**/*"

run reviewer:
  from input.workspace
  goal "Review the code."
`)

	err := NewSymbolTable(doc).Resolve()
	if err == nil || !strings.Contains(err.Error(), "unknown agent") {
		t.Fatalf("expected unknown agent error, got %v", err)
	}
}

func TestSymbolTableRejectsUnknownInput(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe demo
"Demo."

trigger as capability:
  may read workspace

input workspace: "**/*"
agent reviewer uses react

run reviewer:
  from input.missing
  goal "Review the code."
`)

	err := NewSymbolTable(doc).Resolve()
	if err == nil || !strings.Contains(err.Error(), "unknown input") {
		t.Fatalf("expected unknown input error, got %v", err)
	}
}

func TestSymbolTableRejectsDuplicateDeclarations(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe demo
"Demo."

trigger as capability:
  may read workspace

input workspace: "**/*"
input workspace: "README.md"
agent reviewer uses react
`)

	err := NewSymbolTable(doc).Resolve()
	if err == nil || !strings.Contains(err.Error(), "duplicate declaration") {
		t.Fatalf("expected duplicate declaration error, got %v", err)
	}
}

func TestSymbolTableRejectsUnknownNamespace(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe demo
"Demo."

trigger as capability:
  may read workspace

input workspace: "**/*"
agent reviewer uses react

run reviewer:
  from input.workspace
  goal "Review the code."
  capture:
    result -> envelope.result
`)

	err := NewSymbolTable(doc).Resolve()
	if err == nil || !strings.Contains(err.Error(), "unknown namespace") {
		t.Fatalf("expected unknown namespace error, got %v", err)
	}
}

func TestSymbolTableRejectsUnknownCapability(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe demo
"Demo."

trigger as capability:
  may read workspace

input workspace: "**/*"
agent reviewer uses react

run reviewer:
  from input.workspace
  goal "Review the code."
  do relurpic:not_a_real_capability
`)

	err := NewSymbolTable(doc).WithCapabilityRegistry(ecap.NewRegistry()).Resolve()
	if err == nil || !strings.Contains(err.Error(), "unknown capability") {
		t.Fatalf("expected unknown capability error, got %v", err)
	}
}

func TestSymbolTableRejectsUnknownType(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe demo
"Demo."

trigger as capability:
  may read workspace

input workspace: "**/*"
type Finding:
  summary: UnknownType
agent reviewer uses react
`)

	err := NewSymbolTable(doc).Resolve()
	if err == nil || !strings.Contains(err.Error(), "unknown type") {
		t.Fatalf("expected unknown type error, got %v", err)
	}
}

func TestSymbolTableAcceptsCapabilityInvocationWithMatchingTriggerPolicy(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe demo
"Demo."

trigger as capability:
  may read workspace
  may write workspace

input workspace: "**/*"
agent reviewer uses react

run reviewer:
  from input.workspace
  do relurpic:targeted_refactor on input.workspace
`)

	if err := NewSymbolTable(doc).WithCapabilityRegistry(ecap.NewRegistry()).Resolve(); err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
}

func TestSymbolTableRejectsCapabilityInvocationWithoutWritePolicy(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe demo
"Demo."

trigger as capability:
  may read workspace

input workspace: "**/*"
agent reviewer uses react

run reviewer:
  from input.workspace
  do relurpic:targeted_refactor on input.workspace
`)

	err := NewSymbolTable(doc).WithCapabilityRegistry(ecap.NewRegistry()).Resolve()
	if err == nil || !strings.Contains(err.Error(), "write workspace") {
		t.Fatalf("expected write policy error, got %v", err)
	}
}

func TestSymbolTableRejectsAskUserWithoutTriggerPolicy(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe demo
"Demo."

trigger as capability:
  may read workspace

input workspace: "**/*"
agent reviewer uses react

ask user:
  question "Pick one."
  choices ["a", "b"]
`)

	err := NewSymbolTable(doc).Resolve()
	if err == nil || !strings.Contains(err.Error(), "may ask user") {
		t.Fatalf("expected ask user policy error, got %v", err)
	}
}

func TestSymbolTableAcceptsIntentTriggerWithAskUserBlock(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe intent_demo
"Clarify the request."

trigger as intent:
  family ["clarification"]
  keyword ["clarify", "question"]
  handoff ["intent_clarify"]
  may read workspace

input workspace: "**/*"

ask user:
  question "What should I clarify?"
  choices ["scope", "symbols"]
`)

	if err := NewSymbolTable(doc).Resolve(); err != nil {
		t.Fatalf("expected intent trigger to validate without ask-user policy, got %v", err)
	}
}

func TestSymbolTableAcceptsTriggerAssociationsOnCapabilityRoute(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe demo
"Demo."

trigger as capability:
  family ["debug"]
  keyword ["panic", "trace"]
  handoff ["debug_followup"]
  may read workspace
`)

	if err := NewSymbolTable(doc).Resolve(); err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
}

func TestSymbolTableRejectsUnsupportedTriggerRouteKind(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe demo
"Demo."

trigger as bootstrap:
  may read workspace
`)

	err := NewSymbolTable(doc).Resolve()
	if err == nil || !strings.Contains(err.Error(), "unsupported trigger route") {
		t.Fatalf("expected unsupported trigger route error, got %v", err)
	}
}

func TestLowerCapabilityInvocationProducesRuntimePlan(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe demo
"Demo."

trigger as capability:
  may read workspace
  may write workspace

input workspace: "**/*"
agent reviewer uses react

run reviewer:
  do relurpic:targeted_refactor on input.workspace
  do relurpic:code_review with state.plan
`)

	run := doc.Declarations[3].(*RunDecl)
	first, ok := run.Items[0].(*CapabilityInvocation)
	if !ok {
		t.Fatalf("item 0 type = %T, want *CapabilityInvocation", run.Items[0])
	}
	plan, err := LowerCapabilityInvocation(first)
	if err != nil {
		t.Fatalf("LowerCapabilityInvocation failed: %v", err)
	}
	if got, want := plan.CapabilityID, "euclo:cap.targeted_refactor"; got != want {
		t.Fatalf("CapabilityID = %q, want %q", got, want)
	}
	if got, want := plan.Target, "input.workspace"; got != want {
		t.Fatalf("Target = %q, want %q", got, want)
	}
	if got, ok := plan.Arguments["target"].(string); !ok || got != "input.workspace" {
		t.Fatalf("target argument = %#v, want %q", plan.Arguments["target"], "input.workspace")
	}

	second, ok := run.Items[1].(*CapabilityInvocation)
	if !ok {
		t.Fatalf("item 1 type = %T, want *CapabilityInvocation", run.Items[1])
	}
	plan, err = LowerCapabilityInvocation(second)
	if err != nil {
		t.Fatalf("LowerCapabilityInvocation failed: %v", err)
	}
	if got, want := plan.CapabilityID, "euclo:cap.code_review"; got != want {
		t.Fatalf("CapabilityID = %q, want %q", got, want)
	}
	if got, want := plan.Input, "state.plan"; got != want {
		t.Fatalf("Input = %q, want %q", got, want)
	}
	if got, ok := plan.Arguments["input"].(string); !ok || got != "state.plan" {
		t.Fatalf("input argument = %#v, want %q", plan.Arguments["input"], "state.plan")
	}
}

func TestLowerCapabilityInvocationRejectsLiteralOperand(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe demo
"Demo."

trigger as capability:
  may read workspace
  may write workspace

input workspace: "**/*"
agent reviewer uses react

run reviewer:
  do relurpic:targeted_refactor on "README.md"
`)

	run := doc.Declarations[3].(*RunDecl)
	invocation := run.Items[0].(*CapabilityInvocation)
	if _, err := LowerCapabilityInvocation(invocation); err == nil || !strings.Contains(err.Error(), "on operand must be a reference") {
		t.Fatalf("expected reference error, got %v", err)
	}
}

func mustParseDoc(t *testing.T, src string) *ThoughtRecipeDocument {
	t.Helper()
	doc, err := ParseSource("demo.euclo", src)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}
	return doc
}
