package thoughtrecipe

import (
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/execution/agentenv"
	"codeburg.org/lexbit/relurpify/execution/prompt"
)

func TestPromptAndRecipeLibraryIntegrationEndToEnd(t *testing.T) {
	workspace := t.TempDir()
	promptRoot := filepath.Join(workspace, "relurpify_cfg", "prompts")
	recipeRoot := filepath.Join(workspace, ThoughtRecipeSourceRoot)

	if err := os.MkdirAll(promptRoot, 0o755); err != nil {
		t.Fatalf("mkdir prompt root: %v", err)
	}
	if err := os.MkdirAll(recipeRoot, 0o755); err != nil {
		t.Fatalf("mkdir recipe root: %v", err)
	}

	if err := os.WriteFile(filepath.Join(promptRoot, "explore.prompt"), []byte(`---
schema framework.prompt/v2
id named.euclo.code.explore
tag "system"
---
Explore the codebase thoroughly.
`), 0o644); err != nil {
		t.Fatalf("write explore prompt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(promptRoot, "clarify.prompt"), []byte(`---
schema framework.prompt/v2
id named.euclo.intent.clarify.question.v1
tag "system"
---
Which module should be updated?
`), 0o644); err != nil {
		t.Fatalf("write clarify prompt: %v", err)
	}

	if err := os.WriteFile(filepath.Join(recipeRoot, "review_flow.euclo"), []byte(`thoughtrecipe review_flow
"Prompt-backed review flow."

trigger as capability:
  may read workspace
  may ask user

input prompt: user.request

import prompt named.euclo.code.explore as explore
import prompt named.euclo.intent.clarify.question.v1 as clarify_question

agent reviewer uses react

run reviewer:
  from input.prompt
  goal prompt explore

ask user:
  question prompt clarify_question
`), 0o644); err != nil {
		t.Fatalf("write review flow recipe: %v", err)
	}

	promptRegistry := prompt.NewRegistry()
	if err := promptRegistry.LoadDir(promptRoot); err != nil {
		t.Fatalf("LoadDir prompt registry: %v", err)
	}

	loader := NewLoader().WithPromptRegistry(promptRegistry)
	result, err := loader.LoadWorkspace(workspace)
	if err != nil {
		t.Fatalf("LoadWorkspace: %v", err)
	}
	if result == nil || result.Registry == nil {
		t.Fatal("expected loaded thoughtrecipe registry")
	}
	plan, ok := result.Registry.GetPlan("review_flow")
	if !ok || plan == nil {
		t.Fatal("expected compiled plan for review_flow")
	}
	if got := len(plan.Steps); got != 2 {
		t.Fatalf("step count = %d, want 2", got)
	}

	runNode := NewThoughtRecipeStepNode("review_flow.run", agentenv.WorkspaceEnvironment{PromptRegistry: promptRegistry}, plan.Steps[0])
	runEnv := contextdata.NewEnvelope("task-review-flow", "")
	runTask, err := runNode.buildTask(runEnv)
	if err != nil {
		t.Fatalf("buildTask(run): %v", err)
	}
	if runTask.Instruction != "Explore the codebase thoroughly." {
		t.Fatalf("run instruction = %q, want resolved prompt text", runTask.Instruction)
	}
	if got := runTask.Context["prompt_id"]; got != "named.euclo.code.explore" {
		t.Fatalf("run prompt_id = %#v, want named.euclo.code.explore", got)
	}

	askNode := NewThoughtRecipeStepNode("review_flow.ask", agentenv.WorkspaceEnvironment{PromptRegistry: promptRegistry}, plan.Steps[1])
	askTask, err := askNode.buildTask(contextdata.NewEnvelope("task-review-flow", ""))
	if err != nil {
		t.Fatalf("buildTask(ask): %v", err)
	}
	if askTask.Instruction != "Which module should be updated?" {
		t.Fatalf("ask instruction = %q, want resolved prompt text", askTask.Instruction)
	}
	if got := askTask.Context["prompt_id"]; got != "named.euclo.intent.clarify.question.v1" {
		t.Fatalf("ask prompt_id = %#v, want named.euclo.intent.clarify.question.v1", got)
	}
}
