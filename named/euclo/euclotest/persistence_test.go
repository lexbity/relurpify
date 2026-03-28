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
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	"github.com/lexcodex/relurpify/testutil/euclotestutil"
	"github.com/stretchr/testify/require"
)

func TestAgentExecutePersistsArtifactsToWorkflowStore(t *testing.T) {
	ctx := context.Background()
	workflowStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer workflowStore.Close()
	runtimeStore, err := db.NewSQLiteRuntimeMemoryStore(filepath.Join(t.TempDir(), "runtime.db"))
	require.NoError(t, err)
	defer runtimeStore.Close()

	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(testutil.FileWriteTool{}))
	store := memory.NewCompositeRuntimeStore(workflowStore, runtimeStore, nil)

	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: registry,
		Memory:   store,
		Config:   &core.Config{Name: "euclo-persist", Model: "stub", MaxIterations: 1},
	})

	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "verification passed",
		"checks":  []any{map[string]any{"name": "go test ./...", "status": "pass"}},
	})
	result, err := agent.Execute(ctx, &core.Task{
		ID:          "task-persist",
		Instruction: "summarize current status",
		Context:     map[string]any{"workspace": "/tmp/ws"},
	}, state)
	require.NoError(t, err)
	require.NotNil(t, result)

	workflowID := state.GetString("euclo.workflow_id")
	runID := state.GetString("euclo.run_id")
	require.NotEmpty(t, workflowID)
	require.NotEmpty(t, runID)

	artifacts, listErr := workflowStore.ListWorkflowArtifacts(ctx, workflowID, runID)
	require.NoError(t, listErr)
	require.NotEmpty(t, artifacts)
	var kinds []string
	for _, artifact := range artifacts {
		kinds = append(kinds, artifact.Kind)
	}
	require.Contains(t, kinds, string(euclotypes.ArtifactKindCompiledExecution))
	require.Contains(t, kinds, string(euclotypes.ArtifactKindExecutionStatus))
	require.Contains(t, kinds, string(euclotypes.ArtifactKindFinalReport))
	providers, err := workflowStore.ListProviderSnapshots(ctx, workflowID, runID)
	require.NoError(t, err)
	require.Empty(t, providers)
}

func TestAgentExecutePersistsDeferredExecutionIssues(t *testing.T) {
	ctx := context.Background()
	workflowStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer workflowStore.Close()
	runtimeStore, err := db.NewSQLiteRuntimeMemoryStore(filepath.Join(t.TempDir(), "runtime.db"))
	require.NoError(t, err)
	defer runtimeStore.Close()

	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(testutil.FileWriteTool{}))
	store := memory.NewCompositeRuntimeStore(workflowStore, runtimeStore, nil)
	workspace := t.TempDir()

	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: registry,
		Memory:   store,
		Config:   &core.Config{Name: "euclo-persist-deferrals", Model: "stub", MaxIterations: 1},
	})
	agent.DeferralPlan = &guidance.DeferralPlan{
		ID:         "def-plan-1",
		WorkflowID: "wf-deferrals",
	}
	agent.DeferralPlan.AddObservation(guidance.EngineeringObservation{
		ID:           "obs-1",
		Source:       "provider.monitor",
		GuidanceKind: guidance.GuidanceRecovery,
		Title:        "Provider degraded during execution",
		Description:  "Execution continued with a deferred provider concern.",
		BlastRadius:  2,
		Evidence: map[string]any{
			"provider_state_snapshot": map[string]any{"llm": "degraded"},
			"request_refs":            []string{"req-1"},
		},
	})

	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "verification passed",
		"checks":  []any{map[string]any{"name": "go test ./...", "status": "pass"}},
	})
	result, err := agent.Execute(ctx, &core.Task{
		ID:          "task-deferrals",
		Instruction: "summarize current status",
		Context: map[string]any{
			"workspace":   workspace,
			"workflow_id": "wf-deferrals",
		},
	}, state)
	require.NoError(t, err)
	require.NotNil(t, result)

	workflowID := state.GetString("euclo.workflow_id")
	runID := state.GetString("euclo.run_id")
	require.Equal(t, "wf-deferrals", workflowID)
	require.NotEmpty(t, runID)

	rawIssues, ok := state.Get("euclo.deferred_execution_issues")
	require.True(t, ok)
	issues, ok := rawIssues.([]eucloruntime.DeferredExecutionIssue)
	require.True(t, ok)
	require.Len(t, issues, 1)
	require.NotEmpty(t, issues[0].WorkspaceArtifactPath)

	artifacts, listErr := workflowStore.ListWorkflowArtifacts(ctx, workflowID, runID)
	require.NoError(t, listErr)
	require.NotEmpty(t, artifacts)
	var kinds []string
	for _, artifact := range artifacts {
		kinds = append(kinds, artifact.Kind)
	}
	require.Contains(t, kinds, string(euclotypes.ArtifactKindDeferredExecutionIssues))
	require.FileExists(t, issues[0].WorkspaceArtifactPath)
	require.Equal(t, "completed_with_deferrals", result.Metadata["result_class"])
	finalOutput, ok := result.Data["final_output"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "completed_with_deferrals", finalOutput["result_class"])
}

func TestLoadPersistedArtifactsRestoresStateAndFinalReport(t *testing.T) {
	ctx := context.Background()
	writer := &workflowArtifactWriterStub{}
	artifacts := []euclotypes.Artifact{
		{ID: "euclo_intake", Kind: euclotypes.ArtifactKindIntake, Summary: "task", Payload: map[string]any{"task_id": "task-1"}},
		{ID: "euclo_mode", Kind: euclotypes.ArtifactKindModeResolution, Summary: "code", Payload: map[string]any{"mode_id": "code"}},
		{ID: "euclo_verify", Kind: euclotypes.ArtifactKindVerification, Summary: "tests passed", Payload: map[string]any{"status": "pass"}},
		{ID: "euclo_gate", Kind: euclotypes.ArtifactKindSuccessGate, Summary: "accepted", Payload: map[string]any{"allowed": true}},
	}
	require.NoError(t, euclotypes.PersistWorkflowArtifacts(ctx, writer, "wf-1", "run-1", artifacts))

	reader := &workflowArtifactReaderStub{records: writer.records}
	loaded, err := euclotypes.LoadPersistedArtifacts(ctx, reader, "wf-1", "run-1")
	require.NoError(t, err)
	require.Len(t, loaded, 4)

	state := core.NewContext()
	euclotypes.RestoreStateFromArtifacts(state, loaded)
	require.NotEmpty(t, state.GetString("pipeline.verify"))

	report := euclotypes.AssembleFinalReport(loaded)
	require.Equal(t, 4, report["artifacts"])
	require.NotNil(t, report["mode"])
	require.NotNil(t, report["verification"])
	require.NotNil(t, report["success_gate"])
}

func TestAssembleFinalReportIncludesRuntimeResultClass(t *testing.T) {
	artifacts := []euclotypes.Artifact{
		{
			ID:      "exec_status",
			Kind:    euclotypes.ArtifactKindExecutionStatus,
			Summary: "completed with deferrals",
			Payload: eucloruntime.RuntimeExecutionStatus{
				WorkflowID:       "wf-1",
				RunID:            "run-1",
				Status:           eucloruntime.ExecutionStatusCompletedWithDeferrals,
				ResultClass:      eucloruntime.ExecutionResultClassCompletedWithDeferrals,
				DeferredIssueIDs: []string{"defer-1", "defer-2"},
			},
		},
		{
			ID:      "defers",
			Kind:    euclotypes.ArtifactKindDeferredExecutionIssues,
			Summary: "2 deferred issues",
			Payload: []eucloruntime.DeferredExecutionIssue{
				{IssueID: "defer-1"},
				{IssueID: "defer-2"},
			},
		},
	}

	report := euclotypes.AssembleFinalReport(artifacts)
	require.Equal(t, "completed_with_deferrals", report["result_class"])
	require.Equal(t, []string{"defer-1", "defer-2"}, report["deferred_issue_ids"])
}

func TestAgentExecuteRestoresContinuityAfterCompaction(t *testing.T) {
	ctx := context.Background()
	workflowStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer workflowStore.Close()
	runtimeStore, err := db.NewSQLiteRuntimeMemoryStore(filepath.Join(t.TempDir(), "runtime.db"))
	require.NoError(t, err)
	defer runtimeStore.Close()

	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(testutil.FileWriteTool{}))
	store := memory.NewCompositeRuntimeStore(workflowStore, runtimeStore, nil)
	workspace := t.TempDir()

	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: registry,
		Memory:   store,
		Config:   &core.Config{Name: "euclo-restore", Model: "stub", MaxIterations: 1},
	})

	initialState := core.NewContext()
	initialState.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "verification passed",
		"checks":  []any{map[string]any{"name": "go test ./...", "status": "pass"}},
	})
	initialState.Set("euclo.active_plan_version", archaeodomain.VersionedLivingPlan{
		ID:         "plan-version-1",
		WorkflowID: "wf-restore",
		Version:    2,
		Status:     archaeodomain.LivingPlanVersionActive,
		Plan: frameworkplan.LivingPlan{
			ID:         "plan-1",
			WorkflowID: "wf-restore",
			StepOrder:  []string{"step-1"},
			Steps: map[string]*frameworkplan.PlanStep{
				"step-1": {ID: "step-1", Status: frameworkplan.PlanStepInProgress},
			},
		},
		PatternRefs:             []string{"pattern-1"},
		TensionRefs:             []string{"tension-1"},
		FormationProvenanceRefs: []string{"prov-1"},
	})

	firstResult, err := agent.Execute(ctx, &core.Task{
		ID:          "task-restore-seed",
		Instruction: "plan and implement the change",
		Context: map[string]any{
			"workspace":   workspace,
			"workflow_id": "wf-restore",
			"mode":        "planning",
		},
	}, initialState)
	require.NoError(t, err)
	require.NotNil(t, firstResult)

	workflowID := initialState.GetString("euclo.workflow_id")
	runID := initialState.GetString("euclo.run_id")
	require.Equal(t, "wf-restore", workflowID)
	require.NotEmpty(t, runID)

	resumeState := core.NewContext()
	resumeState.Set("euclo.context_compaction", eucloruntime.ContextLifecycleState{
		WorkflowID:         workflowID,
		RunID:              runID,
		Stage:              eucloruntime.ContextLifecycleStageCompacted,
		RestoreRequired:    true,
		CompactionEligible: true,
		CompactionCount:    1,
	})
	resumeResult, err := agent.Execute(ctx, &core.Task{
		ID:          "task-restore-resume",
		Instruction: "status of workflow",
		Context: map[string]any{
			"workspace":   workspace,
			"workflow_id": workflowID,
			"run_id":      runID,
			"mode":        "planning",
		},
	}, resumeState)
	require.NoError(t, err)
	require.NotNil(t, resumeResult)

	rawLifecycle, ok := resumeState.Get("euclo.context_compaction")
	require.True(t, ok)
	lifecycle, ok := rawLifecycle.(eucloruntime.ContextLifecycleState)
	require.True(t, ok)
	require.Equal(t, eucloruntime.ContextLifecycleStageRestored, lifecycle.Stage)
	require.GreaterOrEqual(t, lifecycle.RestoreCount, 1)

	rawUOW, ok := resumeState.Get("euclo.unit_of_work")
	require.True(t, ok)
	uow, ok := rawUOW.(eucloruntime.UnitOfWork)
	require.True(t, ok)
	require.NotNil(t, uow.PlanBinding)
	require.Equal(t, "plan-1", uow.PlanBinding.PlanID)
	require.Equal(t, 2, uow.PlanBinding.PlanVersion)
	require.True(t, uow.ContextBundle.RestoreRequired)
}

func TestAgentExecutePersistsAndRestoresProviderSnapshots(t *testing.T) {
	ctx := context.Background()
	workflowStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer workflowStore.Close()
	runtimeStore, err := db.NewSQLiteRuntimeMemoryStore(filepath.Join(t.TempDir(), "runtime.db"))
	require.NoError(t, err)
	defer runtimeStore.Close()

	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(testutil.FileWriteTool{}))
	store := memory.NewCompositeRuntimeStore(workflowStore, runtimeStore, nil)
	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: registry,
		Memory:   store,
		Config:   &core.Config{Name: "euclo-provider-restore", Model: "stub", MaxIterations: 1},
	})

	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "verification passed",
		"checks":  []any{map[string]any{"name": "go test ./...", "status": "pass"}},
	})
	state.Set("euclo.provider_snapshots", []core.ProviderSnapshot{{
		ProviderID: "provider-1",
		Descriptor: core.ProviderDescriptor{ID: "provider-1", Kind: core.ProviderKindBuiltin},
		CapturedAt: "2026-03-28T00:00:00Z",
	}})
	state.Set("euclo.provider_session_snapshots", []core.ProviderSessionSnapshot{{
		Session:    core.ProviderSession{ID: "session-1", ProviderID: "provider-1"},
		CapturedAt: "2026-03-28T00:00:01Z",
	}})

	_, err = agent.Execute(ctx, &core.Task{
		ID:          "task-provider-restore",
		Instruction: "summarize current status",
		Context:     map[string]any{"workspace": t.TempDir()},
	}, state)
	require.NoError(t, err)

	workflowID := state.GetString("euclo.workflow_id")
	runID := state.GetString("euclo.run_id")
	require.NotEmpty(t, workflowID)
	require.NotEmpty(t, runID)

	providers, err := workflowStore.ListProviderSnapshots(ctx, workflowID, runID)
	require.NoError(t, err)
	require.Len(t, providers, 1)
	sessions, err := workflowStore.ListProviderSessionSnapshots(ctx, workflowID, runID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)

	resumeState := core.NewContext()
	resumeState.Set("euclo.context_compaction", eucloruntime.ContextLifecycleState{
		WorkflowID:      workflowID,
		RunID:           runID,
		Stage:           eucloruntime.ContextLifecycleStageCompacted,
		RestoreRequired: true,
	})
	_, err = agent.Execute(ctx, &core.Task{
		ID:          "task-provider-restore-resume",
		Instruction: "summarize current status",
		Context: map[string]any{
			"workspace":                t.TempDir(),
			"workflow_id":              workflowID,
			"run_id":                   runID,
			"euclo.restore_continuity": true,
		},
	}, resumeState)
	require.NoError(t, err)
	raw, ok := resumeState.Get("euclo.provider_snapshots")
	require.True(t, ok)
	restoredProviders, ok := raw.([]core.ProviderSnapshot)
	require.True(t, ok)
	require.Len(t, restoredProviders, 1)
}

func TestAgentExecuteRestoreFailureClassifiesRun(t *testing.T) {
	ctx := context.Background()
	workflowStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer workflowStore.Close()
	runtimeStore, err := db.NewSQLiteRuntimeMemoryStore(filepath.Join(t.TempDir(), "runtime.db"))
	require.NoError(t, err)
	defer runtimeStore.Close()

	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(testutil.FileWriteTool{}))
	store := memory.NewCompositeRuntimeStore(workflowStore, runtimeStore, nil)

	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: registry,
		Memory:   store,
		Config:   &core.Config{Name: "euclo-restore-fail", Model: "stub", MaxIterations: 1},
	})

	state := core.NewContext()
	result, err := agent.Execute(ctx, &core.Task{
		ID:          "task-restore-fail",
		Instruction: "resume prior workflow",
		Context: map[string]any{
			"workflow_id":              "wf-missing",
			"run_id":                   "run-missing",
			"euclo.restore_continuity": true,
			"euclo.context_compaction": map[string]any{
				"workflow_id":      "wf-missing",
				"run_id":           "run-missing",
				"stage":            "compacted",
				"restore_required": true,
			},
		},
	}, state)
	require.Error(t, err)
	require.NotNil(t, result)
	require.Equal(t, "restore_failed", result.Metadata["result_class"])

	rawLifecycle, ok := state.Get("euclo.context_compaction")
	require.True(t, ok)
	lifecycle, ok := rawLifecycle.(eucloruntime.ContextLifecycleState)
	require.True(t, ok)
	require.Equal(t, eucloruntime.ContextLifecycleStageRestoreFailed, lifecycle.Stage)
}
