package framework

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	"codeburg.org/lexbit/relurpify/governance/authorization"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/platform/fs"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// TestPermissionToAuditToTelemetryFlow validates the complete flow from
// permission enforcement through audit logging to telemetry emission.
// This test consolidates the permission->audit and audit->telemetry flows
// that were previously tested separately.
func TestPermissionToAuditToTelemetryFlow(t *testing.T) {
	env := NewTestEnvironment(t)

	// Step 1: Create permission set
	perms := policy.NewFileSystemPermissionSet(env.WorkspacePath, permissions.FileSystemRead, permissions.FileSystemList)

	// Step 2: Create permission manager with audit sink
	manager, err := authorization.NewPermissionManager(env.WorkspacePath, perms, env.AuditSink, nil)
	if err != nil {
		t.Fatalf("permission manager creation failed: %v", err)
	}

	// Step 3: Create a test file
	testFile := filepath.Join(env.WorkspacePath, "test.txt")
	if err := fs.WriteFileSecure(testFile, []byte("test content")); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Step 4: Check file permission (transition point: permission enforcement)
	ctx := context.Background()
	agentID := "test-agent"
	err = manager.CheckFileAccess(ctx, agentID, permissions.FileSystemRead, testFile)
	if err != nil {
		t.Fatalf("file access check failed: %v", err)
	}

	// Step 5: Verify audit record was created (transition point: audit capture)
	records := env.AuditSink.Records()
	if len(records) == 0 {
		t.Error("expected audit records to be created")
	}

	foundPermissionRecord := false
	for _, record := range records {
		if record.Type == string(permissions.PermissionTypeFilesystem) {
			foundPermissionRecord = true
			if record.Result != "granted" {
				t.Errorf("expected permission to be granted, got %s", record.Result)
			}
			break
		}
	}
	if !foundPermissionRecord {
		t.Error("expected to find permission audit record")
	}

	// Step 6: Emit telemetry event (transition point: telemetry emission)
	env.TelemetrySink.Emit(telemetry.Event{
		Type:      telemetry.EventNodeFinish,
		NodeID:    "test-node",
		TaskID:    "test-task",
		Message:   "permission decision completed",
		Timestamp: time.Now().UTC(),
		Metadata: map[string]any{
			"permission_type": "filesystem",
			"decision":        "granted",
		},
	})

	// Step 7: Verify telemetry event was captured (transition point: telemetry capture)
	events := env.TelemetrySink.Events()
	if len(events) == 0 {
		t.Fatal("expected telemetry events to be captured")
	}

	foundTelemetryEvent := false
	for _, event := range events {
		if event.Type == telemetry.EventNodeFinish {
			foundTelemetryEvent = true
			if event.NodeID != "test-node" {
				t.Errorf("expected node ID 'test-node', got %s", event.NodeID)
			}
			if event.TaskID != "test-task" {
				t.Errorf("expected task ID 'test-task', got %s", event.TaskID)
			}
			break
		}
	}
	if !foundTelemetryEvent {
		t.Fatal("expected to find telemetry event")
	}

	// Step 8: Validate the complete flow
	if len(records) == 0 || len(events) == 0 {
		t.Error("expected both audit records and telemetry events to exist")
	}
}

// TestFullFrameworkFlowScenario validates a complete end-to-end flow through
// multiple framework seams: permissions, knowledge, envelope, audit, and telemetry.
func TestFullFrameworkFlowScenario(t *testing.T) {
	env := NewTestEnvironment(t)

	// Step 1: Create permission manager (permission seam)
	perms := policy.NewFileSystemPermissionSet(env.WorkspacePath, permissions.FileSystemRead, permissions.FileSystemList)
	manager, err := authorization.NewPermissionManager(env.WorkspacePath, perms, env.AuditSink, nil)
	if err != nil {
		t.Fatalf("permission manager creation failed: %v", err)
	}

	// Step 2: Check file permission directly (permission enforcement seam)
	testFile := filepath.Join(env.WorkspacePath, "config.yaml")
	configContent := "version: 1.0\nname: test\n"
	if err := fs.WriteFileSecure(testFile, []byte(configContent)); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	ctx := context.Background()
	err = manager.CheckFileAccess(ctx, "full-flow-agent", permissions.FileSystemRead, testFile)
	if err != nil {
		t.Fatalf("file access check failed: %v", err)
	}

	// Step 3: Create knowledge store and ingest result (knowledge seam)
	opts := graphdb.DefaultOptions(env.WorkspacePath)
	graph, err := graphdb.Open(context.Background(), opts)
	if err != nil {
		t.Fatalf("failed to open graph engine: %v", err)
	}
	defer graph.Close(context.Background())

	store := &knowledge.ChunkStore{Graph: graph}
	events := &knowledge.EventBus{}
	ingester := knowledge.NewOutputIngester(store, events)

	envelope := contextdata.NewEnvelope("full-flow-task", "full-flow-session")
	ctx = contextdata.WithEnvelope(ctx, envelope)

	llmResponse := &model.LLMResponse{
		Text: "Configuration loaded successfully",
	}

	savedChunk, err := ingester.IngestLLMResponseFull(ctx, llmResponse)
	if err != nil {
		t.Fatalf("ingestion failed: %v", err)
	}

	// Transition assertion: chunk ingested
	if savedChunk == nil || savedChunk.ID == "" {
		t.Fatal("expected valid chunk from ingestion")
	}

	// Step 4: Add chunk to envelope (envelope seam)
	envelope.AddStreamedContextReference(contextdata.ChunkReference{
		ChunkID: contextdata.ChunkID(savedChunk.ID),
		Source:  "full-flow",
		Rank:    0,
	})

	// Transition assertion: envelope updated
	if len(envelope.References.StreamedContext) != 1 {
		t.Errorf("expected 1 streamed reference, got %d", len(envelope.References.StreamedContext))
	}

	// Step 5: Verify audit records (audit seam)
	records := env.AuditSink.Records()
	if len(records) == 0 {
		t.Error("expected audit records")
	}

	// Transition assertion: audit captured
	foundFilePermission := false
	for _, record := range records {
		if record.Type == string(permissions.PermissionTypeFilesystem) && record.Result == "granted" {
			foundFilePermission = true
			break
		}
	}
	if !foundFilePermission {
		t.Error("expected to find file permission audit record")
	}

	// Step 6: Emit and verify telemetry (telemetry seam)
	env.TelemetrySink.Emit(telemetry.Event{
		Type:      telemetry.EventGraphFinish,
		NodeID:    "full-flow-node",
		TaskID:    envelope.TaskID,
		Message:   "full framework flow completed",
		Timestamp: time.Now().UTC(),
		Metadata: map[string]any{
			"chunk_id": savedChunk.ID,
			"seams":    []string{"permission", "registry", "knowledge", "envelope", "audit", "telemetry"},
		},
	})

	telemetryEvents := env.TelemetrySink.Events()
	if len(telemetryEvents) == 0 {
		t.Error("expected telemetry events")
	}

	// Transition assertion: telemetry captured
	foundGraphFinish := false
	for _, event := range telemetryEvents {
		if event.Type == telemetry.EventGraphFinish {
			foundGraphFinish = true
			if event.TaskID != envelope.TaskID {
				t.Errorf("expected task ID %s, got %s", envelope.TaskID, event.TaskID)
			}
			break
		}
	}
	if !foundGraphFinish {
		t.Error("expected to find graph finish telemetry event")
	}

	// Step 7: Validate the complete flow
	if len(records) == 0 || len(telemetryEvents) == 0 || len(envelope.References.StreamedContext) == 0 {
		t.Error("expected all seams to be active: audit, telemetry, envelope")
	}

	// Verify the chunk can be retrieved (knowledge persistence validation)
	loadedChunk, ok, err := store.Load(savedChunk.ID)
	if err != nil || !ok {
		t.Errorf("failed to retrieve chunk from storage: %v", err)
	}
	if loadedChunk.ID != savedChunk.ID {
		t.Errorf("chunk ID mismatch: expected %s, got %s", savedChunk.ID, loadedChunk.ID)
	}
}
