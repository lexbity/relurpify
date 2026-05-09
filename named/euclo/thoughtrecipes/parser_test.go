package thoughtrecipe

import "testing"

func TestParseSourceParsesCoreThoughtRecipeConstructs(t *testing.T) {
	src := `thoughtrecipe code_assistant
"Route code requests."

trigger as capability:
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
}
