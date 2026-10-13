package thoughtrecipe

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/descriptor"
)

// TestLoaderUnknownCapabilityFailsAtLoad: a recipe invoking an unregistered
// capability fails at LOAD with the capability named and positioned (FR-28) —
// never mid-execution.
func TestLoaderUnknownCapabilityFailsAtLoad(t *testing.T) {
	dir := t.TempDir()
	writeRecipe(t, dir, "broken.erpe", `thoughtrecipe broken_ref
"Invokes an unregistered capability."

trigger as capability:
  may read workspace

input workspace: "**/*"

route:
  otherwise:
    do relurpic:does_not_exist on input.workspace
`)
	caps := staticCapabilityLookup{"euclo:cap.known": descriptor.CapabilityDescriptor{ID: "euclo:cap.known"}}
	loader := NewLoader().WithCapabilityRegistry(caps)
	_, err := loader.LoadWorkspace(dir)
	if err == nil {
		t.Fatal("load must fail on unknown capability")
	}
	if !strings.Contains(err.Error(), "does_not_exist") {
		t.Fatalf("error must name the capability: %v", err)
	}
	if !strings.Contains(err.Error(), "broken.erpe") || !strings.Contains(err.Error(), ":") {
		t.Fatalf("error must name file and position: %v", err)
	}
}

// TestLoaderKnownCapabilityLoads: with the capability registered, the same
// recipe loads clean and the step scope grants exactly that capability.
func TestLoaderKnownCapabilityLoads(t *testing.T) {
	dir := t.TempDir()
	writeRecipe(t, dir, "ok.erpe", `thoughtrecipe known_ref
"Invokes a registered capability."

trigger as capability:
  may read workspace

input workspace: "**/*"

route:
  otherwise:
    do relurpic:known on input.workspace
`)
	caps := staticCapabilityLookup{"euclo:cap.known": descriptor.CapabilityDescriptor{ID: "euclo:cap.known"}}
	loader := NewLoader().WithCapabilityRegistry(caps)
	result, err := loader.LoadWorkspace(dir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	plan, ok := result.Registry.GetPlan("known_ref")
	if !ok || plan == nil {
		t.Fatal("compiled plan missing")
	}
	found := false
	var walk func(steps []ExecutionStep)
	walk = func(steps []ExecutionStep) {
		for _, step := range steps {
			if step.Kind != StepKindCapability {
				continue
			}
			found = true
			if !step.Scope.Permits("euclo:cap.known") || step.Scope.Permits("euclo:cap.unknown") {
				t.Fatalf("scope = %v", step.Scope.AllowedToolNames())
			}
		}
	}
	walk(plan.Steps)
	for _, route := range plan.Routes {
		for _, branch := range route.Branches {
			walk(branch.Steps)
		}
	}
	if !found {
		t.Fatal("no capability step in compiled plan")
	}
}

// TestLoaderNilRegistryRejectsInvocations: with no registry wired, a recipe
// with capability invocations fails the load (fail-closed) rather than
// loading ungoverned steps.
func TestLoaderNilRegistryRejectsInvocations(t *testing.T) {
	dir := t.TempDir()
	writeRecipe(t, dir, "nocap.erpe", `thoughtrecipe nocap
"Invokes a capability without a wired registry."

trigger as capability:
  may read workspace

input workspace: "**/*"

route:
  otherwise:
    do relurpic:known on input.workspace
`)
	loader := NewLoader()
	if _, err := loader.LoadWorkspace(dir); err == nil {
		t.Fatal("load must fail when a registry is required but not wired")
	}
}

func writeRecipe(t *testing.T, dir, name, content string) {
	t.Helper()
	root := filepath.Join(dir, ThoughtRecipeSourceRoot)
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

type staticCapabilityLookup map[string]descriptor.CapabilityDescriptor

func (s staticCapabilityLookup) GetCapability(idOrName string) (descriptor.CapabilityDescriptor, bool) {
	d, ok := s[idOrName]
	return d, ok
}

func (s staticCapabilityLookup) CallableCapabilityNames() []string {
	names := make([]string, 0, len(s))
	for name := range s {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// classifiedCapabilityLookup is a staticCapabilityLookup that also implements
// ModelCallableToolClassifier — the registry's model-facing view.
type classifiedCapabilityLookup struct {
	staticCapabilityLookup
	modelCallable []string
}

func (c classifiedCapabilityLookup) ModelCallableToolNames() []string {
	return c.modelCallable
}

// TestFindNonCallableDeclaredTools: names that resolve in the registry but are
// absent from the model-callable view are reported; unresolved names are left
// to the load failure; lookups without the classifier report nothing.
func TestFindNonCallableDeclaredTools(t *testing.T) {
	lookup := classifiedCapabilityLookup{
		staticCapabilityLookup: staticCapabilityLookup{
			"file_read":          descriptor.CapabilityDescriptor{ID: "file_read"},
			"test_run":           descriptor.CapabilityDescriptor{ID: "euclo:cap.test_run"},
			"euclo:cap.test_run": descriptor.CapabilityDescriptor{ID: "euclo:cap.test_run"},
		},
		modelCallable: []string{"file_read"},
	}
	got := FindNonCallableDeclaredTools([]string{"file_read", "test_run", "never_registered", ""}, lookup)
	if len(got) != 1 || got[0] != "test_run" {
		t.Fatalf("FindNonCallableDeclaredTools = %v, want [test_run]", got)
	}
	if plain := FindNonCallableDeclaredTools([]string{"test_run"}, staticCapabilityLookup{"test_run": descriptor.CapabilityDescriptor{ID: "euclo:cap.test_run"}}); plain != nil {
		t.Fatalf("lookup without classifier must report nothing, got %v", plain)
	}
}

// TestLoaderWarnsVocabularyNotCallable pins the P1 contract with the exact
// situation the live test drive exposed: debug_tdd_repair declares bare
// `test_run` in `may invoke`; the name resolves (the governed capability
// euclo:cap.test_run) so the recipe loads, but the model's tool list never
// contains it. The load must carry a vocabulary_not_model_callable warning
// naming recipe, step, and tool — never silently run disarmed.
func TestLoaderWarnsVocabularyNotCallable(t *testing.T) {
	dir := t.TempDir()
	writeRecipe(t, dir, "repair.erpe", `thoughtrecipe euclo.thoughtrecipe.repair
"Repair recipe whose declared vocabulary includes a governed capability."

trigger as capability:
  may read workspace
  may invoke ["file_read", "test_run"]
  family ["debug"]

input workspace: "**/*"
input prompt: user.request

agent repairer uses react

run repairer:
  may invoke ["file_read", "test_run"]
  from input.workspace
  goal "Repair the reported failure and verify."
  capture result -> state.repair
`)
	caps := classifiedCapabilityLookup{
		staticCapabilityLookup: staticCapabilityLookup{
			"file_read":          descriptor.CapabilityDescriptor{ID: "file_read"},
			"test_run":           descriptor.CapabilityDescriptor{ID: "euclo:cap.test_run"},
			"euclo:cap.test_run": descriptor.CapabilityDescriptor{ID: "euclo:cap.test_run"},
		},
		modelCallable: []string{"file_read"},
	}
	loader := NewLoader().WithCapabilityRegistry(caps)
	result, err := loader.LoadWorkspace(dir)
	if err != nil {
		t.Fatalf("load must succeed (the name is legal vocabulary): %v", err)
	}
	var found []LoadWarning
	for _, warning := range result.Warnings {
		if warning.Kind == LoadWarningKindVocabularyNotCallable {
			found = append(found, warning)
		}
	}
	if len(found) != 1 {
		t.Fatalf("vocabulary warnings = %d (%v), want 1 (the compiled plan merges trigger and run vocabulary into one react scope)", len(found), result.Warnings)
	}
	for _, warning := range found {
		if !strings.Contains(warning.Message, "test_run") || !strings.Contains(warning.Message, "repair") {
			t.Fatalf("warning must name tool and recipe: %q", warning.Message)
		}
	}
}
