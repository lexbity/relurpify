//go:build integration

package relurpic

import (
	"context"
	"path/filepath"
	"testing"

	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	archaeotensions "codeburg.org/lexbit/relurpify/archaeo/tensions"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	memorydb "codeburg.org/lexbit/relurpify/framework/memory/db"
	"codeburg.org/lexbit/relurpify/framework/patterns"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	testutil "codeburg.org/lexbit/relurpify/testutil/euclotestutil"
)

func TestRelurpicGapDetector_InvokesArchaeoPersistenceWithWorkflowScope(t *testing.T) {
	registry := capabilityRegistryForIntegration(t)
	model := &relurpicCapabilityQueueModel{
		responses: []*core.LLMResponse{
			{Text: `{"results":[{"severity":"significant","description":"Wrap returns the raw error instead of wrapping it at the boundary.","evidence_lines":[5]}]}`},
		},
	}
	indexManager, graphEngine, _, retrievalDB, sourcePath := newPatternDetectorFixtures(t)
	anchor, err := retrieval.DeclareAnchor(context.Background(), retrievalDB, retrieval.AnchorDeclaration{
		Term:       "error wrapping",
		Definition: "Boundary functions wrap returned errors.",
		Class:      "commitment",
	}, "workspace", string(patterns.TrustClassBuiltinTrusted))
	if err != nil {
		t.Fatalf("DeclareAnchor: %v", err)
	}

	planStore := &stubRelurpicPlanStore{
		plan: &frameworkplan.LivingPlan{
			ID:         "plan-1",
			WorkflowID: "wf-1",
			Steps: map[string]*frameworkplan.PlanStep{
				"step-1": {
					ID:            "step-1",
					Status:        frameworkplan.PlanStepPending,
					InvalidatedBy: []frameworkplan.InvalidationRule{{Kind: frameworkplan.InvalidationAnchorDrifted, Target: anchor.AnchorID}},
				},
			},
			StepOrder: []string{"step-1"},
		},
	}
	workflowStore, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("NewSQLiteWorkflowStateStore: %v", err)
	}
	t.Cleanup(func() { _ = workflowStore.Close() })
	if err := workflowStore.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "task-gap",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "detect gap",
		Status:      memory.WorkflowRunStatusRunning,
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	if err := RegisterBuiltinRelurpicCapabilities(registry, model, &core.Config{Name: "coding", Model: "stub"},
		WithIndexManager(indexManager),
		WithGraphDB(graphEngine),
		WithRetrievalDB(retrievalDB),
		WithPlanStore(planStore),
		WithWorkflowStore(workflowStore),
	); err != nil {
		t.Fatalf("RegisterBuiltinRelurpicCapabilities: %v", err)
	}

	result, err := registry.InvokeCapability(context.Background(), core.NewContext(), "gap-detector.detect", map[string]interface{}{
		"file_path":    sourcePath,
		"corpus_scope": "workspace",
		"anchor_ids":   []any{anchor.AnchorID},
		"workflow_id":  "wf-1",
	})
	if err != nil {
		t.Fatalf("InvokeCapability: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success result, got %+v", result)
	}

	tensions, err := (archaeotensions.Service{Store: workflowStore}).ListByWorkflow(context.Background(), "wf-1")
	if err != nil {
		t.Fatalf("ListByWorkflow: %v", err)
	}
	if len(tensions) != 1 {
		t.Fatalf("len(tensions) = %d, want 1", len(tensions))
	}
	if tensions[0].WorkflowID != "wf-1" {
		t.Fatalf("workflow id = %q, want wf-1", tensions[0].WorkflowID)
	}
	if tensions[0].Status != archaeodomain.TensionUnresolved {
		t.Fatalf("status = %q, want %q", tensions[0].Status, archaeodomain.TensionUnresolved)
	}
}

func TestRelurpicCapabilityHandler_NilStateDoesNotPanic(t *testing.T) {
	handler := &AgentCapabilityHandler{
		env:       testutil.Env(t),
		agentType: "goalcon",
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("Invoke panicked: %v", recovered)
		}
	}()

	_, _ = handler.Invoke(context.Background(), nil, map[string]interface{}{
		"instruction": "analyze goal feasibility",
		"task_id":     "goalcon-integration",
	})
}

func capabilityRegistryForIntegration(t *testing.T) *capability.Registry {
	t.Helper()
	registry := capability.NewRegistry()
	registerRelurpicReadTool(t, registry, ".")
	return registry
}
