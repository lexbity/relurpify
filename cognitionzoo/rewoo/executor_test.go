package rewoo

import (
	"context"
	"testing"

	capability "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/context/contextdata"
)

const (
	rewooTestTool = "rewoo_read"
	rewooTestPath = "docs/spec.md"
)

func newRewooTestRegistry(t *testing.T) *capability.CapabilityRegistry {
	t.Helper()
	reg := capability.NewRegistry()
	if err := reg.RegisterLegacyTool(context.Background(), scopedRewooTool{name: rewooTestTool}); err != nil {
		t.Fatalf("register %s: %v", rewooTestTool, err)
	}
	return reg
}

// TestExecutePlanWritesStepKeys pins the fix for the ReWOO step-key bug:
// executeStep must persist each result under rewoo.step.<ID>.
func TestExecutePlanWritesStepKeys(t *testing.T) {
	reg := newRewooTestRegistry(t)
	plan := &RewooPlan{
		Goal:  "",
		Steps: []RewooStep{{ID: "s1", Tool: rewooTestTool, Params: map[string]any{"path": rewooTestPath}}},
	}
	env := contextdata.NewEnvelope("task", "session")

	if _, err := ExecutePlan(context.Background(), reg, plan, env, RewooOptions{PermissionChecker: allowAllChecker()}); err != nil {
		t.Fatalf("ExecutePlan: %v", err)
	}

	raw, ok := contextdata.GetTyped[any](env, "rewoo.step.s1")
	if !ok {
		t.Fatal("expected rewoo.step.s1 to be written")
	}
	stepResult, ok := raw.(RewooStepResult)
	if !ok {
		t.Fatalf("rewoo.step.s1 type = %T, want RewooStepResult", raw)
	}
	if !stepResult.Success {
		t.Fatalf("step result = %+v, want success", stepResult)
	}
}

// TestAggregateNodeReadsExecutedSteps proves the aggregate node no longer
// reports "step not executed" for steps the executor ran.
func TestAggregateNodeReadsExecutedSteps(t *testing.T) {
	reg := newRewooTestRegistry(t)
	plan := &RewooPlan{
		Goal: "",
		Steps: []RewooStep{
			{ID: "s1", Tool: rewooTestTool},
			{ID: "s2", Tool: rewooTestTool, DependsOn: []string{"s1"}},
		},
	}
	env := contextdata.NewEnvelope("task", "session")

	if _, err := ExecutePlan(context.Background(), reg, plan, env, RewooOptions{PermissionChecker: allowAllChecker()}); err != nil {
		t.Fatalf("ExecutePlan: %v", err)
	}

	if _, err := NewAggregateNode("agg", plan).Execute(context.Background(), env); err != nil {
		t.Fatalf("AggregateNode: %v", err)
	}

	raw, ok := contextdata.GetTyped[any](env, "rewoo.tool_results")
	if !ok {
		t.Fatal("expected rewoo.tool_results")
	}
	results, ok := raw.([]RewooStepResult)
	if !ok {
		t.Fatalf("rewoo.tool_results type = %T, want []RewooStepResult", raw)
	}
	if len(results) != 2 {
		t.Fatalf("aggregated %d results, want 2", len(results))
	}
	for _, r := range results {
		if !r.Success {
			t.Fatalf("step %s reported failure: %q", r.StepID, r.Error)
		}
	}
}
