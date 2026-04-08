package nexus

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
	fwfmp "github.com/lexcodex/relurpify/framework/middleware/fmp"
	"github.com/lexcodex/relurpify/named/rex/reconcile"
	"github.com/stretchr/testify/require"
)

func TestRuntimeEndpointExportContextAndFenceAttempt(t *testing.T) {
	t.Parallel()

	endpoint := &RuntimeEndpoint{
		Packager: &stubContextPackager{},
	}
	pkg, err := endpoint.ExportContext(context.Background(), core.LineageRecord{LineageID: "wf-1"}, core.AttemptRecord{AttemptID: "run-1"})
	require.NoError(t, err)
	require.NotNil(t, pkg)
	require.NoError(t, endpoint.FenceAttempt(context.Background(), core.FenceNotice{}))
}

func TestLineageBridgeApplyReconciliationOutcomeUpdatesAttemptAndBinding(t *testing.T) {
	t.Parallel()

	store, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer store.Close()

	ownership := &fwfmp.InMemoryOwnershipStore{}
	ctx := context.Background()
	require.NoError(t, ownership.CreateLineage(ctx, core.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
	}))
	require.NoError(t, ownership.UpsertAttempt(ctx, core.AttemptRecord{
		AttemptID:        "run-1",
		LineageID:        "lineage-1",
		RuntimeID:        "rex",
		State:            core.AttemptStateRunning,
		LastProgressTime: time.Now().UTC(),
	}))
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "task-1",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "resume",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	require.NoError(t, store.CreateRun(ctx, memory.WorkflowRunRecord{
		RunID:      "run-1",
		WorkflowID: "wf-1",
		Status:     memory.WorkflowRunStatusRunning,
		StartedAt:  time.Now().UTC(),
	}))

	bridge := &LineageBridge{
		WorkflowStore: store,
		Service:       &fwfmp.Service{Ownership: ownership},
		Now:           func() time.Time { return time.Date(2026, 4, 8, 15, 0, 0, 0, time.UTC) },
	}
	require.NoError(t, bridge.persistBinding(ctx, "wf-1", "run-1", LineageBinding{
		LineageID: "lineage-1",
		AttemptID: "run-1",
		RuntimeID: "rex",
		State:     string(core.AttemptStateRunning),
		UpdatedAt: time.Now().UTC(),
	}))

	outcome := &reconcile.Record{
		ID:              "run-1",
		WorkflowID:      "wf-1",
		RunID:           "run-1",
		LineageID:       "lineage-1",
		AttemptID:       "run-1",
		Status:          reconcile.StatusVerified,
		RepairSummary:   "fixed",
		ResolutionNotes: "notes",
	}
	require.NoError(t, bridge.ApplyReconciliationOutcome(ctx, "wf-1", "run-1", outcome))

	attempt, ok, err := ownership.GetAttempt(ctx, "run-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, core.AttemptStateCompleted, attempt.State)

	binding, err := bridge.readBinding(ctx, "wf-1", "run-1")
	require.NoError(t, err)
	require.NotNil(t, binding)
	require.Equal(t, string(core.AttemptStateCompleted), binding.State)
}

func TestLineageBridgeEnsureBindingAndHelperBranches(t *testing.T) {
	t.Parallel()

	store, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-2",
		TaskID:      "task-2",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "resume",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	require.NoError(t, store.CreateRun(ctx, memory.WorkflowRunRecord{
		RunID:      "run-2",
		WorkflowID: "wf-2",
		Status:     memory.WorkflowRunStatusRunning,
		StartedAt:  time.Now().UTC(),
	}))
	require.NoError(t, (&LineageBridge{WorkflowStore: store}).persistBinding(ctx, "wf-2", "run-2", LineageBinding{
		LineageID: "lineage-2",
		AttemptID: "run-2",
		RuntimeID: "rex",
		State:     string(core.AttemptStateRunning),
		UpdatedAt: time.Now().UTC(),
	}))

	state := core.NewContext()
	state.Set("fmp.lineage_id", "lineage-2")
	state.Set("gateway.session_id", "sess-2")
	bridge := &LineageBridge{WorkflowStore: store, RuntimeID: "rex", Now: func() time.Time { return time.Date(2026, 4, 8, 16, 0, 0, 0, time.UTC) }}

	binding, err := bridge.ensureBinding(ctx, "wf-2", "run-2", &core.Task{Context: map[string]any{"session_id": "task-sess"}}, state, bridge.nowUTC())
	require.NoError(t, err)
	require.NotNil(t, binding)
	require.Equal(t, "lineage-2", binding.LineageID)
	require.Equal(t, "run-2", binding.AttemptID)
	require.Equal(t, "sess-2", binding.SessionID)

	readBinding, err := bridge.ensureBinding(ctx, "wf-2", "run-2", nil, nil, bridge.nowUTC())
	require.NoError(t, err)
	require.NotNil(t, readBinding)
	require.Equal(t, "lineage-2", readBinding.LineageID)

	require.Equal(t, "sess-2", sessionIDFromState(state, nil))
	require.Equal(t, "task-sess", sessionIDFromState(nil, &core.Task{Context: map[string]any{"session_id": "task-sess"}}))
	require.Equal(t, core.SensitivityClassModerate, sensitivityFromState(nil))
	state.Set("fmp.allowed_federation_targets", []string{"mesh-a", "mesh-b"})
	require.Equal(t, []string{"mesh-a", "mesh-b"}, federationTargetsFromState(state))

	require.Equal(t, string(core.AttemptStateHandoffAccepted), func() string {
		s, ok, err := bridgeStateForFrameworkEvent(core.FrameworkEvent{Type: core.FrameworkEventFMPHandoffAccepted})
		require.NoError(t, err)
		require.True(t, ok)
		return s
	}())
	require.Equal(t, string(core.AttemptStateFenced), func() string {
		s, ok, err := bridgeStateForFrameworkEvent(core.FrameworkEvent{Type: core.FrameworkEventFMPFenceIssued})
		require.NoError(t, err)
		require.True(t, ok)
		return s
	}())
	require.False(t, func() bool {
		_, ok, err := bridgeStateForFrameworkEvent(core.FrameworkEvent{Type: "other"})
		require.NoError(t, err)
		return ok
	}())

	require.Equal(t, string(core.AttemptStateRunning), func() string {
		next, changed := applyBridgeState(LineageBinding{LineageID: "lineage-2", AttemptID: "run-2"}, map[string]any{"new_attempt": "run-2"}, string(core.AttemptStateCommittedRemote))
		require.True(t, changed)
		return next
	}())
	require.Equal(t, string(core.AttemptStateFenced), func() string {
		next, changed := applyBridgeState(LineageBinding{LineageID: "lineage-2", AttemptID: "run-2"}, map[string]any{"attempt_id": "run-2"}, string(core.AttemptStateFenced))
		require.True(t, changed)
		return next
	}())
	require.Equal(t, "fmp lifecycle event", bridgeMessageForEvent("other"))
}

func TestLineageBridgeAfterExecuteAndResolveBindingBranches(t *testing.T) {
	t.Parallel()

	store, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer store.Close()

	ownership := &fwfmp.InMemoryOwnershipStore{}
	ctx := context.Background()
	require.NoError(t, ownership.CreateLineage(ctx, core.LineageRecord{
		LineageID:    "lineage-3",
		TenantID:     "tenant-3",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-3", Kind: core.SubjectKindServiceAccount, ID: "svc-3"},
	}))
	require.NoError(t, ownership.UpsertAttempt(ctx, core.AttemptRecord{
		AttemptID:        "run-3",
		LineageID:        "lineage-3",
		RuntimeID:        "rex",
		State:            core.AttemptStateRunning,
		LastProgressTime: time.Now().UTC(),
	}))
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-3",
		TaskID:      "task-3",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "resume",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	require.NoError(t, store.CreateRun(ctx, memory.WorkflowRunRecord{
		RunID:      "run-3",
		WorkflowID: "wf-3",
		Status:     memory.WorkflowRunStatusRunning,
		StartedAt:  time.Now().UTC(),
	}))

	bridge := &LineageBridge{
		WorkflowStore: store,
		Service:       &fwfmp.Service{Ownership: ownership},
		Now:           func() time.Time { return time.Date(2026, 4, 8, 17, 0, 0, 0, time.UTC) },
	}
	require.NoError(t, bridge.persistBinding(ctx, "wf-3", "run-3", LineageBinding{
		LineageID: "lineage-3",
		AttemptID: "run-3",
		RuntimeID: "rex",
		State:     string(core.AttemptStateRunning),
		UpdatedAt: time.Now().UTC(),
	}))

	require.NoError(t, bridge.AfterExecute(ctx, "wf-3", "run-3", &core.Task{Instruction: "resume"}, core.NewContext(), &core.Result{Success: true}, nil))
	attempt, ok, err := ownership.GetAttempt(ctx, "run-3")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, core.AttemptStateCompleted, attempt.State)

	require.NoError(t, bridge.AfterExecute(ctx, "wf-3", "run-3", &core.Task{Instruction: "resume"}, core.NewContext(), &core.Result{Success: false}, context.Canceled))
	attempt, ok, err = ownership.GetAttempt(ctx, "run-3")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, core.AttemptStateFailed, attempt.State)

	readonlyStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow2.db"))
	require.NoError(t, err)
	defer readonlyStore.Close()
	require.NoError(t, readonlyStore.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-4",
		TaskID:      "task-4",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "resume",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	require.NoError(t, readonlyStore.CreateRun(ctx, memory.WorkflowRunRecord{
		RunID:      "run-4",
		WorkflowID: "wf-4",
		Status:     memory.WorkflowRunStatusRunning,
		StartedAt:  time.Now().UTC(),
	}))
	require.NoError(t, (&LineageBridge{WorkflowStore: readonlyStore}).persistBinding(ctx, "wf-4", "run-4", LineageBinding{
		LineageID: "lineage-4",
		AttemptID: "run-4",
		RuntimeID: "rex",
		State:     string(core.AttemptStateRunning),
		UpdatedAt: time.Now().UTC(),
	}))
	binding, err := (&LineageBridge{WorkflowStore: readonlyStore, Service: &fwfmp.Service{Ownership: &fwfmp.InMemoryOwnershipStore{}}}).ResolveReconciliationBinding(ctx, "wf-4", "run-4")
	require.NoError(t, err)
	require.NotNil(t, binding)
	require.Equal(t, "lineage-4", binding.LineageID)
}

func TestRuntimeEndpointAndSnapshotStoreHelperBranches(t *testing.T) {
	t.Parallel()

	store, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-5",
		TaskID:      "task-5",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "review",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	require.NoError(t, store.CreateRun(ctx, memory.WorkflowRunRecord{
		RunID:      "run-5",
		WorkflowID: "wf-5",
		Status:     memory.WorkflowRunStatusRunning,
		StartedAt:  time.Now().UTC(),
	}))
	require.NoError(t, store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:    "run-5:action",
		WorkflowID:    "wf-5",
		RunID:         "run-5",
		Kind:          "rex.action_log",
		ContentType:   "application/json",
		StorageKind:   memory.ArtifactStorageInline,
		InlineRawText: `{"action":"resume"}`,
		CreatedAt:     time.Now().UTC(),
	}))
	require.NoError(t, store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:    "run-5:completion",
		WorkflowID:    "wf-5",
		RunID:         "run-5",
		Kind:          "rex.completion",
		ContentType:   "application/json",
		StorageKind:   memory.ArtifactStorageInline,
		InlineRawText: `{"result":"ok"}`,
		CreatedAt:     time.Now().UTC(),
	}))
	require.NoError(t, store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:    "run-5:verification",
		WorkflowID:    "wf-5",
		RunID:         "run-5",
		Kind:          "rex.verification",
		ContentType:   "application/json",
		StorageKind:   memory.ArtifactStorageInline,
		InlineRawText: `{"verified":true}`,
		CreatedAt:     time.Now().UTC(),
	}))
	require.NoError(t, store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:    "run-5:lineage",
		WorkflowID:    "wf-5",
		RunID:         "run-5",
		Kind:          "rex.fmp_lineage",
		ContentType:   "application/json",
		StorageKind:   memory.ArtifactStorageInline,
		InlineRawText: `{"lineage_id":"lineage-5","attempt_id":"run-5"}`,
		CreatedAt:     time.Now().UTC(),
	}))

	// Exercise the workflow import helper on an already-present workflow/run.
	endpoint := &RuntimeEndpoint{WorkflowStore: store, Now: func() time.Time { return time.Date(2026, 4, 8, 18, 0, 0, 0, time.UTC) }}
	require.NoError(t, endpoint.ensureImportedWorkflow(ctx, "wf-5", "run-5", &core.Task{ID: "task-5", Type: core.TaskTypeCodeGeneration, Instruction: "review"}))
	require.NoError(t, endpoint.persistImport(ctx, "wf-5", "run-5", core.HandoffAccept{OfferID: "offer-5", ProvisionalAttemptID: "run-5", AcceptedContextClass: "workflow-runtime"}, &fwfmp.PortableContextPackage{Manifest: core.ContextManifest{ContextID: "ctx-5"}}))

	// Snapshot store should summarize the additional rex artifact kinds and
	// still be able to recover the fallback state when no explicit state block is present.
	payload, err := (SnapshotStore{WorkflowStore: store}).QueryWorkflowRuntime(ctx, "wf-5", "run-5")
	require.NoError(t, err)
	require.Equal(t, "ok", payload["completion"].(map[string]any)["result"])
	require.Equal(t, true, payload["verification_evidence"].(map[string]any)["verified"])
	require.Equal(t, "lineage-5", payload["lineage_binding"].(map[string]any)["lineage_id"])
	require.Equal(t, "wf-5", payload["state"].(map[string]any)["workflow_id"])

	_, err = (SnapshotStore{}).QueryWorkflowRuntime(ctx, "wf-5", "run-5")
	require.Error(t, err)

	// Also ensure decodeArtifactJSON rejects invalid JSON without failing the query.
	require.Nil(t, decodeArtifactJSON(memory.WorkflowArtifactRecord{InlineRawText: "{invalid"}))
	raw, err := json.Marshal(map[string]any{"value": 1})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"value": float64(1)}, decodeArtifactJSON(memory.WorkflowArtifactRecord{InlineRawText: string(raw)}))
}

func TestAdapterAndRuntimeEndpointNilBranches(t *testing.T) {
	t.Parallel()

	adapter := &Adapter{name: "rex"}
	require.Equal(t, Registration{}, adapter.Registration())
	_, err := adapter.Invoke(context.Background(), &core.Task{}, core.NewContext())
	require.Error(t, err)
	_, err = adapter.AdminSnapshot(context.Background())
	require.Error(t, err)

	var nilEndpoint *RuntimeEndpoint
	_, err = nilEndpoint.ExportContext(context.Background(), core.LineageRecord{}, core.AttemptRecord{})
	require.Error(t, err)
}

func TestRuntimeEndpointCreateAttemptMissingWorkflowAndNilStore(t *testing.T) {
	t.Parallel()

	workflowStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer workflowStore.Close()

	packager := &stubContextPackager{}
	endpoint := &RuntimeEndpoint{
		DescriptorValue: core.RuntimeDescriptor{RuntimeID: "rex"},
		Packager:        packager,
		WorkflowStore:   workflowStore,
		Now:             func() time.Time { return time.Date(2026, 4, 8, 19, 0, 0, 0, time.UTC) },
	}
	payload, err := json.Marshal(map[string]any{
		"task": map[string]any{
			"id":          "task-6",
			"instruction": "imported",
			"context":     map[string]any{"workflow_id": "wf-6"},
		},
		"state": map[string]any{"session_id": "sess-6"},
	})
	require.NoError(t, err)

	attempt, err := endpoint.CreateAttempt(context.Background(), core.LineageRecord{LineageID: "wf-6"}, core.HandoffAccept{
		OfferID:              "offer-6",
		ProvisionalAttemptID: "run-6",
		AcceptedContextClass: "workflow-runtime",
	}, &fwfmp.PortableContextPackage{ExecutionPayload: payload})
	require.NoError(t, err)
	require.Equal(t, "run-6", attempt.AttemptID)
	require.True(t, packager.unsealCalled == false)

	createdWorkflow, ok, err := workflowStore.GetWorkflow(context.Background(), "wf-6")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "task-6", createdWorkflow.TaskID)
	createdRun, ok, err := workflowStore.GetRun(context.Background(), "run-6")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "wf-6", createdRun.WorkflowID)

	nilStoreEndpoint := &RuntimeEndpoint{DescriptorValue: core.RuntimeDescriptor{RuntimeID: "rex"}, Packager: packager}
	task, state, workflowID, runID, err := nilStoreEndpoint.rehydrateTask(&fwfmp.PortableContextPackage{ExecutionPayload: payload}, core.LineageRecord{LineageID: "wf-6"}, core.HandoffAccept{ProvisionalAttemptID: "run-6"})
	require.NoError(t, err)
	require.NotNil(t, task)
	require.NotNil(t, state)
	require.Equal(t, "wf-6", workflowID)
	require.Equal(t, "run-6", runID)
}

func TestRuntimeEndpointCreateAttemptWithoutWorkflowStore(t *testing.T) {
	t.Parallel()

	endpoint := &RuntimeEndpoint{
		DescriptorValue: core.RuntimeDescriptor{RuntimeID: "rex"},
		Now:             func() time.Time { return time.Date(2026, 4, 8, 20, 0, 0, 0, time.UTC) },
	}
	payload, err := json.Marshal(map[string]any{
		"task": map[string]any{
			"id":          "task-7",
			"instruction": "run without persistence",
			"context":     map[string]any{"workflow_id": "wf-7"},
		},
	})
	require.NoError(t, err)

	attempt, err := endpoint.CreateAttempt(context.Background(), core.LineageRecord{LineageID: "wf-7"}, core.HandoffAccept{
		OfferID:              "offer-7",
		ProvisionalAttemptID: "run-7",
		AcceptedContextClass: "workflow-runtime",
	}, &fwfmp.PortableContextPackage{ExecutionPayload: payload})
	require.NoError(t, err)
	require.Equal(t, "run-7", attempt.AttemptID)
	require.Equal(t, core.CapabilityEnvelope{}, endpoint.projectionForAttempt("missing"))
	require.Equal(t, core.CapabilityEnvelope{}, endpoint.projectionForAttempt(""))
	require.Equal(t, time.Date(2026, 4, 8, 20, 0, 0, 0, time.UTC), endpoint.nowUTC())
}
