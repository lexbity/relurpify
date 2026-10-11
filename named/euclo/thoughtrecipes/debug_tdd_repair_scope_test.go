package thoughtrecipe

import (
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/descriptor"
)

// TestDebugTddRepairStepScope compiles the canonical debug_tdd_repair recipe
// and pins the lowering's scope merge: the repairer run's scope is the UNION
// of the trigger-level and run-level may invoke declarations
// (file_read, grep, test_run, file_write) — a scope missing the run-level
// additions silently disarms the agent (the live scratch-turn finding). The
// capability step's scope is by design the capability itself.
func TestDebugTddRepairStepScope(t *testing.T) {
	dir := t.TempDir()
	src := `thoughtrecipe euclo.thoughtrecipe.debug_tdd_repair
"Debug/repair-family recipe."

trigger as capability:
  may read workspace
  may write workspace
  may invoke ["file_read", "grep", "test_run"]
  family ["debug"]

input workspace: "**/*"
input prompt: user.request

agent repairer uses react

run repairer:
  may invoke ["file_read", "grep", "test_run", "file_write"]
  from input.workspace
  goal "Find the failing test, repair the underlying cause, and re-run the test suite to verify."
  capture result -> state.repair

route:
  otherwise:
    do relurpic:test_run on state.repair
`
	if err := os.MkdirAll(filepath.Join(dir, "relurpify_cfg", "euclo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "relurpify_cfg", "euclo", "recipe.erpe"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	caps := staticCapabilityLookup{
		"file_read":  descriptor.CapabilityDescriptor{ID: "file_read"},
		"grep":       descriptor.CapabilityDescriptor{ID: "file_search"},
		"test_run":   descriptor.CapabilityDescriptor{ID: "euclo:cap.test_run"},
		"file_write": descriptor.CapabilityDescriptor{ID: "file_write"},
	}
	loader := NewLoader().WithCapabilityRegistry(caps)
	result, err := loader.LoadWorkspace(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	plan, ok := result.Registry.GetPlan("euclo.thoughtrecipe.debug_tdd_repair")
	if !ok {
		t.Fatal("compiled plan missing")
	}

	var scopes [][]string
	var walk func(steps []ExecutionStep)
	walk = func(steps []ExecutionStep) {
		for _, step := range steps {
			scopes = append(scopes, step.Scope.AllowedToolNames())
		}
	}
	walk(plan.Steps)
	for _, route := range plan.Routes {
		for _, branch := range route.Branches {
			walk(branch.Steps)
		}
	}
	if len(scopes) < 2 {
		t.Fatalf("expected the react step and the capability step, got %d scopes", len(scopes))
	}
	runScope, capabilityScope := scopes[0], scopes[1]
	for _, want := range []string{"file_read", "grep", "test_run", "file_write"} {
		found := false
		for _, name := range runScope {
			if name == want {
				found = true
			}
		}
		if !found {
			t.Errorf("run scope missing %q: scope=%v (trigger+run may invoke must merge)", want, runScope)
		}
	}
	if len(capabilityScope) != 1 || capabilityScope[0] != "euclo:cap.test_run" {
		t.Errorf("capability step scope must be exactly its own capability, got %v", capabilityScope)
	}
}
