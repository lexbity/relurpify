package thoughtrecipe

import (
	"context"
	"errors"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/descriptor"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
)

// TestLoaderUnresolvedToolNameFailsLoad pins D2/FR-4: a declared `may invoke`
// name that the capability registry cannot resolve is a load error — typed
// as ErrUnresolvedToolName and naming recipe, step, and tool — never a
// silent in-step drop.
func TestLoaderUnresolvedToolNameFailsLoad(t *testing.T) {
	dir := t.TempDir()
	writeRecipe(t, dir, "disarmed.erpe", `thoughtrecipe disarmed
"Declares a tool the registry does not have."

trigger as capability:
  may read workspace
  may invoke ["file_read", "file_reed"]

input workspace: "**/*"
agent reviewer uses react

run reviewer:
  from input.workspace
  goal "Work on the workspace."
`)
	caps := staticCapabilityLookup{
		"file_read": descriptor.CapabilityDescriptor{ID: "file_read"},
	}
	_, err := NewLoader().WithCapabilityRegistry(caps).LoadWorkspace(dir)
	var unresolved *ErrUnresolvedToolName
	if !errors.As(err, &unresolved) {
		t.Fatalf("load must fail with ErrUnresolvedToolName, got %v", err)
	}
	if unresolved.Recipe != "disarmed" || unresolved.Tool != "file_reed" {
		t.Fatalf("error must name recipe and tool, got %+v", unresolved)
	}
	if unresolved.Step != "trigger" {
		t.Fatalf("error must name the declaring step (the trigger here), got %q", unresolved.Step)
	}
	if !strings.Contains(err.Error(), "does not resolve in the capability registry") {
		t.Fatalf("error must carry the normative message: %v", err)
	}
	if len(unresolved.Nearest) == 0 || unresolved.Nearest[0] != "file_read" {
		t.Fatalf("error must suggest the nearest resolvable name, got %v", unresolved.Nearest)
	}
}

// TestLoaderUnresolvedToolNameSuggestsNearest pins the Nearest contract: up
// to 5 resolvable names within Levenshtein distance 3.
func TestLoaderUnresolvedToolNameSuggestsNearest(t *testing.T) {
	got := nearestVocabularyNames("file_reed", []string{"file_read", "file_write", "file_list", "file_search", "file_edit", "file_create", "file_delete"})
	if len(got) == 0 || got[0] != "file_read" {
		t.Fatalf("nearest must rank file_read first for file_reed, got %v", got)
	}
	if len(got) > 5 {
		t.Fatalf("nearest is capped at 5 suggestions, got %d", len(got))
	}
	if nearest := nearestVocabularyNames("zzzzzzzz", []string{"file_read"}); nearest != nil {
		t.Fatalf("distant names must not be suggested, got %v", nearest)
	}
}

// TestProgrammaticRegisterCompiledRejectsUnresolvedVocabulary pins FR-14:
// a compiled plan registered programmatically (bypassing the loader) fails
// registration when its declared vocabulary does not resolve.
func TestProgrammaticRegisterCompiledRejectsUnresolvedVocabulary(t *testing.T) {
	plan := &ExecutionPlan{
		ThoughtRecipe: &surface.ThoughtRecipe{Name: "prog_recipe"},
		Steps: []ExecutionStep{
			{ID: "reviewer", Paradigm: "react", Scope: AllowTools([]string{"file_read", "ghost_tool"})},
		},
	}
	vocab := staticCapabilityLookup{
		"file_read": descriptor.CapabilityDescriptor{ID: "file_read"},
	}
	reg := NewThoughtRecipeRegistry().WithToolVocabulary(vocab)
	err := reg.RegisterCompiled(&surface.ThoughtRecipe{Name: "prog_recipe"}, plan, "programmatic")
	var unresolved *ErrUnresolvedToolName
	if !errors.As(err, &unresolved) || unresolved.Tool != "ghost_tool" {
		t.Fatalf("registration must fail with ErrUnresolvedToolName naming ghost_tool, got %v", err)
	}
}

// TestStepScopingUnresolvedNameIsTypedError pins the step-time defensive arm
// (FR-14): the deleted silent continue is replaced by a typed
// ErrUnresolvedToolName from the scoping path.
func TestStepScopingUnresolvedNameIsTypedError(t *testing.T) {
	reg := registry.NewRegistry()
	if err := reg.RegisterLegacyTool(context.Background(), semanticTestTool{name: "file_read", available: true}); err != nil {
		t.Fatalf("register file_read: %v", err)
	}
	c := &stepCore{
		id:   "reviewer.execute",
		deps: &paradigm.Deps{Registry: reg},
		step: ExecutionStep{ID: "reviewer.execute", Paradigm: "react", Scope: AllowTools([]string{"file_read", "ghost_tool"})},
	}
	_, err := c.effectiveToolAllowlist()
	var unresolved *ErrUnresolvedToolName
	if !errors.As(err, &unresolved) || unresolved.Tool != "ghost_tool" || unresolved.Step != "reviewer.execute" {
		t.Fatalf("scoping must return ErrUnresolvedToolName naming ghost_tool and the step, got %v", err)
	}
	if _, scopingErr := c.scopedRegistry(); !errors.As(scopingErr, &unresolved) {
		t.Fatalf("scopedRegistry must propagate the typed unresolved error, got %v", scopingErr)
	}
}
