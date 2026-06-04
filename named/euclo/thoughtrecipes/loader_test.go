package thoughtrecipe

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/prompt/prompttest"
	"codeburg.org/lexbit/relurpify/framework/services"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
)

func TestThoughtRecipeLoaderRejectsBytes(t *testing.T) {
	loader := NewLoader()

	_, err := loader.LoadFromBytes([]byte("thoughtrecipe test\n"))
	if err == nil {
		t.Fatal("expected legacy thoughtrecipe loader rejection")
	}
	if !errors.Is(err, ErrYAMLThoughtRecipeLoadingRemoved) {
		t.Fatalf("expected ErrYAMLThoughtRecipeLoadingRemoved, got %v", err)
	}
}

func TestThoughtRecipeLoaderRejectsFiles(t *testing.T) {
	loader := NewLoader()

	_, err := loader.LoadFromFile("test.thoughtrecipe")
	if err == nil {
		t.Fatal("expected file loader rejection")
	}
	if !errors.Is(err, ErrYAMLThoughtRecipeLoadingRemoved) {
		t.Fatalf("expected ErrYAMLThoughtRecipeLoadingRemoved, got %v", err)
	}
}

func TestThoughtRecipeLoaderScansAcceptedExtensionsInOrder(t *testing.T) {
	root := t.TempDir()
	sourceRoot := filepath.Join(root, ThoughtRecipeSourceRoot)
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatalf("mkdir source root: %v", err)
	}

	files := map[string]string{
		"b.euclo":         "thoughtrecipe b\n\ntrigger as capability:\n  may read workspace\n",
		"a.thoughtrecipe": "thoughtrecipe a\n\ntrigger as capability:\n  may read workspace\n",
		"c.erpe":          "thoughtrecipe c\n\ntrigger as capability:\n  may read workspace\n",
		"ignore.txt":      "not a thoughtrecipe\n",
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(sourceRoot, name), []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	loader := NewLoader()
	result, err := loader.LoadWorkspace(root)
	if err != nil {
		t.Fatalf("LoadWorkspace failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.SourceRoot != sourceRoot {
		t.Fatalf("SourceRoot = %q, want %q", result.SourceRoot, sourceRoot)
	}
	if got := len(result.Sources); got != 3 {
		t.Fatalf("expected 3 accepted sources, got %d", got)
	}

	wantNames := []string{"a", "b", "c"}
	for i, source := range result.Sources {
		if source.Name != wantNames[i] {
			t.Fatalf("Sources[%d].Name = %q, want %q", i, source.Name, wantNames[i])
		}
	}

	if got := len(result.Warnings); got != 1 {
		t.Fatalf("expected 1 warning, got %d", got)
	}
	if result.Warnings[0].Path != filepath.Join(sourceRoot, "ignore.txt") {
		t.Fatalf("warning path = %q, want %q", result.Warnings[0].Path, filepath.Join(sourceRoot, "ignore.txt"))
	}
}

func TestThoughtRecipeLoaderIgnoresNestedDirectories(t *testing.T) {
	root := t.TempDir()
	sourceRoot := filepath.Join(root, ThoughtRecipeSourceRoot)
	if err := os.MkdirAll(filepath.Join(sourceRoot, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "nested", "inner.euclo"), []byte("thoughtrecipe nested\n"), 0o644); err != nil {
		t.Fatalf("write nested file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "top.euclo"), []byte("thoughtrecipe top\n\ntrigger as capability:\n  may read workspace\n"), 0o644); err != nil {
		t.Fatalf("write top file: %v", err)
	}

	loader := NewLoader()
	result, err := loader.LoadWorkspace(root)
	if err != nil {
		t.Fatalf("LoadWorkspace failed: %v", err)
	}

	if got := len(result.Sources); got != 1 {
		t.Fatalf("expected 1 top-level source, got %d", got)
	}
	if result.Sources[0].Name != "top" {
		t.Fatalf("expected top-level file to be loaded, got %q", result.Sources[0].Name)
	}
	if got := len(result.Warnings); got != 1 {
		t.Fatalf("expected 1 warning for nested directory, got %d", got)
	}
	if result.Warnings[0].Path != filepath.Join(sourceRoot, "nested") {
		t.Fatalf("warning path = %q, want %q", result.Warnings[0].Path, filepath.Join(sourceRoot, "nested"))
	}
}

func TestThoughtRecipeLoaderWarnsOnDuplicateThoughtRecipeNames(t *testing.T) {
	root := t.TempDir()
	sourceRoot := filepath.Join(root, ThoughtRecipeSourceRoot)
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatalf("mkdir source root: %v", err)
	}

	if err := os.WriteFile(filepath.Join(sourceRoot, "a.euclo"), []byte(`thoughtrecipe shared
"First."

trigger as capability:
  may read workspace
`), 0o644); err != nil {
		t.Fatalf("write first thoughtrecipe: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "b.euclo"), []byte(`thoughtrecipe shared
"Second."

trigger as capability:
  may read workspace
`), 0o644); err != nil {
		t.Fatalf("write second thoughtrecipe: %v", err)
	}

	result, err := NewLoader().LoadWorkspace(root)
	if err != nil {
		t.Fatalf("LoadWorkspace failed: %v", err)
	}

	if got := len(result.Warnings); got != 1 {
		t.Fatalf("expected 1 duplicate warning, got %d", got)
	}
	if result.Warnings[0].Path != filepath.Join(sourceRoot, "b.euclo") {
		t.Fatalf("warning path = %q, want %q", result.Warnings[0].Path, filepath.Join(sourceRoot, "b.euclo"))
	}
	if got := result.Registry.Count(); got != 1 {
		t.Fatalf("expected first-wins registry count 1, got %d", got)
	}
	thoughtrecipe, ok := result.Registry.Get("shared")
	if !ok || thoughtrecipe == nil {
		t.Fatal("expected shared thoughtrecipe to be registered")
	}
	if thoughtrecipe.Description != "First." {
		t.Fatalf("expected first thoughtrecipe to win, got description %q", thoughtrecipe.Description)
	}
	if plan, ok := result.Registry.GetPlan("shared"); !ok || plan == nil {
		t.Fatal("expected compiled plan to be registered")
	}
}

func TestThoughtRecipeLoaderCapturesIntentRouteKind(t *testing.T) {
	root := t.TempDir()
	sourceRoot := filepath.Join(root, ThoughtRecipeSourceRoot)
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatalf("mkdir source root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "intent.euclo"), []byte(`thoughtrecipe intent_route
"Clarify."

trigger as intent:
  family ["clarification"]
  keyword ["clarify", "route"]
  handoff ["intent_clarify"]
  may read workspace
`), 0o644); err != nil {
		t.Fatalf("write intent thoughtrecipe: %v", err)
	}

	result, err := NewLoader().LoadWorkspace(root)
	if err != nil {
		t.Fatalf("LoadWorkspace failed: %v", err)
	}
	thoughtrecipe, ok := result.Registry.Get("intent_route")
	if !ok || thoughtrecipe == nil {
		t.Fatal("expected intent thoughtrecipe to be registered")
	}
	if thoughtrecipe.RouteKind != surface.TriggerRouteKindIntent {
		t.Fatalf("thoughtrecipe route kind = %q, want %q", thoughtrecipe.RouteKind, surface.TriggerRouteKindIntent)
	}
	if got := thoughtrecipe.Metadata.Families; len(got) != 1 || got[0] != "clarification" {
		t.Fatalf("thoughtrecipe families = %#v, want [clarification]", got)
	}
	if got := thoughtrecipe.Metadata.Keywords; len(got) != 2 || got[0] != "clarify" || got[1] != "route" {
		t.Fatalf("thoughtrecipe keywords = %#v, want [clarify route]", got)
	}
	if got := thoughtrecipe.Metadata.HandoffTargets; len(got) != 1 || got[0] != "intent_clarify" {
		t.Fatalf("thoughtrecipe handoff targets = %#v, want [intent_clarify]", got)
	}
	if plan, ok := result.Registry.GetPlan("intent_route"); !ok || plan == nil {
		t.Fatal("expected compiled plan for intent thoughtrecipe")
	} else if plan.RouteKind != surface.TriggerRouteKindIntent {
		t.Fatalf("compiled plan route kind = %q, want %q", plan.RouteKind, surface.TriggerRouteKindIntent)
	}
}

func TestThoughtRecipeLoaderRejectsPromptImportsWithoutPromptRegistry(t *testing.T) {
	root := t.TempDir()
	sourceRoot := filepath.Join(root, ThoughtRecipeSourceRoot)
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatalf("mkdir source root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "prompt_import.euclo"), []byte(`thoughtrecipe prompt_import
"Prompt import."

trigger as capability:
  may read workspace

import prompt named.euclo.code.explore as explore

agent reviewer uses react

run reviewer:
  goal prompt explore
`), 0o644); err != nil {
		t.Fatalf("write prompt import thoughtrecipe: %v", err)
	}

	result, err := NewLoader().LoadWorkspace(root)
	if err == nil {
		t.Fatal("expected prompt import validation error without prompt registry")
	}
	if result != nil {
		t.Fatalf("expected nil result on hard validation failure, got %#v", result)
	}
}

func TestThoughtRecipeLoaderValidatesPromptImportsWithPromptRegistry(t *testing.T) {
	root := t.TempDir()
	sourceRoot := filepath.Join(root, ThoughtRecipeSourceRoot)
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatalf("mkdir source root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "prompt_import.euclo"), []byte(`thoughtrecipe prompt_import
"Prompt import."

trigger as capability:
  may read workspace

import prompt named.euclo.code.explore as explore

agent reviewer uses react

run reviewer:
  goal prompt explore
`), 0o644); err != nil {
		t.Fatalf("write prompt import thoughtrecipe: %v", err)
	}

	loader := NewLoader().WithPromptRegistry(prompttest.New().With("named.euclo.code.explore", "Explore."))
	result, err := loader.LoadWorkspace(root)
	if err != nil {
		t.Fatalf("LoadWorkspace returned error: %v", err)
	}
	if got := len(result.Warnings); got != 0 {
		t.Fatalf("expected no warnings, got %d: %#v", got, result.Warnings)
	}
	if got := result.Registry.Count(); got != 1 {
		t.Fatalf("registry count = %d, want 1", got)
	}
}

func TestThoughtRecipeLoaderValidatesPromptImportsWithRepositoryPromptRegistry(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..")
	promptRegistry, err := services.BuildPromptRegistry(repoRoot, nil)
	if err != nil {
		t.Fatalf("BuildPromptRegistry: %v", err)
	}
	for _, id := range []string{
		"named.euclo.code.explore",
		"named.euclo.intent.clarify.question.v1",
	} {
		if cfg, ok := promptRegistry.Get(id); !ok || cfg == nil {
			t.Fatalf("expected repository prompt %q to be registered", id)
		}
	}

	root := t.TempDir()
	sourceRoot := filepath.Join(root, ThoughtRecipeSourceRoot)
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatalf("mkdir source root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "prompt_import.euclo"), []byte(`thoughtrecipe prompt_import
"Prompt import."

trigger as capability:
  may read workspace
  may ask user

import prompt named.euclo.code.explore as explore
import prompt named.euclo.intent.clarify.question.v1 as clarify_question

agent reviewer uses react

run reviewer:
  goal prompt explore

ask user:
  question prompt clarify_question
`), 0o644); err != nil {
		t.Fatalf("write prompt import thoughtrecipe: %v", err)
	}

	result, err := NewLoader().WithPromptRegistry(promptRegistry).LoadWorkspace(root)
	if err != nil {
		t.Fatalf("LoadWorkspace returned error: %v", err)
	}
	if result == nil || result.Registry == nil {
		t.Fatal("expected loaded thoughtrecipe registry")
	}
	if got := result.Registry.Count(); got != 1 {
		t.Fatalf("registry count = %d, want 1", got)
	}
}

func TestThoughtRecipeLoaderStopsOnHardImportValidationFailures(t *testing.T) {
	root := t.TempDir()
	sourceRoot := filepath.Join(root, ThoughtRecipeSourceRoot)
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatalf("mkdir source root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "a.euclo"), []byte(`thoughtrecipe valid_recipe
"Valid."

trigger as capability:
  may read workspace
`), 0o644); err != nil {
		t.Fatalf("write valid thoughtrecipe: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "b.euclo"), []byte(`thoughtrecipe invalid_recipe
"Invalid."

trigger as capability:
  may read workspace

import prompt named.euclo.code.missing as explore

agent reviewer uses react

run reviewer:
  goal prompt explore
`), 0o644); err != nil {
		t.Fatalf("write invalid thoughtrecipe: %v", err)
	}

	result, err := NewLoader().WithPromptRegistry(prompttest.New().With("named.euclo.code.explore", "Explore.")).LoadWorkspace(root)
	if err == nil {
		t.Fatal("expected hard import validation failure")
	}
	if result != nil {
		t.Fatalf("expected nil result on hard failure, got %#v", result)
	}
}
