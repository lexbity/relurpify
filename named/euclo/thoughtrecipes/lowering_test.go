package thoughtrecipe

import (
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
)

func TestLowerDocumentPreservesAgentBindingsAndRunStructure(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe code_assistant
"Route code requests."

trigger as capability:
  family ["debug", "debug"]
  keyword ["fix", "panic"]
  handoff ["reviewer", "executor", "reviewer"]
  may read workspace
  may write workspace

input workspace: "**/*"
input prompt: user.request

agent router uses goalcon
agent reviewer uses react

run router:
  from input.workspace
  from input.prompt
  goal "Review the codebase."
  step inspect when missing input.workspace
  capture:
    result -> state.summary

run reviewer:
  from state.summary
  goal "Summarize the findings."
`)

	plan, err := LowerDocument(doc)
	if err != nil {
		t.Fatalf("LowerDocument failed: %v", err)
	}

	if got, want := len(plan.Agents), 2; got != want {
		t.Fatalf("agent binding count = %d, want %d", got, want)
	}
	if got := plan.Agents["router"].Paradigm; got != "goalcon" {
		t.Fatalf("router paradigm = %q, want %q", got, "goalcon")
	}
	if got := plan.Agents["reviewer"].Paradigm; got != "react" {
		t.Fatalf("reviewer paradigm = %q, want %q", got, "react")
	}

	if got, want := len(plan.Steps), 2; got != want {
		t.Fatalf("step count = %d, want %d", got, want)
	}
	if got := plan.RouteKind; got != surface.TriggerRouteKindCapability {
		t.Fatalf("plan route kind = %q, want %q", got, surface.TriggerRouteKindCapability)
	}
	if got := plan.ThoughtRecipe.RouteKind; got != surface.TriggerRouteKindCapability {
		t.Fatalf("thoughtrecipe route kind = %q, want %q", got, surface.TriggerRouteKindCapability)
	}
	if got := plan.ThoughtRecipe.Metadata.Families; len(got) != 1 || got[0] != "debug" {
		t.Fatalf("families = %#v, want [debug]", got)
	}
	if got := plan.ThoughtRecipe.Metadata.Keywords; len(got) != 2 || got[0] != "fix" || got[1] != "panic" {
		t.Fatalf("keywords = %#v, want [fix panic]", got)
	}
	if got := plan.ThoughtRecipe.Metadata.HandoffTargets; len(got) != 2 || got[0] != "reviewer" || got[1] != "executor" {
		t.Fatalf("handoff targets = %#v, want [reviewer executor]", got)
	}
	if got := plan.ThoughtRecipe.Metadata.Tags; len(got) != 5 {
		t.Fatalf("tags = %#v, want 5 unique entries", got)
	}

	first := plan.Steps[0]
	if got := first.Paradigm; got != "goalcon" {
		t.Fatalf("first step paradigm = %q, want %q", got, "goalcon")
	}
	if got := first.Goal; got != "Review the codebase." {
		t.Fatalf("first step goal = %q, want %q", got, "Review the codebase.")
	}
	if got := strings.Join(first.Sources, ","); got != "input.workspace,input.prompt" {
		t.Fatalf("first step sources = %q, want %q", got, "input.workspace,input.prompt")
	}
	if got, ok := first.Step.Config["from_sources"].([]string); !ok || len(got) != 2 {
		t.Fatalf("first step config from_sources = %#v, want 2 entries", first.Step.Config["from_sources"])
	}
	if got, ok := first.Step.Config["goals"].([]string); !ok || len(got) != 1 || got[0] != "Review the codebase." {
		t.Fatalf("first step config goals = %#v, want [Review the codebase.]", first.Step.Config["goals"])
	}
	if got, ok := first.Step.Config["execution_items"].([]map[string]any); ok && len(got) == 0 {
		t.Fatalf("first step execution_items unexpectedly empty")
	}

	second := plan.Steps[1]
	if got := second.Paradigm; got != "react" {
		t.Fatalf("second step paradigm = %q, want %q", got, "react")
	}
	if got := second.Goal; got != "Summarize the findings." {
		t.Fatalf("second step goal = %q, want %q", got, "Summarize the findings.")
	}
	if got, want := len(first.CaptureBindings), 1; got != want {
		t.Fatalf("capture binding count = %d, want %d", got, want)
	}
	if got := first.CaptureBindings[0].Destination.Raw; got != "state.summary" {
		t.Fatalf("capture destination = %q, want %q", got, "state.summary")
	}
}

func TestLowerDocumentLowersRouteBranchesInOrder(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe route_demo
"Route demo."

trigger as capability:
  may read workspace

agent router uses goalcon
agent reviewer uses react

route:
  when state.intent is review:
    run reviewer:
      from input.prompt
      goal "Review the prompt."
  when state.intent confidence below 70%:
    delegate to router:
      from state.intent
      goal "Resolve the intent."
  otherwise:
    capture:
      state.intent -> state.route_choice
`)

	plan, err := LowerDocument(doc)
	if err != nil {
		t.Fatalf("LowerDocument failed: %v", err)
	}

	if got, want := len(plan.Routes), 1; got != want {
		t.Fatalf("route group count = %d, want %d", got, want)
	}
	route := plan.Routes[0]
	if got, want := len(route.Branches), 3; got != want {
		t.Fatalf("branch count = %d, want %d", got, want)
	}
	if route.Branches[0].Predicate == nil || route.Branches[0].Predicate.Kind != "is" {
		t.Fatalf("branch 0 predicate = %#v, want is", route.Branches[0].Predicate)
	}
	if route.Branches[1].Predicate == nil || route.Branches[1].Predicate.Kind != "confidence_below" {
		t.Fatalf("branch 1 predicate = %#v, want confidence_below", route.Branches[1].Predicate)
	}
	if !route.Branches[2].IsElse {
		t.Fatal("otherwise branch was not preserved as explicit fallback")
	}
	if got, want := len(route.Branches[0].Steps), 1; got != want {
		t.Fatalf("branch 0 body steps = %d, want %d", got, want)
	}
	if got := route.Branches[0].Steps[0].Type; got != "run" {
		t.Fatalf("branch 0 step type = %q, want run", got)
	}
	if got := route.Branches[1].Steps[0].Type; got != "delegate" {
		t.Fatalf("branch 1 step type = %q, want delegate", got)
	}
	if got := len(plan.Warnings); got != 0 {
		t.Fatalf("unexpected warnings: %#v", plan.Warnings)
	}
}

func TestLowerDocumentWarnsOnRouteWithoutOtherwise(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe route_demo
"Route demo."

trigger as capability:
  may read workspace

agent reviewer uses react

route:
  when state.intent is review:
    run reviewer:
      goal "Review the prompt."
`)

	plan, err := LowerDocument(doc)
	if err != nil {
		t.Fatalf("LowerDocument failed: %v", err)
	}
	if got, want := len(plan.Warnings), 1; got != want {
		t.Fatalf("warning count = %d, want %d", got, want)
	}
	if msg := plan.Warnings[0].Message; !strings.Contains(msg, "otherwise") {
		t.Fatalf("warning message = %q, want otherwise coverage warning", msg)
	}
}

func TestLowerDocumentLowersAskUserBlock(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe ask_demo
"Ask demo."

trigger as capability:
  may ask user

ask user:
  question "Do you want review or refactor?"
  choices ["review", "refactor"]
  capture answer -> state.intent
`)

	plan, err := LowerDocument(doc)
	if err != nil {
		t.Fatalf("LowerDocument failed: %v", err)
	}
	if got, want := len(plan.Steps), 1; got != want {
		t.Fatalf("step count = %d, want %d", got, want)
	}
	step := plan.Steps[0]
	if step.Type != "ask" {
		t.Fatalf("step type = %q, want ask", step.Type)
	}
	if step.Question != "Do you want review or refactor?" {
		t.Fatalf("question = %q", step.Question)
	}
	if got, want := len(step.Choices), 2; got != want {
		t.Fatalf("choice count = %d, want %d", got, want)
	}
	if got := step.Choices[0]; got != "review" {
		t.Fatalf("first choice = %q, want review", got)
	}
	if got, want := len(step.CaptureBindings), 1; got != want {
		t.Fatalf("capture binding count = %d, want %d", got, want)
	}
	if got := step.CaptureBindings[0].Destination.Raw; got != "state.intent" {
		t.Fatalf("capture destination = %q, want state.intent", got)
	}
}

func TestLowerDocumentLowersPromptBoundGoalAndQuestion(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe prompt_demo
"Prompt demo."

trigger as capability:
  may read workspace

import prompt named.euclo.code.explore as explore
import prompt named.euclo.intent.clarify.question.v1 as clarify_question

agent reviewer uses react

run reviewer:
  from input.prompt
  goal prompt explore

ask user:
  question prompt clarify_question
`)

	plan, err := LowerDocument(doc)
	if err != nil {
		t.Fatalf("LowerDocument failed: %v", err)
	}
	if got, want := len(plan.Steps), 2; got != want {
		t.Fatalf("step count = %d, want %d", got, want)
	}

	run := plan.Steps[0]
	if run.Type != "run" {
		t.Fatalf("run step type = %q, want run", run.Type)
	}
	if run.Goal != "" {
		t.Fatalf("run goal = %q, want empty", run.Goal)
	}
	if run.PromptID != "explore" {
		t.Fatalf("run prompt ID = %q, want explore", run.PromptID)
	}
	if run.Prompt != "" {
		t.Fatalf("run prompt text = %q, want empty", run.Prompt)
	}
	if run.Step.PromptID != "explore" {
		t.Fatalf("run step prompt ID = %q, want explore", run.Step.PromptID)
	}
	if run.Step.Prompt != "" {
		t.Fatalf("run step prompt text = %q, want empty", run.Step.Prompt)
	}
	if got := run.Step.Config["prompt_id"]; got != "explore" {
		t.Fatalf("run config prompt_id = %#v, want explore", got)
	}

	ask := plan.Steps[1]
	if ask.Type != "ask" {
		t.Fatalf("ask step type = %q, want ask", ask.Type)
	}
	if ask.Question != "" {
		t.Fatalf("ask question = %q, want empty", ask.Question)
	}
	if ask.PromptID != "clarify_question" {
		t.Fatalf("ask prompt ID = %q, want clarify_question", ask.PromptID)
	}
	if ask.Prompt != "" {
		t.Fatalf("ask prompt text = %q, want empty", ask.Prompt)
	}
	if ask.Step.PromptID != "clarify_question" {
		t.Fatalf("ask step prompt ID = %q, want clarify_question", ask.Step.PromptID)
	}
	if ask.Step.Prompt != "" {
		t.Fatalf("ask step prompt text = %q, want empty", ask.Step.Prompt)
	}
	if got := ask.Step.Config["prompt_id"]; got != "clarify_question" {
		t.Fatalf("ask config prompt_id = %#v, want clarify_question", got)
	}
}

func TestLowerDocumentPreservesIntentRouteKind(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe intent_demo
"Clarify."

trigger as intent:
  may read workspace

ask user:
  question "What should I clarify?"
  choices ["scope", "symbols"]
`)

	plan, err := LowerDocument(doc)
	if err != nil {
		t.Fatalf("LowerDocument failed: %v", err)
	}
	if got := plan.RouteKind; got != surface.TriggerRouteKindIntent {
		t.Fatalf("plan route kind = %q, want %q", got, surface.TriggerRouteKindIntent)
	}
	if got := plan.ThoughtRecipe.RouteKind; got != surface.TriggerRouteKindIntent {
		t.Fatalf("thoughtrecipe route kind = %q, want %q", got, surface.TriggerRouteKindIntent)
	}
}

func TestLowerDocumentLowersPipelineStagesInOrder(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe pipeline_demo
"Pipeline demo."

trigger as capability:
  may read workspace

agent explorer uses react
agent reviewer uses planner

pipeline:
  stage explore:
    run explorer:
      goal "Explore the workspace."

  stage summarize:
    run reviewer:
      from state.findings
      goal "Summarize the findings."
`)

	plan, err := LowerDocument(doc)
	if err != nil {
		t.Fatalf("LowerDocument failed: %v", err)
	}
	if got, want := len(plan.Steps), 1; got != want {
		t.Fatalf("step count = %d, want %d", got, want)
	}
	step := plan.Steps[0]
	if step.Type != "pipeline" {
		t.Fatalf("step type = %q, want pipeline", step.Type)
	}
	if got, want := len(step.PipelineStages), 2; got != want {
		t.Fatalf("pipeline stage count = %d, want %d", got, want)
	}
	if got := step.PipelineStages[0].Name; got != "explore" {
		t.Fatalf("stage 0 name = %q, want explore", got)
	}
	if got := step.PipelineStages[1].Name; got != "summarize" {
		t.Fatalf("stage 1 name = %q, want summarize", got)
	}
	if got, want := len(step.PipelineStages[0].Steps), 1; got != want {
		t.Fatalf("stage 0 step count = %d, want %d", got, want)
	}
	if got := step.PipelineStages[0].Steps[0].Type; got != "run" {
		t.Fatalf("stage 0 step type = %q, want run", got)
	}
	if got := step.PipelineStages[1].Steps[0].Goal; got != "Summarize the findings." {
		t.Fatalf("stage 1 goal = %q, want Summarize the findings.", got)
	}
	if got, want := len(plan.Pipelines), 1; got != want {
		t.Fatalf("pipeline group count = %d, want %d", got, want)
	}
	if got := plan.Pipelines[0].Group.ID; !strings.HasPrefix(got, "pipeline.") {
		t.Fatalf("pipeline group ID = %q, want pipeline.*", got)
	}
}

func TestApplyCaptureBindingsWritesExplicitNamespaces(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	env.SetWorkingValue("input.workspace", "workspace.txt", contextdata.MemoryClassTask)
	env.SetWorkingValue("state.summary", "ready", contextdata.MemoryClassTask)

	bindings := []CaptureBinding{
		{
			Source:      Identifier{positioned: positioned{Span: NewSpan("demo.euclo", 3, 5, 3, 10)}, Value: "state.summary"},
			Destination: PathExpr{positioned: positioned{Span: NewSpan("demo.euclo", 3, 14, 3, 27)}, Raw: "output.result", Parts: []Identifier{{Value: "output"}, {Value: "result"}}},
		},
		{
			Source:      PathExpr{positioned: positioned{Span: NewSpan("demo.euclo", 4, 5, 4, 21)}, Raw: "input.workspace", Parts: []Identifier{{Value: "input"}, {Value: "workspace"}}},
			Destination: PathExpr{positioned: positioned{Span: NewSpan("demo.euclo", 4, 25, 4, 36)}, Raw: "state.plan", Parts: []Identifier{{Value: "state"}, {Value: "plan"}}},
			Forwarding:  true,
		},
	}

	writes, err := ApplyCaptureBindings(env, bindings, map[string]any{"state.summary": "summary data"})
	if err != nil {
		t.Fatalf("ApplyCaptureBindings failed: %v", err)
	}
	if got, want := len(writes), 2; got != want {
		t.Fatalf("write count = %d, want %d", got, want)
	}
	if got, _ := env.GetWorkingValue("output.result"); got != "summary data" {
		t.Fatalf("output.result = %#v, want %q", got, "summary data")
	}
	if got, _ := env.GetWorkingValue("state.plan"); got != "workspace.txt" {
		t.Fatalf("state.plan = %#v, want %q", got, "workspace.txt")
	}
	if got, ok := env.GetWorkingValue("euclo.thoughtrecipe.demo.output.result"); ok {
		t.Fatalf("unexpected legacy capture key present: %#v", got)
	}
}

func TestLowerDocumentLowersToolScopesAndEvictsRunLocalScope(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe scope_demo
"Scope demo."

trigger as capability:
  may read workspace
  may invoke ["file_write"]

input workspace: "**/*"
agent writer uses react

run writer:
  may invoke ["file_search"]
  goal "Inspect the workspace."

run writer:
  goal "Inspect again."
`)

	reg := capability.NewRegistry()
	if err := reg.RegisterLegacyTool(semanticTestTool{name: "file_write", available: true}); err != nil {
		t.Fatalf("register file_write: %v", err)
	}
	if err := reg.RegisterLegacyTool(semanticTestTool{name: "file_search", available: true}); err != nil {
		t.Fatalf("register file_search: %v", err)
	}
	if err := NewSymbolTable(doc).WithToolRegistry(reg).Resolve(); err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	plan, err := LowerDocument(doc)
	if err != nil {
		t.Fatalf("LowerDocument failed: %v", err)
	}

	if got, want := len(plan.ToolScopes), 1; got != want {
		t.Fatalf("plan tool scope count = %d, want %d", got, want)
	}
	if got := plan.ToolScopes[0].ScopeKind; got != "trigger" {
		t.Fatalf("plan tool scope kind = %q, want trigger", got)
	}
	if got, want := plan.ToolScopes[0].ToolNames, []string{"file_write"}; !equalStringSlices(got, want) {
		t.Fatalf("plan tool names = %#v, want %#v", got, want)
	}

	first := plan.Steps[0]
	if got, want := len(first.ToolScopes), 1; got != want {
		t.Fatalf("first step local tool scope count = %d, want %d", got, want)
	}
	if got := first.ToolScopes[0].ScopeKind; got != "run" {
		t.Fatalf("first step scope kind = %q, want run", got)
	}
	if got, want := first.ToolScopes[0].ToolNames, []string{"file_search"}; !equalStringSlices(got, want) {
		t.Fatalf("first step tool names = %#v, want %#v", got, want)
	}
	if got, want := first.EffectiveToolNames, []string{"file_write", "file_search"}; !equalStringSlices(got, want) {
		t.Fatalf("first step effective tool names = %#v, want %#v", got, want)
	}
	if cfg, ok := first.Step.Config["effective_tool_names"].([]string); !ok || !equalStringSlices(cfg, []string{"file_write", "file_search"}) {
		t.Fatalf("first step config effective_tool_names = %#v", first.Step.Config["effective_tool_names"])
	}
	if scopes, ok := first.Step.Config["tool_scopes"].([]map[string]any); !ok || len(scopes) != 1 {
		t.Fatalf("first step config tool_scopes = %#v, want 1 frame", first.Step.Config["tool_scopes"])
	}

	second := plan.Steps[1]
	if got, want := len(second.ToolScopes), 0; got != want {
		t.Fatalf("second step local tool scope count = %d, want %d", got, want)
	}
	if got, want := second.EffectiveToolNames, []string{"file_write"}; !equalStringSlices(got, want) {
		t.Fatalf("second step effective tool names = %#v, want %#v", got, want)
	}
	if cfg, ok := second.Step.Config["effective_tool_names"].([]string); !ok || !equalStringSlices(cfg, []string{"file_write"}) {
		t.Fatalf("second step config effective_tool_names = %#v", second.Step.Config["effective_tool_names"])
	}
	if _, ok := second.Step.Config["tool_scopes"]; ok {
		t.Fatal("second step should not carry a local tool_scopes entry")
	}
}

func TestLowerDocumentLowersDirectCapabilityInvocationToExecutableStep(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe capability_demo
"Capability demo."

trigger as capability:
  may read workspace

agent reviewer uses react

run reviewer:
  do relurpic:code_review on input.workspace
`)

	plan, err := LowerDocument(doc)
	if err != nil {
		t.Fatalf("LowerDocument failed: %v", err)
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("plan steps = %d, want 1", len(plan.Steps))
	}
	step := plan.Steps[0]
	if got, want := step.CapabilityID, "euclo:cap.code_review"; got != want {
		t.Fatalf("step capability id = %q, want %q", got, want)
	}
	if got, want := step.Step.Config["target"], "input.workspace"; got != want {
		t.Fatalf("step target = %#v, want %q", got, want)
	}
	if got, want := step.Step.Config["capability_id"], "euclo:cap.code_review"; got != want {
		t.Fatalf("step config capability id = %#v, want %q", got, want)
	}
}

func TestLowerDocumentLowersNestedCapabilityInvocationsInRouteAndPipeline(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe nested_capabilities
"Nested capability demo."

trigger as capability:
  may read workspace

route:
  when state.intent is review:
    do relurpic:code_review on input.workspace

pipeline:
  stage review:
    do relurpic:code_review on input.workspace
`)

	plan, err := LowerDocument(doc)
	if err != nil {
		t.Fatalf("LowerDocument failed: %v", err)
	}

	if got, want := len(plan.Routes), 1; got != want {
		t.Fatalf("route group count = %d, want %d", got, want)
	}
	routeStep := plan.Routes[0].Branches[0].Steps[0]
	if got, want := routeStep.CapabilityID, "euclo:cap.code_review"; got != want {
		t.Fatalf("route capability id = %q, want %q", got, want)
	}

	if got, want := len(plan.Pipelines), 1; got != want {
		t.Fatalf("pipeline group count = %d, want %d", got, want)
	}
	pipelineStep := plan.Pipelines[0].Stages[0].Steps[0]
	if got, want := pipelineStep.CapabilityID, "euclo:cap.code_review"; got != want {
		t.Fatalf("pipeline capability id = %q, want %q", got, want)
	}
}

func TestLowerDocumentRejectsMultipleDirectCapabilityInvocationsInRun(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe capability_demo
"Capability demo."

trigger as capability:
  may read workspace

agent reviewer uses react

run reviewer:
  do relurpic:code_review on input.workspace
  do relurpic:targeted_refactor on input.workspace
`)

	_, err := LowerDocument(doc)
	if err == nil || !strings.Contains(err.Error(), "multiple direct capability invocations") {
		t.Fatalf("expected multiple capability error, got %v", err)
	}
}

func TestSymbolTableRejectsUnsupportedAgentType(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe demo
"Demo."

trigger as capability:
  may read workspace

agent reviewer uses not_a_real_paradigm
`)

	err := NewSymbolTable(doc).Resolve()
	if err == nil || !strings.Contains(err.Error(), "unsupported agent paradigm") {
		t.Fatalf("expected unsupported agent paradigm error, got %v", err)
	}
}

func equalStringSlices(got, want []string) bool {
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
