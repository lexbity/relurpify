package thoughtrecipe

import (
	"strings"
	"testing"
)

func TestParseSourceParsesCoreThoughtRecipeConstructs(t *testing.T) {
	src := `thoughtrecipe code_assistant
"Route code requests."

trigger as capability:
  family ["debug"]
  keyword ["fix", "diagnose", "trace"]
  handoff ["reviewer", "executor"]
  may read workspace
  may write workspace

input workspace: "**/*"
input prompt: user.request

type ReviewFindings:
  summary: Markdown
  complexity: low | medium | high

agent router uses goalcon
agent reviewer uses react

run router:
  from input.prompt
  goal "Classify the user's request."
  capture:
    intent: review | refactor | explain | ambiguous -> state.intent
    confidence: Percent -> state.intent_confidence

route:
  when state.intent is ambiguous:
    ask user:
      question "Do you want review, refactor, or explanation?"
      choices ["review", "refactor", "explain"]
      capture answer -> state.intent

  when state.intent confidence below 70%:
    run reviewer:
      from input.workspace
      from input.prompt
      goal "Review the code."
      do relurpic:code_review
      capture:
        result: Markdown -> output.result

  otherwise:
    capture:
      state.prompt -> state.summary

delegate to reviewer:
  from state.findings
  goal "Review the findings."
  do relurpic:code_review
  capture:
    result: Markdown -> output.result

ask user:
  question "Pick a mode."
  choices from state.modes
  capture answer -> state.intent

pipeline:
  stage explore:
    run router:
      from input.workspace
      goal "Explore the workspace."
      capture:
        findings: ReviewFindings -> state.findings

  stage summarize:
    run reviewer:
      from state.findings
      goal "Summarize the findings."
      capture:
        result: Markdown -> output.result
`

	doc, err := ParseSource("code_assistant.euclo", src)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}

	if doc.Name != "code_assistant" {
		t.Fatalf("document name = %q, want %q", doc.Name, "code_assistant")
	}
	if len(doc.Declarations) < 9 {
		t.Fatalf("declaration count = %d, want at least 9", len(doc.Declarations))
	}

	trigger := doc.Declarations[0].(*TriggerDecl)
	if got := len(trigger.Lines); got != 2 {
		t.Fatalf("trigger lines = %d, want 2", got)
	}
	if got := trigger.RouteKind; got != TriggerRouteKindCapability {
		t.Fatalf("trigger route kind = %q, want %q", got, TriggerRouteKindCapability)
	}
	if got := len(trigger.Associations); got != 3 {
		t.Fatalf("trigger associations = %d, want 3", got)
	}
	if got := trigger.Associations[0].Name.Value; got != "family" {
		t.Fatalf("association 0 name = %q, want family", got)
	}
	if got := trigger.Associations[0].Values.Raw; got != `["debug"]` {
		t.Fatalf("association 0 raw = %q, want %q", got, `["debug"]`)
	}
	if got := trigger.Associations[1].Values.Raw; got != `["fix", "diagnose", "trace"]` {
		t.Fatalf("association 1 raw = %q, want %q", got, `["fix", "diagnose", "trace"]`)
	}
	if got := trigger.Associations[2].Name.Value; got != "handoff" {
		t.Fatalf("association 2 name = %q, want handoff", got)
	}
	if got := trigger.Associations[2].Values.Raw; got != `["reviewer", "executor"]` {
		t.Fatalf("association 2 raw = %q, want %q", got, `["reviewer", "executor"]`)
	}

	run := doc.Declarations[6].(*RunDecl)
	if run.Agent.Value != "router" {
		t.Fatalf("run agent = %q, want %q", run.Agent.Value, "router")
	}
	if _, ok := run.Items[1].(*GoalClause); !ok {
		t.Fatalf("run goal item type = %T, want *GoalClause", run.Items[1])
	}
	if _, ok := run.Items[2].(*CaptureBlock); !ok {
		t.Fatalf("run capture item type = %T, want *CaptureBlock", run.Items[2])
	}

	route := doc.Declarations[7].(*RouteDecl)
	if len(route.Branches) != 3 {
		t.Fatalf("route branches = %d, want 3", len(route.Branches))
	}
	if route.Branches[0].Predicate.Kind != "is" {
		t.Fatalf("branch 0 predicate kind = %q, want %q", route.Branches[0].Predicate.Kind, "is")
	}
	if route.Branches[1].Predicate.Kind != "confidence_below" {
		t.Fatalf("branch 1 predicate kind = %q, want %q", route.Branches[1].Predicate.Kind, "confidence_below")
	}
	if !route.Branches[2].IsElse {
		t.Fatal("otherwise branch was not marked as else")
	}

	delegate := doc.Declarations[8].(*DelegateDecl)
	if delegate.Agent.Value != "reviewer" {
		t.Fatalf("delegate target = %q, want %q", delegate.Agent.Value, "reviewer")
	}

	ask := doc.Declarations[9].(*AskDecl)
	if len(ask.Items) != 3 {
		t.Fatalf("ask item count = %d, want 3", len(ask.Items))
	}

	pipeline := doc.Declarations[10].(*PipelineDecl)
	if len(pipeline.Stages) != 2 {
		t.Fatalf("pipeline stage count = %d, want 2", len(pipeline.Stages))
	}
	if pipeline.Stages[0].Name.Value != "explore" {
		t.Fatalf("stage 0 name = %q, want %q", pipeline.Stages[0].Name.Value, "explore")
	}
}

func TestParseSourceParsesImportsAndPromptBindings(t *testing.T) {
	doc, err := ParseSource("imports.euclo", `thoughtrecipe review_flow
"Route review questions."

trigger as capability:
  may read workspace

import prompt named.euclo.code.explore as explore
import recipe named.euclo.review.basic as review_basic

agent reviewer uses react

run reviewer:
  from input.prompt
  goal prompt explore

ask user:
  question prompt clarify_question
`)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}

	if got, want := len(doc.Declarations), 6; got != want {
		t.Fatalf("declaration count = %d, want %d", got, want)
	}

	importPrompt, ok := doc.Declarations[1].(*ImportDecl)
	if !ok {
		t.Fatalf("declaration 1 type = %T, want *ImportDecl", doc.Declarations[1])
	}
	if got := importPrompt.Kind; got != ImportKindPrompt {
		t.Fatalf("prompt import kind = %q, want %q", got, ImportKindPrompt)
	}
	if got := importPrompt.Target.Raw; got != "named.euclo.code.explore" {
		t.Fatalf("prompt import target = %q, want %q", got, "named.euclo.code.explore")
	}
	if got := importPrompt.Alias.Value; got != "explore" {
		t.Fatalf("prompt import alias = %q, want %q", got, "explore")
	}

	importRecipe, ok := doc.Declarations[2].(*ImportDecl)
	if !ok {
		t.Fatalf("declaration 2 type = %T, want *ImportDecl", doc.Declarations[2])
	}
	if got := importRecipe.Kind; got != ImportKindRecipe {
		t.Fatalf("recipe import kind = %q, want %q", got, ImportKindRecipe)
	}
	if got := importRecipe.Target.Raw; got != "named.euclo.review.basic" {
		t.Fatalf("recipe import target = %q, want %q", got, "named.euclo.review.basic")
	}
	if got := importRecipe.Alias.Value; got != "review_basic" {
		t.Fatalf("recipe import alias = %q, want %q", got, "review_basic")
	}

	run := doc.Declarations[4].(*RunDecl)
	if got := run.Items[1].(*GoalClause).PromptID; got == nil || got.Name.Value != "explore" {
		t.Fatalf("run goal prompt binding = %#v, want explore", got)
	}
	if got := run.Items[1].(*GoalClause).Text.Value; got != "" {
		t.Fatalf("run goal inline text = %q, want empty", got)
	}

	ask := doc.Declarations[5].(*AskDecl)
	if got := ask.Items[0].(*QuestionClause).PromptID; got == nil || got.Name.Value != "clarify_question" {
		t.Fatalf("ask question prompt binding = %#v, want clarify_question", got)
	}
	if got := ask.Items[0].(*QuestionClause).Text.Value; got != "" {
		t.Fatalf("ask question inline text = %q, want empty", got)
	}
}

func TestParseSourceRejectsMalformedImportDeclarations(t *testing.T) {
	_, err := ParseSource("bad_import.euclo", `thoughtrecipe demo
"Demo."

trigger as capability:
  may read workspace

import named.euclo.code.explore as explore
`)
	if err == nil {
		t.Fatal("expected malformed import to fail")
	}
}

func TestParseSourceRejectsMalformedPromptBindings(t *testing.T) {
	_, err := ParseSource("bad_bindings.euclo", `thoughtrecipe demo
"Demo."

trigger as capability:
  may read workspace

agent reviewer uses react

run reviewer:
  goal prompt

ask user:
  question prompt
`)
	if err == nil {
		t.Fatal("expected malformed prompt binding to fail")
	}
}

func TestParseSourceRejectsBadIndentation(t *testing.T) {
	_, err := ParseSource("bad.euclo", "thoughtrecipe demo\ntrigger as capability:\n    may read workspace\n")
	if err == nil {
		t.Fatal("expected indentation error")
	}
}

func TestParseSourceRejectsUnsupportedTopLevelItem(t *testing.T) {
	_, err := ParseSource("bad.euclo", "thoughtrecipe demo\nfoo bar\n")
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestParseSourceParsesIntentTriggerKind(t *testing.T) {
	doc, err := ParseSource("intent.euclo", `thoughtrecipe intent_demo
"Clarify a request."

trigger as intent:
  family ["clarification"]
  keyword ["clarify"]
  handoff ["intent_clarify", "route_intent"]
  may read workspace
  may ask user
`)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}
	trigger := doc.Declarations[0].(*TriggerDecl)
	if got := trigger.RouteKind; got != TriggerRouteKindIntent {
		t.Fatalf("trigger route kind = %q, want %q", got, TriggerRouteKindIntent)
	}
	if got := len(trigger.Associations); got != 3 {
		t.Fatalf("trigger associations = %d, want 3", got)
	}
}

func TestParseSourceRejectsTriggerAssociationWithoutList(t *testing.T) {
	_, err := ParseSource("bad.euclo", `thoughtrecipe demo

trigger as capability:
  family debug
`)
	if err == nil {
		t.Fatal("expected trigger association parse error")
	}
}

func TestParseSourceHandlesDeepNestedTypeExpressionsIteratively(t *testing.T) {
	var b strings.Builder
	b.WriteString("thoughtrecipe deep_types\n")
	b.WriteString("\"Deep nesting.\"\n\n")
	b.WriteString("trigger as capability:\n  may read workspace\n\n")
	b.WriteString("type Deep:\n  value: ")
	depth := 600
	for i := 0; i < depth; i++ {
		b.WriteString("list<")
	}
	b.WriteString("Text")
	for i := 0; i < depth; i++ {
		b.WriteString(">")
	}
	b.WriteString("\n")

	doc, err := ParseSource("deep.euclo", b.String())
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}
	typeDecl := doc.Declarations[1].(*TypeDecl)
	field := typeDecl.Body.(*RecordTypeDefinition).Fields[0]
	current := field.Type
	count := 0
	for {
		list, ok := current.(*ListTypeExpr)
		if !ok {
			break
		}
		count++
		current = list.Element
	}
	if count != depth {
		t.Fatalf("nested list depth = %d, want %d", count, depth)
	}
}
