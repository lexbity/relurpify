package rex

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/rex/route"
	rexruntime "codeburg.org/lexbit/relurpify/named/rex/runtime"
	rexstore "codeburg.org/lexbit/relurpify/named/rex/store"
)

type planExecutionWorkflowStoreProvider struct {
	workflow *rexstore.SQLiteWorkflowStore
}

func (p planExecutionWorkflowStoreProvider) WorkflowStore() *rexstore.SQLiteWorkflowStore {
	return p.workflow
}

func TestPlanExecutionReturnsExpectedPlan(t *testing.T) {
	store, err := rexstore.NewSQLiteWorkflowStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewSQLiteWorkflowStore: %v", err)
	}
	env := testEnv(t)
	agent := New(env)
	agent.Runtime = rexruntime.New(agent.rexConfig, planExecutionWorkflowStoreProvider{workflow: store})

	task := &core.Task{
		ID:          "task-1",
		Instruction: "write code for a new feature",
		Type:        string(core.TaskTypeCodeGeneration),
		Context: map[string]any{
			"workspace":      t.TempDir(),
			"edit_permitted": true,
		},
	}
	envelope := contextdata.NewEnvelope(task.ID, "")

	surfaces := agent.openSurfaces(context.Background(), task)
	if surfaces.Workflow != store {
		t.Fatalf("openSurfaces workflow = %v, want %v", surfaces.Workflow, store)
	}

	plan, err := agent.planExecution(context.Background(), task, envelope, surfaces)
	if err != nil {
		t.Fatalf("planExecution: %v", err)
	}
	if plan.Decision.Family != route.FamilyArchitect {
		t.Fatalf("Decision.Family = %q, want %q", plan.Decision.Family, route.FamilyArchitect)
	}
	if plan.RoutePlan.PrimaryFamily != route.FamilyArchitect {
		t.Fatalf("RoutePlan.PrimaryFamily = %q, want %q", plan.RoutePlan.PrimaryFamily, route.FamilyArchitect)
	}
	if plan.Delegate == nil {
		t.Fatal("expected delegate to be resolved")
	}
	if got := plan.Delegate.Family(); got != route.FamilyArchitect {
		t.Fatalf("Delegate.Family() = %q, want %q", got, route.FamilyArchitect)
	}
	if !plan.RoutePlan.RequireRetrieval {
		t.Fatal("expected retrieval to be required for architect route")
	}
	if plan.Identity.WorkflowID == "" || plan.Identity.RunID == "" {
		t.Fatalf("unexpected identity: %+v", plan.Identity)
	}
	if plan.ExecutionTask != task {
		t.Fatalf("ExecutionTask = %p, want %p", plan.ExecutionTask, task)
	}
}
