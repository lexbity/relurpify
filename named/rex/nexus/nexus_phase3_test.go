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
	rexconfig "github.com/lexcodex/relurpify/named/rex/config"
	"github.com/lexcodex/relurpify/named/rex/proof"
	"github.com/lexcodex/relurpify/named/rex/runtime"
	"github.com/stretchr/testify/require"
)

type stubContextPackager struct {
	unsealCalled bool
}

func (s *stubContextPackager) BuildPackage(context.Context, core.LineageRecord, core.AttemptRecord, fwfmp.RuntimeQuery) (*fwfmp.PortableContextPackage, error) {
	return &fwfmp.PortableContextPackage{}, nil
}

func (s *stubContextPackager) SealPackage(context.Context, core.ContextManifest, *fwfmp.PortableContextPackage, []string) (*core.SealedContext, error) {
	return &core.SealedContext{}, nil
}

func (s *stubContextPackager) UnsealPackage(_ context.Context, _ core.SealedContext, pkg *fwfmp.PortableContextPackage) error {
	s.unsealCalled = true
	if pkg != nil {
		pkg.ExecutionPayload = []byte(`{"task":{"instruction":"imported"}}`)
	}
	return nil
}

func TestRuntimeEndpointHelpersAndAttemptFlow(t *testing.T) {
	workflowStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer workflowStore.Close()

	packager := &stubContextPackager{}
	endpoint := &RuntimeEndpoint{
		DescriptorValue:     core.RuntimeDescriptor{RuntimeID: "rex"},
		Packager:            packager,
		WorkflowStore:       workflowStore,
		LineageBindingStore: workflowStore,
		Now:                 func() time.Time { return time.Date(2026, 4, 8, 10, 0, 0, 0, time.UTC) },
	}

	desc, err := endpoint.Descriptor(context.Background())
	require.NoError(t, err)
	require.Equal(t, "rex", desc.RuntimeID)

	require.Error(t, endpoint.ValidateContext(context.Background(), core.ContextManifest{}, core.SealedContext{}))
	require.Error(t, endpoint.ValidateContext(context.Background(), core.ContextManifest{SchemaVersion: "v1"}, core.SealedContext{}))
	require.NoError(t, endpoint.ValidateContext(context.Background(), core.ContextManifest{SchemaVersion: "v1"}, core.SealedContext{CipherSuite: "aes-gcm"}))

	_, err = (&RuntimeEndpoint{}).ImportContext(context.Background(), core.LineageRecord{}, core.ContextManifest{}, core.SealedContext{})
	require.Error(t, err)

	pkg, err := endpoint.ImportContext(context.Background(), core.LineageRecord{}, core.ContextManifest{SchemaVersion: "v1"}, core.SealedContext{CipherSuite: "aes-gcm"})
	require.NoError(t, err)
	require.True(t, packager.unsealCalled)
	require.NotEmpty(t, pkg.ExecutionPayload)

	_, _, _, _, err = endpoint.rehydrateTask(&fwfmp.PortableContextPackage{}, core.LineageRecord{LineageID: "wf-1"}, core.HandoffAccept{ProvisionalAttemptID: "attempt-1"})
	require.Error(t, err)

	_, _, _, _, err = endpoint.rehydrateTask(&fwfmp.PortableContextPackage{ExecutionPayload: []byte(`not json`)}, core.LineageRecord{LineageID: "wf-1"}, core.HandoffAccept{ProvisionalAttemptID: "attempt-1"})
	require.Error(t, err)

	taskPayload, err := json.Marshal(map[string]any{
		"task": map[string]any{
			"id":          "task-1",
			"type":        string(core.TaskTypeReview),
			"instruction": "resume work",
			"context":     map[string]any{"workflow_id": "wf-1", "session_id": "sess-1"},
			"metadata":    map[string]any{"owner": "rex"},
		},
		"state": map[string]any{"custom": "value"},
	})
	require.NoError(t, err)
	task, state, workflowID, runID, err := endpoint.rehydrateTask(&fwfmp.PortableContextPackage{ExecutionPayload: taskPayload}, core.LineageRecord{LineageID: "wf-1"}, core.HandoffAccept{ProvisionalAttemptID: "attempt-2"})
	require.NoError(t, err)
	require.Equal(t, "resume work", task.Instruction)
	require.Equal(t, "wf-1", workflowID)
	require.Equal(t, "attempt-2", runID)
	require.Equal(t, "attempt-2", state.GetString("run_id"))
	require.Equal(t, "wf-1", state.GetString("workflow_id"))

	require.NoError(t, endpoint.ensureImportedWorkflow(context.Background(), "wf-1", "attempt-2", task))
	require.NoError(t, endpoint.persistImport(context.Background(), "wf-1", "attempt-2", "lineage-1", task, state, core.HandoffAccept{OfferID: "offer-1", ProvisionalAttemptID: "attempt-2", AcceptedContextClass: "workflow-runtime"}))

	binding, ok, err := workflowStore.GetLineageBinding(context.Background(), "wf-1", "attempt-2")
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, binding)
	require.Equal(t, "lineage-1", binding.LineageID)
	require.Equal(t, "attempt-2", binding.AttemptID)

	endpoint.rememberProjection("attempt-2", core.CapabilityEnvelope{AllowedCapabilityIDs: []string{string(core.CapabilityExecute)}})
	require.NotEmpty(t, endpoint.projectionForAttempt("attempt-2").AllowedCapabilityIDs)
	require.Equal(t, time.Date(2026, 4, 8, 10, 0, 0, 0, time.UTC), endpoint.nowUTC())

	att, err := endpoint.CreateAttempt(context.Background(), core.LineageRecord{LineageID: "lineage-1"}, core.HandoffAccept{
		OfferID:              "offer-1",
		ProvisionalAttemptID: "attempt-3",
		AcceptedContextClass: "workflow-runtime",
	}, &fwfmp.PortableContextPackage{ExecutionPayload: taskPayload})
	require.NoError(t, err)
	require.Equal(t, "attempt-3", att.AttemptID)
	receipt, err := endpoint.IssueReceipt(context.Background(), core.LineageRecord{LineageID: "lineage-1"}, *att, nil)
	require.NoError(t, err)
	require.Equal(t, "attempt-3:receipt", receipt.ReceiptID)
}

func TestLineageBridgeHelpersAndLifecycle(t *testing.T) {
	store, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer store.Close()

	ownership := &fwfmp.InMemoryOwnershipStore{}
	require.NoError(t, ownership.CreateLineage(context.Background(), core.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
	}))
	require.NoError(t, ownership.UpsertAttempt(context.Background(), core.AttemptRecord{
		AttemptID:        "attempt-1",
		LineageID:        "lineage-1",
		RuntimeID:        "rex",
		State:            core.AttemptStateRunning,
		FencingEpoch:     4,
		LastProgressTime: time.Now().UTC(),
	}))
	require.NoError(t, store.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "task-1",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "resume",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	require.NoError(t, store.CreateRun(context.Background(), memory.WorkflowRunRecord{
		RunID:      "run-1",
		WorkflowID: "wf-1",
		Status:     memory.WorkflowRunStatusRunning,
		StartedAt:  time.Now().UTC(),
	}))

	bridge := &LineageBridge{
		WorkflowStore: store,
		Service:       &fwfmp.Service{Ownership: ownership},
		Now:           func() time.Time { return time.Date(2026, 4, 8, 11, 0, 0, 0, time.UTC) },
	}

	state := core.NewContext()
	state.Set("gateway.session_id", "sess-1")
	state.Set("fmp.sensitivity_class", "restricted")
	state.Set("fmp.allowed_federation_targets", []string{"mesh-a", "mesh-b"})
	state.Set("fmp.lineage_id", "lineage-1")
	state.Set("fmp.attempt_id", "attempt-1")

	require.Equal(t, "rex", bridge.runtimeID())
	require.Equal(t, time.Date(2026, 4, 8, 11, 0, 0, 0, time.UTC), bridge.nowUTC())
	require.Equal(t, "sess-1", sessionIDFromState(state, &core.Task{Context: map[string]any{"session_id": "fallback"}}))
	require.NotEmpty(t, defaultCapabilityEnvelope().AllowedCapabilityIDs)

	require.Equal(t, string(core.AttemptStateCommittedRemote), func() string {
		s, ok, err := bridgeStateForFrameworkEvent(core.FrameworkEvent{Type: core.FrameworkEventFMPResumeCommitted})
		require.NoError(t, err)
		require.True(t, ok)
		return s
	}())
	require.Equal(t, "", func() string {
		s, ok, err := bridgeStateForFrameworkEvent(core.FrameworkEvent{Type: "unknown"})
		require.NoError(t, err)
		require.False(t, ok)
		return s
	}())

	payload, err := decodeFrameworkPayload(core.FrameworkEvent{Payload: []byte(`{"lineage_id":"lineage-1"}`)})
	require.NoError(t, err)
	require.Equal(t, "lineage-1", payload["lineage_id"])
	_, err = decodeFrameworkPayload(core.FrameworkEvent{Payload: []byte(`{`)})
	require.Error(t, err)

	require.True(t, matchesFrameworkBinding(LineageBinding{LineageID: "lineage-1"}, map[string]any{"lineage_id": "lineage-1"}))
	require.False(t, matchesFrameworkBinding(LineageBinding{LineageID: "lineage-1"}, map[string]any{"attempt_id": "attempt-9"}))
	next, changed := applyBridgeState(LineageBinding{LineageID: "lineage-1", AttemptID: "attempt-1"}, map[string]any{"old_attempt": "attempt-1"}, string(core.AttemptStateCommittedRemote))
	require.True(t, changed)
	require.Equal(t, string(core.AttemptStateCommittedRemote), next)
	require.Equal(t, "fmp resume committed", bridgeMessageForEvent(core.FrameworkEventFMPResumeCommitted))
	require.Equal(t, "value", stringValue("value"))
	require.Equal(t, "first", firstNonEmpty(" ", "first", "second"))

	require.NoError(t, bridge.BeforeExecute(context.Background(), "wf-1", "run-1", &core.Task{Instruction: "resume"}, state))
	require.NoError(t, bridge.AfterExecute(context.Background(), "wf-1", "run-1", &core.Task{Instruction: "resume"}, state, &core.Result{Success: true}, nil))
	require.NoError(t, bridge.AfterExecute(context.Background(), "wf-1", "run-1", &core.Task{Instruction: "resume"}, state, &core.Result{Success: false}, context.Canceled))

	binding, err := bridge.ResolveReconciliationBinding(context.Background(), "wf-1", "run-1")
	require.NoError(t, err)
	require.NotNil(t, binding)
	require.Equal(t, int64(4), binding.FencingEpoch)

	require.NoError(t, bridge.HandleFrameworkEvent(context.Background(), core.FrameworkEvent{
		Seq:       1,
		Timestamp: time.Date(2026, 4, 8, 11, 5, 0, 0, time.UTC),
		Type:      core.FrameworkEventFMPResumeCommitted,
		Payload:   []byte(`{"lineage_id":"lineage-1","old_attempt":"attempt-1","new_attempt":"attempt-2"}`),
	}))
}

func TestNexusAdapterAndSnapshotHelpers(t *testing.T) {
	managed := &stubManagedRuntime{
		projection: Projection{
			Health:     runtime.HealthHealthy,
			WorkflowID: "wf-1",
			RunID:      "run-1",
		},
		capabilities: []core.Capability{core.CapabilityPlan},
	}
	store, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer store.Close()
	ctx := context.Background()
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{WorkflowID: "wf-1", TaskID: "task-1", TaskType: core.TaskTypeCodeGeneration, Instruction: "run", Status: memory.WorkflowRunStatusRunning}))
	require.NoError(t, store.CreateRun(ctx, memory.WorkflowRunRecord{RunID: "run-1", WorkflowID: "wf-1", Status: memory.WorkflowRunStatusRunning, StartedAt: time.Now().UTC()}))
	require.NoError(t, store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{ArtifactID: "a-1", WorkflowID: "wf-1", RunID: "run-1", Kind: "rex.action_log", ContentType: "application/json", StorageKind: memory.ArtifactStorageInline, InlineRawText: `{"ok":true}`, CreatedAt: time.Now().UTC()}))
	require.NoError(t, store.AppendEvent(ctx, memory.WorkflowEventRecord{EventID: "e-1", WorkflowID: "wf-1", RunID: "run-1", EventType: "step_completed", Message: "step completed", StepID: "step-1", CreatedAt: time.Now().UTC()}))

	adapter := NewAdapter(" ", managed, store)
	require.Equal(t, "rex", adapter.name)
	registration := adapter.Registration()
	require.True(t, registration.Managed)
	require.Equal(t, "nexus-managed", registration.RuntimeType)
	require.NoError(t, func() error {
		_, err := adapter.Invoke(ctx, &core.Task{Instruction: "run"}, core.NewContext())
		return err
	}())

	snapshot, err := adapter.AdminSnapshot(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, snapshot.WorkflowRefURI)
	require.Equal(t, "wf-1", snapshot.HotState["workflow_id"])

	projection := BuildProjection(runtime.New(rexconfig.Default(), nil), proof.ProofSurface{RouteFamily: "react"})
	require.False(t, projection.FailoverReady)
	require.Equal(t, "healthy", projection.RecoveryState)

	require.Nil(t, firstStructuredContent(nil))
	require.Nil(t, firstStructuredContent(&core.ResourceReadResult{}))
	require.Equal(t, map[string]any{"hello": "world"}, firstStructuredContent(&core.ResourceReadResult{Contents: []core.ContentBlock{core.StructuredContentBlock{Data: map[string]any{"hello": "world"}}}}))
}

func TestSnapshotStoreHelperFunctions(t *testing.T) {
	records := []memory.WorkflowArtifactRecord{
		{ArtifactID: "b", Kind: "k", InlineRawText: `{"x":2}`},
		{ArtifactID: "a", Kind: "k", InlineRawText: `not-json`},
	}
	summaries := summarizeArtifacts(records)
	require.Len(t, summaries, 2)
	require.Equal(t, "a", summaries[0]["artifact_id"])
	require.Nil(t, decodeArtifactJSON(records[1]))

	events := []memory.WorkflowEventRecord{
		{StepID: "step-1", EventType: "step_completed", Message: "completed"},
		{StepID: "step-1", EventType: "step_completed", Message: "completed"},
		{StepID: "step-2", EventType: "note", Message: "completed"},
	}
	require.Equal(t, []string{"step-1", "step-2"}, completedStepIDs(events))
}
