package euclotest

import (
	"context"
	"path/filepath"
	"testing"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/guidance"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	"github.com/lexcodex/relurpify/named/euclo"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	"github.com/lexcodex/relurpify/testutil/euclotestutil"
	"github.com/stretchr/testify/require"
)

func TestAgentExecuteRepeatedCompactionRestorePreservesContinuity(t *testing.T) {
	ctx := context.Background()
	workflowStore, runtimeStore := openWorkflowAndRuntimeStores(t)
	defer workflowStore.Close()
	defer runtimeStore.Close()

	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(testutil.FileWriteTool{}))
	store := memory.NewCompositeRuntimeStore(workflowStore, runtimeStore, nil)
	workspace := t.TempDir()

	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: registry,
		Memory:   store,
		Config:   &core.Config{Name: "euclo-reliability-restore", Model: "stub", MaxIterations: 1},
	})

	initialState := core.NewContext()
	initialState.Set("pipeline.verify", passingVerification())
	initialState.Set("euclo.active_plan_version", archaeodomain.VersionedLivingPlan{
		ID:         "plan-version-1",
		WorkflowID: "wf-repeated-restore",
		Version:    2,
		Status:     archaeodomain.LivingPlanVersionActive,
		Plan: frameworkplan.LivingPlan{
			ID:         "plan-1",
			WorkflowID: "wf-repeated-restore",
			StepOrder:  []string{"step-1"},
			Steps: map[string]*frameworkplan.PlanStep{
				"step-1": {ID: "step-1", Status: frameworkplan.PlanStepInProgress},
			},
		},
		PatternRefs:             []string{"pattern-1"},
		TensionRefs:             []string{"tension-1"},
		FormationProvenanceRefs: []string{"prov-1"},
	})

	_, err := agent.Execute(ctx, &core.Task{
		ID:          "task-restore-prime",
		Instruction: "plan and implement the change",
		Context: map[string]any{
			"workspace":   workspace,
			"workflow_id": "wf-repeated-restore",
			"mode":        "planning",
		},
	}, initialState)
	require.NoError(t, err)

	workflowID := initialState.GetString("euclo.workflow_id")
	runID := initialState.GetString("euclo.run_id")
	require.NotEmpty(t, workflowID)
	require.NotEmpty(t, runID)

	for i := 0; i < 3; i++ {
		resumeState := core.NewContext()
		resumeState.Set("pipeline.verify", passingVerification())
		resumeState.Set("euclo.context_compaction", eucloruntime.ContextLifecycleState{
			WorkflowID:         workflowID,
			RunID:              runID,
			Stage:              eucloruntime.ContextLifecycleStageCompacted,
			RestoreRequired:    true,
			CompactionEligible: true,
			CompactionCount:    i + 1,
		})
		result, err := agent.Execute(ctx, &core.Task{
			ID:          "task-restore-resume",
			Instruction: "continue the workflow after compaction",
			Context: map[string]any{
				"workspace":                workspace,
				"workflow_id":              workflowID,
				"run_id":                   runID,
				"mode":                     "planning",
				"euclo.restore_continuity": true,
			},
		}, resumeState)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotEqual(t, "restore_failed", result.Metadata["result_class"])

		rawUOW, ok := resumeState.Get("euclo.unit_of_work")
		require.True(t, ok)
		uow, ok := rawUOW.(eucloruntime.UnitOfWork)
		require.True(t, ok)
		require.NotNil(t, uow.PlanBinding)
		require.Equal(t, "plan-1", uow.PlanBinding.PlanID)
		require.Equal(t, 2, uow.PlanBinding.PlanVersion)

		rawLifecycle, ok := resumeState.Get("euclo.context_compaction")
		require.True(t, ok)
		lifecycle, ok := rawLifecycle.(eucloruntime.ContextLifecycleState)
		require.True(t, ok)
		require.Equal(t, eucloruntime.ContextLifecycleStageRestored, lifecycle.Stage)
		require.GreaterOrEqual(t, lifecycle.RestoreCount, 1)
	}
}

func TestAgentExecuteProviderDegradationContinuesWithDeferrals(t *testing.T) {
	ctx := context.Background()
	workflowStore, runtimeStore := openWorkflowAndRuntimeStores(t)
	defer workflowStore.Close()
	defer runtimeStore.Close()

	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(testutil.FileWriteTool{}))
	store := memory.NewCompositeRuntimeStore(workflowStore, runtimeStore, nil)
	workspace := t.TempDir()

	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: registry,
		Memory:   store,
		Config:   &core.Config{Name: "euclo-provider-degradation", Model: "stub", MaxIterations: 1},
	})
	agent.DeferralPlan = &guidance.DeferralPlan{ID: "def-plan-provider", WorkflowID: "wf-provider-degradation"}
	agent.DeferralPlan.AddObservation(guidance.EngineeringObservation{
		ID:           "provider-degraded",
		Source:       "provider.monitor",
		GuidanceKind: guidance.GuidanceRecovery,
		Title:        "Provider degraded during execution",
		Description:  "Execution continued while preserving a provider constraint for later review.",
		BlastRadius:  2,
		Evidence: map[string]any{
			"provider_constraint":     true,
			"provider_state_snapshot": map[string]any{"llm": "degraded"},
			"request_refs":            []string{"req-provider-1"},
		},
	})
	agent.RuntimeProviders = []core.Provider{&providerRestoreHarness{
		desc: core.ProviderDescriptor{
			ID:                 "provider-low-trust",
			Kind:               core.ProviderKindMCPClient,
			TrustBaseline:      core.TrustClassRemoteDeclared,
			RecoverabilityMode: core.RecoverabilityEphemeral,
		},
	}}

	state := core.NewContext()
	state.Set("pipeline.verify", passingVerification())
	result, err := agent.Execute(ctx, &core.Task{
		ID:          "task-provider-degradation",
		Instruction: "summarize current status",
		Context: map[string]any{
			"workspace":   workspace,
			"workflow_id": "wf-provider-degradation",
		},
	}, state)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "completed_with_deferrals", result.Metadata["result_class"])

	raw, ok := state.Get("euclo.deferred_execution_issues")
	require.True(t, ok)
	issues, ok := raw.([]eucloruntime.DeferredExecutionIssue)
	require.True(t, ok)
	require.Len(t, issues, 1)
	require.Equal(t, eucloruntime.DeferredIssueProviderConstraint, issues[0].Kind)
	require.FileExists(t, issues[0].WorkspaceArtifactPath)

	raw, ok = state.Get("euclo.security_runtime")
	require.True(t, ok)
	security, ok := raw.(eucloruntime.SecurityRuntimeState)
	require.True(t, ok)
	require.False(t, security.Blocked)
	require.NotEmpty(t, security.Diagnostics)

	finalOutput, ok := result.Data["final_output"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "completed_with_deferrals", finalOutput["result_class"])
}

func openWorkflowAndRuntimeStores(t *testing.T) (*db.SQLiteWorkflowStateStore, *db.SQLiteRuntimeMemoryStore) {
	t.Helper()
	workflowStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	runtimeStore, err := db.NewSQLiteRuntimeMemoryStore(filepath.Join(t.TempDir(), "runtime.db"))
	require.NoError(t, err)
	return workflowStore, runtimeStore
}

func passingVerification() map[string]any {
	return map[string]any{
		"status":  "pass",
		"summary": "verification passed",
		"checks":  []any{map[string]any{"name": "go test ./...", "status": "pass"}},
	}
}
