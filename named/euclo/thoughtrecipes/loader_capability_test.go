package thoughtrecipe

import (
	"os"
	"path/filepath"
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

func (s staticCapabilityLookup) Select(id string) (descriptor.CapabilityDescriptor, bool) {
	d, ok := s[id]
	return d, ok
}
