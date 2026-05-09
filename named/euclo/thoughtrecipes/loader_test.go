package thoughtrecipe

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
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
	if thoughtrecipe.RouteKind != TriggerRouteKindIntent {
		t.Fatalf("thoughtrecipe route kind = %q, want %q", thoughtrecipe.RouteKind, TriggerRouteKindIntent)
	}
	if plan, ok := result.Registry.GetPlan("intent_route"); !ok || plan == nil {
		t.Fatal("expected compiled plan for intent thoughtrecipe")
	} else if plan.RouteKind != TriggerRouteKindIntent {
		t.Fatalf("compiled plan route kind = %q, want %q", plan.RouteKind, TriggerRouteKindIntent)
	}
}
