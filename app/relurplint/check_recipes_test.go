package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/fs"
	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
)

func TestRecipesCheckNoDirectory(t *testing.T) {
	workspace := t.TempDir()
	c := recipesCheck{}
	diags := c.Run(workspace)
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics for missing recipe dir, got %d", len(diags))
	}
}

func TestRecipesCheckEmptyDirectory(t *testing.T) {
	workspace := t.TempDir()
	_ = os.MkdirAll(filepath.Join(workspace, "relurpify_cfg", "euclo"), fs.PublicDirMode) // public: test dir
	c := recipesCheck{}
	diags := c.Run(workspace)
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics for empty recipe dir, got %d", len(diags))
	}
}

func TestRecipesCheckValidRecipe(t *testing.T) {
	workspace := t.TempDir()
	recipesDir := filepath.Join(workspace, "relurpify_cfg", "euclo")
	_ = os.MkdirAll(recipesDir, fs.PublicDirMode) // public: test dir
	testhelper.MustWrite(t, filepath.Join(recipesDir, "hello.erpe"), `thoughtrecipe hello

trigger as capability:
  may read workspace
`)

	c := recipesCheck{}
	diags := c.Run(workspace)
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics for valid recipe, got %d: %+v", len(diags), diags)
	}
}

func TestRecipesCheckParseError(t *testing.T) {
	workspace := t.TempDir()
	recipesDir := filepath.Join(workspace, "relurpify_cfg", "euclo")
	_ = os.MkdirAll(recipesDir, fs.PublicDirMode) // public: test dir
	testhelper.MustWrite(t, filepath.Join(recipesDir, "broken.erpe"), `thoughtrecipe broken

trigger as capability:
  may read workspace
{{bad
`)

	c := recipesCheck{}
	diags := c.Run(workspace)
	if len(diags) == 0 {
		t.Fatal("expected diagnostics for parse error, got none")
	}
	found := false
	for _, d := range diags {
		if d.Code == "recipes.parse" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected recipes.parse diagnostic, got: %+v", diags)
	}
}

func TestRecipesCheckIgnoresNonRecipeFiles(t *testing.T) {
	workspace := t.TempDir()
	recipesDir := filepath.Join(workspace, "relurpify_cfg", "euclo")
	_ = os.MkdirAll(recipesDir, fs.PublicDirMode) // public: test dir
	testhelper.MustWrite(t, filepath.Join(recipesDir, "notes.txt"), "this is not a recipe")

	c := recipesCheck{}
	diags := c.Run(workspace)
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics for non-recipe files, got %d", len(diags))
	}
}

func TestRecipesCheckCleanRepo(t *testing.T) {
	c := recipesCheck{}
	diags := c.Run(testRepoRoot)
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics for clean repo, got %d: %+v", len(diags), diags)
	}
}

func TestRecipesCheckUnknownEpistemicsAnnotation(t *testing.T) {
	workspace := t.TempDir()
	recipesDir := filepath.Join(workspace, "relurpify_cfg", "euclo")
	_ = os.MkdirAll(recipesDir, fs.PublicDirMode) // public: test dir
	testhelper.MustWrite(t, filepath.Join(recipesDir, "bad_epistemics.erpe"), `thoughtrecipe bad_epistemics

trigger as capability:
  may read workspace

agent reviewer uses react

run reviewer:
  capture:
    input.prompt : Text as verified -> state.answer
`)

	c := recipesCheck{}
	diags := c.Run(workspace)
	if len(diags) == 0 {
		t.Fatal("expected a diagnostic for an unknown epistemic annotation, got none")
	}
	found := false
	for _, d := range diags {
		if strings.Contains(d.Message, "epistemic annotation") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected an epistemic-vocabulary diagnostic, got: %+v", diags)
	}
}

// TestRecipesCheckFlagsRetiredDirective: a recipe carrying a directive no
// paradigm honors (here: `method` on a planner agent — retired by the contract
// audit) is a load error at lint time, not a silent no-op.
func TestRecipesCheckFlagsRetiredDirective(t *testing.T) {
	workspace := t.TempDir()
	recipesDir := filepath.Join(workspace, "relurpify_cfg", "euclo")
	_ = os.MkdirAll(recipesDir, fs.PublicDirMode) // public: test dir
	testhelper.MustWrite(t, filepath.Join(recipesDir, "retired.erpe"), `thoughtrecipe retired

trigger as capability:
  may read workspace

agent planner uses planner

run planner:
  method "decompose"
`)

	c := recipesCheck{}
	diags := c.Run(workspace)
	found := false
	for _, d := range diags {
		if d.Code == "recipes.contract" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected recipes.contract diagnostic for retired directive, got: %+v", diags)
	}
}
