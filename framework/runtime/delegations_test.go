package runtime

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/persistence"
	"github.com/stretchr/testify/require"
)

type delegationTestCapability struct {
	desc core.CapabilityDescriptor
	call func(context.Context, *core.Context, map[string]any) (*core.ToolResult, error)
}

type delegationRegistryStub struct {
	policySnapshot *core.PolicySnapshot
	targets        map[string]delegationTestCapability
}

type backgroundRunnerStub struct {
	handle *BackgroundDelegationHandle
	err    error
}

func (r backgroundRunnerStub) StartBackgroundDelegation(context.Context, core.DelegationRequest, core.CapabilityDescriptor, map[string]any, DelegationExecutionOptions) (*BackgroundDelegationHandle, error) {
	return r.handle, r.err
}

func (r *delegationRegistryStub) CapturePolicySnapshot() *core.PolicySnapshot {
	if r.policySnapshot != nil {
		return r.policySnapshot
	}
	return &core.PolicySnapshot{ID: "policy-1"}
}

func (r *delegationRegistryStub) GetCoordinationTarget(idOrName string) (core.CapabilityDescriptor, bool) {
	for _, target := range r.targets {
		if target.desc.ID == idOrName || target.desc.Name == idOrName {
			return target.desc, true
		}
	}
	return core.CapabilityDescriptor{}, false
}

func (r *delegationRegistryStub) CoordinationTargets(selectors ...core.CapabilitySelector) []core.CapabilityDescriptor {
	out := make([]core.CapabilityDescriptor, 0, len(r.targets))
	for _, target := range r.targets {
		matched := true
		for _, selector := range selectors {
			if !core.SelectorMatchesDescriptor(selector, target.desc) {
				matched = false
				break
			}
		}
		if matched {
			out = append(out, target.desc)
		}
	}
	return out
}

func (r *delegationRegistryStub) InvokeCapability(ctx context.Context, state *core.Context, idOrName string, args map[string]interface{}) (*core.ToolResult, error) {
	for _, target := range r.targets {
		if target.desc.ID == idOrName || target.desc.Name == idOrName {
			return target.Invoke(ctx, state, args)
		}
	}
	return nil, context.Canceled
}

func (c delegationTestCapability) Descriptor(context.Context, *core.Context) core.CapabilityDescriptor {
	return c.desc
}

func (c delegationTestCapability) Invoke(ctx context.Context, state *core.Context, args map[string]any) (*core.ToolResult, error) {
	if c.call == nil {
		return &core.ToolResult{Success: true, Data: map[string]any{}}, nil
	}
	return c.call(ctx, state, args)
}

func TestDelegationManagerStartListAndComplete(t *testing.T) {
	manager := NewDelegationManager()

	snapshot, err := manager.StartDelegation(context.Background(), core.DelegationRequest{
		ID:                 "delegation-1",
		WorkflowID:         "workflow-1",
		TaskID:             "task-1",
		TargetCapabilityID: "agent:planner",
		TargetProviderID:   "planner-runtime",
		TaskType:           "plan",
		Instruction:        "Produce a plan",
	}, DelegationStartOptions{
		TrustClass:     core.TrustClassWorkspaceTrusted,
		Recoverability: core.RecoverabilityInProcess,
		Metadata:       map[string]any{"lane": "default"},
	})
	require.NoError(t, err)
	require.Equal(t, core.DelegationStateRunning, snapshot.State)

	listed := manager.ListDelegations(core.DelegationFilter{WorkflowID: "workflow-1"})
	require.Len(t, listed, 1)
	require.Equal(t, "delegation-1", listed[0].Request.ID)

	completed, err := manager.CompleteDelegation("delegation-1", &core.DelegationResult{
		DelegationID: "delegation-1",
		State:        core.DelegationStateSucceeded,
		Success:      true,
		Data:         map[string]any{"summary": "done"},
	})
	require.NoError(t, err)
	require.Equal(t, core.DelegationStateSucceeded, completed.State)
	require.Equal(t, "done", completed.Result.Data["summary"])
}

func TestDelegationManagerCancelUsesHookAndTerminalResult(t *testing.T) {
	manager := NewDelegationManager()
	cancelled := false

	_, err := manager.StartDelegation(context.Background(), core.DelegationRequest{
		ID:                 "delegation-2",
		WorkflowID:         "workflow-1",
		TargetCapabilityID: "agent:background-reviewer",
		TargetProviderID:   "review-runtime",
		TargetSessionID:    "session-1",
		TaskType:           "review",
		Instruction:        "Review in background",
	}, DelegationStartOptions{
		TrustClass: core.TrustClassRemoteApproved,
		Background: true,
		OnCancel: func(context.Context, core.DelegationSnapshot) error {
			cancelled = true
			return nil
		},
	})
	require.NoError(t, err)

	snapshot, err := manager.CancelDelegation(context.Background(), "delegation-2", "operator cancelled")
	require.NoError(t, err)
	require.True(t, cancelled)
	require.Equal(t, core.DelegationStateCancelled, snapshot.State)
	require.NotNil(t, snapshot.Result)
	require.Equal(t, "review-runtime", snapshot.Result.ProviderID)
	require.Equal(t, "session-1", snapshot.Result.SessionID)
	require.Equal(t, []string{"operator cancelled"}, snapshot.Result.Diagnostics)
}

func TestDelegationManagerSnapshotIsCloned(t *testing.T) {
	manager := NewDelegationManager()
	_, err := manager.StartDelegation(context.Background(), core.DelegationRequest{
		ID:                 "delegation-3",
		WorkflowID:         "workflow-2",
		TargetCapabilityID: "agent:architect",
		TaskType:           "design",
		Instruction:        "Design the change",
		Metadata:           map[string]any{"priority": "high"},
	}, DelegationStartOptions{
		Metadata: map[string]any{"lane": "fast"},
	})
	require.NoError(t, err)

	snapshots := manager.SnapshotDelegations()
	require.Len(t, snapshots, 1)
	snapshots[0].Metadata["lane"] = "mutated"
	snapshots[0].Request.Metadata["priority"] = "mutated"

	current, err := manager.GetDelegation("delegation-3")
	require.NoError(t, err)
	require.Equal(t, "fast", current.Metadata["lane"])
	require.Equal(t, "high", current.Request.Metadata["priority"])
}

func TestDelegationManagerPersistDelegationsStoresRecordsAndArtifacts(t *testing.T) {
	manager := NewDelegationManager()
	store, err := persistence.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow_state.db"))
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	require.NoError(t, store.CreateWorkflow(ctx, persistence.WorkflowRecord{
		WorkflowID:  "workflow-1",
		TaskID:      "task-1",
		TaskType:    core.TaskTypeCodeModification,
		Instruction: "persist delegation",
	}))
	require.NoError(t, store.CreateRun(ctx, persistence.WorkflowRunRecord{
		RunID:      "run-1",
		WorkflowID: "workflow-1",
		Status:     persistence.WorkflowRunStatusRunning,
	}))

	_, err = manager.StartDelegation(ctx, core.DelegationRequest{
		ID:                 "delegation-1",
		WorkflowID:         "workflow-1",
		TaskID:             "task-1",
		TargetCapabilityID: "agent:planner",
		TaskType:           "plan",
		Instruction:        "Create a plan",
	}, DelegationStartOptions{
		TrustClass:     core.TrustClassRemoteApproved,
		Recoverability: core.RecoverabilityPersistedRestore,
		Background:     true,
	})
	require.NoError(t, err)

	_, err = manager.CompleteDelegation("delegation-1", &core.DelegationResult{
		DelegationID: "delegation-1",
		State:        core.DelegationStateSucceeded,
		Success:      true,
		Data:         map[string]any{"summary": "ready"},
	})
	require.NoError(t, err)

	require.NoError(t, manager.PersistDelegations(ctx, store, "workflow-1", "run-1"))

	records, err := store.ListDelegations(ctx, "workflow-1", "run-1")
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, core.DelegationStateSucceeded, records[0].State)

	transitions, err := store.ListDelegationTransitions(ctx, "delegation-1")
	require.NoError(t, err)
	require.Len(t, transitions, 1)
	require.Equal(t, core.DelegationStateSucceeded, transitions[0].ToState)

	artifacts, err := store.ListWorkflowArtifacts(ctx, "workflow-1", "run-1")
	require.NoError(t, err)
	require.Len(t, artifacts, 1)
	require.Equal(t, "delegation_result", artifacts[0].Kind)
	require.Contains(t, artifacts[0].InlineRawText, "delegation-1")
}

func TestDelegationManagerExecuteDelegationSelectsTargetAndBuildsWorkflowHandoff(t *testing.T) {
	manager := NewDelegationManager()
	store := newDelegationProjectionStore(t)
	defer store.Close()

	registry := &delegationRegistryStub{targets: map[string]delegationTestCapability{
		"reviewer.review": {
			desc: core.NormalizeCapabilityDescriptor(core.CapabilityDescriptor{
				ID:            "relurpic:reviewer.review",
				Name:          "reviewer.review",
				Kind:          core.CapabilityKindTool,
				RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
				TrustClass:    core.TrustClassBuiltinTrusted,
				Availability:  core.AvailabilitySpec{Available: true},
				Coordination: &core.CoordinationTargetMetadata{
					Target:                 true,
					Role:                   core.CoordinationRoleReviewer,
					TaskTypes:              []string{"review"},
					ExecutionModes:         []core.CoordinationExecutionMode{core.CoordinationExecutionModeSync},
					DirectInsertionAllowed: false,
				},
				InputSchema: &core.Schema{
					Type: "object",
					Properties: map[string]*core.Schema{
						"instruction":         {Type: "string"},
						"artifact_summary":    {Type: "string"},
						"acceptance_criteria": {Type: "array", Items: &core.Schema{Type: "string"}},
						"resource_refs":       {Type: "array", Items: &core.Schema{Type: "string"}},
					},
					Required: []string{"instruction", "artifact_summary"},
				},
				OutputSchema: &core.Schema{
					Type: "object",
					Properties: map[string]*core.Schema{
						"summary": {Type: "string"},
					},
					Required: []string{"summary"},
				},
			}),
			call: func(_ context.Context, _ *core.Context, args map[string]any) (*core.ToolResult, error) {
				return &core.ToolResult{Success: true, Data: map[string]any{
					"summary":             "reviewed handoff",
					"artifact_summary":    args["artifact_summary"],
					"acceptance_criteria": args["acceptance_criteria"],
					"resource_refs":       args["resource_refs"],
				}}, nil
			},
		}}}

	snapshot, err := manager.ExecuteDelegation(context.Background(), core.DelegationRequest{
		ID:          "delegation-exec-1",
		WorkflowID:  "wf-proj",
		TaskID:      "task-proj",
		TaskType:    "review",
		Instruction: "Review the projected workflow",
		Metadata: map[string]any{
			"acceptance_criteria": []string{"must mention workflow artifacts"},
		},
	}, DelegationExecutionOptions{
		Registry:       registry,
		WorkflowStore:  store,
		WorkflowRunID:  "run-proj",
		WorkflowStepID: "step-b",
		AgentSpec: &core.AgentRuntimeSpec{
			Mode:  core.AgentModePrimary,
			Model: core.AgentModelConfig{Name: "stub", Provider: "test"},
			Coordination: core.AgentCoordinationSpec{
				Enabled: true,
				DelegationTargetSelectors: []core.CapabilitySelector{{
					CoordinationRoles:     []core.CoordinationRole{core.CoordinationRoleReviewer},
					CoordinationTaskTypes: []string{"review"},
				}},
				RequireApprovalCrossTrust: true,
			},
		},
		CallerTrust: core.TrustClassWorkspaceTrusted,
	})
	require.NoError(t, err)
	require.Equal(t, core.DelegationStateSucceeded, snapshot.State)
	require.Equal(t, "relurpic:reviewer.review", snapshot.Request.TargetCapabilityID)
	require.Len(t, snapshot.Request.ResourceRefs, 2)
	require.NotNil(t, snapshot.Result)
	require.Contains(t, snapshot.Result.Data["artifact_summary"].(string), "\"workflow_artifacts\"")
	require.Equal(t, core.InsertionActionHITLRequired, snapshot.Result.Insertion.Action)
}

func TestDelegationManagerExecuteDelegationRejectsRemoteWhenNotAllowed(t *testing.T) {
	manager := NewDelegationManager()
	registry := &delegationRegistryStub{targets: map[string]delegationTestCapability{
		"remote.review": {
			desc: core.NormalizeCapabilityDescriptor(core.CapabilityDescriptor{
				ID:            "provider:remote-review",
				Name:          "remote.review",
				Kind:          core.CapabilityKindTool,
				RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
				Source: core.CapabilitySource{
					Scope:      core.CapabilityScopeRemote,
					ProviderID: "remote-mcp",
				},
				TrustClass:   core.TrustClassRemoteDeclared,
				Availability: core.AvailabilitySpec{Available: true},
				Coordination: &core.CoordinationTargetMetadata{
					Target:         true,
					Role:           core.CoordinationRoleReviewer,
					TaskTypes:      []string{"review"},
					ExecutionModes: []core.CoordinationExecutionMode{core.CoordinationExecutionModeSync},
				},
			}),
		}}}

	_, err := manager.ExecuteDelegation(context.Background(), core.DelegationRequest{
		ID:          "delegation-exec-2",
		TaskType:    "review",
		Instruction: "Review remotely",
	}, DelegationExecutionOptions{
		Registry: registry,
		AgentSpec: &core.AgentRuntimeSpec{
			Mode:  core.AgentModePrimary,
			Model: core.AgentModelConfig{Name: "stub", Provider: "test"},
			Coordination: core.AgentCoordinationSpec{
				Enabled: true,
				DelegationTargetSelectors: []core.CapabilitySelector{{
					CoordinationRoles:     []core.CoordinationRole{core.CoordinationRoleReviewer},
					CoordinationTaskTypes: []string{"review"},
				}},
				AllowRemoteDelegation: false,
			},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "remote delegation")
}

func TestDelegationManagerExecuteDelegationStartsBackgroundDelegation(t *testing.T) {
	manager := NewDelegationManager()
	results := make(chan BackgroundDelegationOutcome, 1)
	registry := &delegationRegistryStub{targets: map[string]delegationTestCapability{
		"architect.execute": {
			desc: core.NormalizeCapabilityDescriptor(core.CapabilityDescriptor{
				ID:            "relurpic:architect.execute",
				Name:          "architect.execute",
				Kind:          core.CapabilityKindTool,
				RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
				TrustClass:    core.TrustClassBuiltinTrusted,
				Availability:  core.AvailabilitySpec{Available: true},
				Coordination: &core.CoordinationTargetMetadata{
					Target:         true,
					Role:           core.CoordinationRoleArchitect,
					TaskTypes:      []string{"implement"},
					ExecutionModes: []core.CoordinationExecutionMode{core.CoordinationExecutionModeBackgroundAgent},
				},
			}),
		},
	}}

	snapshot, err := manager.ExecuteDelegation(context.Background(), core.DelegationRequest{
		ID:          "delegation-bg-1",
		TaskType:    "implement",
		Instruction: "Run in background",
		Metadata: map[string]any{
			"background": true,
		},
	}, DelegationExecutionOptions{
		Registry: registry,
		AgentSpec: &core.AgentRuntimeSpec{
			Mode:  core.AgentModePrimary,
			Model: core.AgentModelConfig{Name: "stub", Provider: "test"},
			Coordination: core.AgentCoordinationSpec{
				Enabled:                   true,
				AllowBackgroundDelegation: true,
				DelegationTargetSelectors: []core.CapabilitySelector{{
					CoordinationRoles:     []core.CoordinationRole{core.CoordinationRoleArchitect},
					CoordinationTaskTypes: []string{"implement"},
				}},
			},
		},
		Background: true,
		BackgroundRunner: backgroundRunnerStub{
			handle: &BackgroundDelegationHandle{
				ProviderID:     "delegation-runtime",
				SessionID:      "delegation-runtime:delegation-bg-1",
				Recoverability: core.RecoverabilityInProcess,
				Results:        results,
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, core.DelegationStateRunning, snapshot.State)
	require.True(t, snapshot.Background)
	require.Equal(t, "delegation-runtime", snapshot.Request.TargetProviderID)
	require.Equal(t, "delegation-runtime:delegation-bg-1", snapshot.Request.TargetSessionID)

	results <- BackgroundDelegationOutcome{
		Result: &core.ToolResult{Success: true, Data: map[string]any{"summary": "completed async"}},
	}
	close(results)

	require.Eventually(t, func() bool {
		current, err := manager.GetDelegation("delegation-bg-1")
		return err == nil && current.State == core.DelegationStateSucceeded
	}, time.Second, 10*time.Millisecond)
}

func newDelegationProjectionStore(t *testing.T) *persistence.SQLiteWorkflowStateStore {
	t.Helper()
	store, err := persistence.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow_state.db"))
	require.NoError(t, err)

	ctx := context.Background()
	now := time.Now().UTC()
	finished := now.Add(-time.Minute)

	require.NoError(t, store.CreateWorkflow(ctx, persistence.WorkflowRecord{
		WorkflowID:  "wf-proj",
		TaskID:      "task-proj",
		TaskType:    core.TaskTypeCodeModification,
		Instruction: "Project workflow state",
		Status:      persistence.WorkflowRunStatusRunning,
	}))
	require.NoError(t, store.CreateRun(ctx, persistence.WorkflowRunRecord{
		RunID:      "run-proj",
		WorkflowID: "wf-proj",
		Status:     persistence.WorkflowRunStatusRunning,
		AgentName:  "architect",
		AgentMode:  "primary",
		StartedAt:  finished,
	}))
	require.NoError(t, store.SavePlan(ctx, persistence.WorkflowPlanRecord{
		PlanID:     "plan-proj",
		WorkflowID: "wf-proj",
		RunID:      "run-proj",
		Plan: core.Plan{
			Goal: "Ship projection resources",
			Steps: []core.PlanStep{
				{ID: "step-a", Description: "prepare context"},
				{ID: "step-b", Description: "consume handoff"},
			},
		},
		IsActive: true,
	}))
	require.NoError(t, store.CreateStepRun(ctx, persistence.StepRunRecord{
		StepRunID:      "step-run-a1",
		WorkflowID:     "wf-proj",
		RunID:          "run-proj",
		StepID:         "step-a",
		Attempt:        1,
		Status:         persistence.StepStatusCompleted,
		Summary:        "prepared handoff",
		ResultData:     map[string]any{"summary": "prepared handoff"},
		VerificationOK: true,
		StartedAt:      finished.Add(-time.Minute),
		FinishedAt:     &finished,
	}))
	require.NoError(t, store.UpdateStepStatus(ctx, "wf-proj", "step-a", persistence.StepStatusCompleted, "prepared handoff"))
	require.NoError(t, store.UpsertWorkflowArtifact(ctx, persistence.WorkflowArtifactRecord{
		ArtifactID:      "workflow-artifact-1",
		WorkflowID:      "wf-proj",
		RunID:           "run-proj",
		Kind:            "planner_output",
		ContentType:     "application/json",
		StorageKind:     persistence.ArtifactStorageInline,
		SummaryText:     "planner summary",
		SummaryMetadata: map[string]any{"source": "planner"},
		InlineRawText:   `{"goal":"Ship projection resources"}`,
		RawSizeBytes:    int64(len(`{"goal":"Ship projection resources"}`)),
	}))
	return store
}
