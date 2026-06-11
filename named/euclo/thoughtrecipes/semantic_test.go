package thoughtrecipe

import (
	"context"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/execution/prompt/prompttest"
	ecap "codeburg.org/lexbit/relurpify/named/euclo/capabilities"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
)

type semanticTestTool struct {
	name      string
	available bool
}

func (t semanticTestTool) Name() string                      { return t.name }
func (t semanticTestTool) Description() string               { return t.name }
func (t semanticTestTool) Category() string                  { return "test" }
func (t semanticTestTool) Parameters() []ports.ToolParameter { return nil }
func (t semanticTestTool) Execute(ctx context.Context, args map[string]any) (*ports.ToolResult, error) {
	return &ports.ToolResult{Success: true}, nil
}
func (t semanticTestTool) IsAvailable(ctx context.Context) bool { return t.available }
func (t semanticTestTool) Permissions() ports.ToolPermissions   { return ports.ToolPermissions{} }
func (t semanticTestTool) Tags() []string                       { return nil }

func TestSymbolTableResolvesValidDocument(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe code_review
"Review the workspace."

trigger as capability:
  may read workspace
  may ask user
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

func TestSymbolTableResolvesToolPoliciesWithCanonicalNames(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe demo
"Demo."

trigger as capability:
  may read workspace
  may invoke [" file_edit ", "grep", "file_write", "grep"]

input workspace: "**/*"
agent reviewer uses react

run reviewer:
  may invoke ["grep", "file_edit"]
  goal "Inspect the code."
`)

	reg := registry.NewRegistry()
	if err := reg.RegisterLegacyTool(context.Background(), semanticTestTool{name: "file_write", available: true}); err != nil {
		t.Fatalf("register file_write: %v", err)
	}
	if err := reg.RegisterLegacyTool(context.Background(), semanticTestTool{name: "file_search", available: true}); err != nil {
		t.Fatalf("register file_search: %v", err)
	}

	st := NewSymbolTable(doc).WithToolRegistry(reg)
	if err := st.Resolve(); err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	trigger := doc.Declarations[0].(*TriggerDecl)
	if got, want := trigger.ToolPolicies[0].ResolvedToolNames, []string{"file_write", "file_search"}; !equalStrings(got, want) {
		t.Fatalf("trigger resolved tool names = %#v, want %#v", got, want)
	}

	run := doc.Declarations[3].(*RunDecl)
	policy := run.Items[0].(*ToolInvokePolicyDecl)
	if got, want := policy.ResolvedToolNames, []string{"file_search", "file_write"}; !equalStrings(got, want) {
		t.Fatalf("run resolved tool names = %#v, want %#v", got, want)
	}
}

func TestSymbolTableRejectsUnknownTool(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe demo
"Demo."

trigger as capability:
  may read workspace
  may invoke ["missing_tool"]

input workspace: "**/*"
agent reviewer uses react
`)

	reg := registry.NewRegistry()
	if err := reg.RegisterLegacyTool(context.Background(), semanticTestTool{name: "file_search", available: true}); err != nil {
		t.Fatalf("register file_search: %v", err)
	}

	err := NewSymbolTable(doc).WithToolRegistry(reg).Resolve()
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("expected unknown tool error, got %v", err)
	}
}

func TestSymbolTableRejectsNonCallableTool(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe demo
"Demo."

trigger as capability:
  may read workspace
  may invoke ["hidden_tool"]

input workspace: "**/*"
agent reviewer uses react
`)

	reg := registry.NewRegistry()
	if err := reg.RegisterLegacyTool(context.Background(), semanticTestTool{name: "hidden_tool", available: true}); err != nil {
		t.Fatalf("register hidden_tool: %v", err)
	}
	reg.AddExposurePolicies([]agentspec.CapabilityExposurePolicy{{
		Selector: agentspec.CapabilitySelector{Name: "hidden_tool"},
		Access:   agentspec.CapabilityExposureHidden,
	}})

	err := NewSymbolTable(doc).WithToolRegistry(reg).Resolve()
	if err == nil || !strings.Contains(err.Error(), "not callable") {
		t.Fatalf("expected not callable error, got %v", err)
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

func TestSymbolTableResolvesPromptAndRecipeImports(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe review_flow
"Route review prompts."

trigger as capability:
  may read workspace
  may ask user

import prompt named.euclo.code.explore as explore
import prompt named.euclo.intent.clarify.question.v1 as clarify_question
import recipe named.euclo.review.basic as review_basic

agent reviewer uses react

run reviewer:
  goal prompt explore

ask user:
  question prompt clarify_question
`)

	prompts := prompttest.New().
		With("named.euclo.code.explore", "Explore the workspace.").
		With("named.euclo.intent.clarify.question.v1", "Clarify the request.")
	recipes := NewThoughtRecipeRegistry()
	seedRecipe := &surface.ThoughtRecipe{
		ID:   "named.euclo.review.basic",
		Name: "named.euclo.review.basic",
		Metadata: surface.ThoughtRecipeMetadata{
			Name: "named.euclo.review.basic",
		},
	}
	if _, err := recipes.RegisterCompiledFirstWins(seedRecipe, &ExecutionPlan{ThoughtRecipe: seedRecipe}, "seed.euclo"); err != nil {
		t.Fatalf("RegisterCompiledFirstWins failed: %v", err)
	}

	st := NewSymbolTable(doc).
		WithPromptRegistry(prompts).
		WithRecipeRegistry(recipes)
	if err := st.Resolve(); err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
}

func TestSymbolTableRejectsUnknownPromptImportTarget(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe demo
"Demo."

trigger as capability:
  may read workspace

import prompt named.euclo.code.missing as explore
`)

	err := NewSymbolTable(doc).WithPromptRegistry(prompttest.New()).Resolve()
	if err == nil || !strings.Contains(err.Error(), "unknown prompt import") {
		t.Fatalf("expected unknown prompt import error, got %v", err)
	}
}

func TestSymbolTableRejectsUnknownRecipeImportTarget(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe demo
"Demo."

trigger as capability:
  may read workspace

import recipe named.euclo.review.missing as review_basic
`)

	err := NewSymbolTable(doc).WithRecipeRegistry(NewThoughtRecipeRegistry()).Resolve()
	if err == nil || !strings.Contains(err.Error(), "unknown recipe import") {
		t.Fatalf("expected unknown recipe import error, got %v", err)
	}
}

func TestSymbolTableRejectsPromptBindingWithoutPromptRegistry(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe demo
"Demo."

trigger as capability:
  may read workspace

import prompt named.euclo.code.explore as explore

agent reviewer uses react

run reviewer:
  goal prompt explore
`)

	prompts := prompttest.New().With("named.euclo.code.explore", "Explore.")
	err := NewSymbolTable(doc).
		WithPromptRegistry(nil).
		Resolve()
	if err == nil || !strings.Contains(err.Error(), "prompt registry is required") {
		t.Fatalf("expected prompt registry error, got %v", err)
	}

	// Ensure the positive path still validates with a registry.
	if err := NewSymbolTable(doc).WithPromptRegistry(prompts).Resolve(); err != nil {
		t.Fatalf("expected prompt registry validation to succeed, got %v", err)
	}
}

func TestSymbolTableRejectsPromptBindingToRecipeImport(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe demo
"Demo."

trigger as capability:
  may read workspace

import recipe named.euclo.review.basic as review_basic

agent reviewer uses react

run reviewer:
  goal prompt review_basic
`)

	recipes := NewThoughtRecipeRegistry()
	seedRecipe := &surface.ThoughtRecipe{
		ID:   "named.euclo.review.basic",
		Name: "named.euclo.review.basic",
		Metadata: surface.ThoughtRecipeMetadata{
			Name: "named.euclo.review.basic",
		},
	}
	if _, err := recipes.RegisterCompiledFirstWins(seedRecipe, &ExecutionPlan{ThoughtRecipe: seedRecipe}, "seed.euclo"); err != nil {
		t.Fatalf("RegisterCompiledFirstWins failed: %v", err)
	}

	err := NewSymbolTable(doc).
		WithRecipeRegistry(recipes).
		Resolve()
	if err == nil || !strings.Contains(err.Error(), "not a prompt import") {
		t.Fatalf("expected binding kind error, got %v", err)
	}
}

func TestSymbolTableRejectsDuplicateImportBindings(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe demo
"Demo."

trigger as capability:
  may read workspace

import prompt named.euclo.code.explore as shared
import recipe named.euclo.review.basic as shared
`)

	prompts := prompttest.New().With("named.euclo.code.explore", "Explore.")
	recipes := NewThoughtRecipeRegistry()
	seedRecipe := &surface.ThoughtRecipe{
		ID:   "named.euclo.review.basic",
		Name: "named.euclo.review.basic",
		Metadata: surface.ThoughtRecipeMetadata{
			Name: "named.euclo.review.basic",
		},
	}
	if _, err := recipes.RegisterCompiledFirstWins(seedRecipe, &ExecutionPlan{ThoughtRecipe: seedRecipe}, "seed.euclo"); err != nil {
		t.Fatalf("RegisterCompiledFirstWins failed: %v", err)
	}

	err := NewSymbolTable(doc).
		WithPromptRegistry(prompts).
		WithRecipeRegistry(recipes).
		Resolve()
	if err == nil || !strings.Contains(err.Error(), "duplicate declaration") {
		t.Fatalf("expected duplicate binding error, got %v", err)
	}
}

func TestSymbolTableRejectsDirectImportCycles(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe demo
"Demo."

trigger as capability:
  may read workspace

import recipe demo as self
`)

	recipes := NewThoughtRecipeRegistry()
	seedRecipe := &surface.ThoughtRecipe{
		ID:   "demo",
		Name: "demo",
		Metadata: surface.ThoughtRecipeMetadata{
			Name: "demo",
		},
	}
	if _, err := recipes.RegisterCompiledFirstWins(seedRecipe, &ExecutionPlan{ThoughtRecipe: seedRecipe}, "seed.euclo"); err != nil {
		t.Fatalf("RegisterCompiledFirstWins failed: %v", err)
	}

	err := NewSymbolTable(doc).
		WithRecipeRegistry(recipes).
		Resolve()
	if err == nil || !strings.Contains(err.Error(), "direct import cycle") {
		t.Fatalf("expected direct import cycle error, got %v", err)
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

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
