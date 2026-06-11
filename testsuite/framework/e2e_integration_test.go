package framework

import (
	"context"
	"testing"
	"time"

	regpkg "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	"codeburg.org/lexbit/relurpify/governance/authorization"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// TestEndToEndAgentExecution validates that all seams work together
// in a realistic agent execution scenario, including manifest, permissions,
// capability registry, tool execution, telemetry, audit, and knowledge storage.
func TestEndToEndAgentExecution(t *testing.T) {
	env := NewTestEnvironment(t)

	// Set up graph engine for knowledge storage
	opts := graphdb.DefaultOptions(env.WorkspacePath)
	graph, err := graphdb.Open(opts)
	if err != nil {
		t.Fatalf("failed to open graph engine: %v", err)
	}
	defer graph.Close()
	knowledgeStore := &knowledge.ChunkStore{Graph: graph}

	// Step 1: Create a manifest with policy (manifest seam)
	m := ValidManifest().
		WithFileSystemPermission(permissions.FileSystemRead, env.WorkspacePath+"/**").
		Build()

	if err := m.Validate(); err != nil {
		t.Fatalf("manifest validation failed: %v", err)
	}

	// Step 2: Create permission manager from manifest policy (permission seam)
	manager, err := authorization.NewPermissionManager(env.WorkspacePath, &m.Policy.Permissions, env.AuditSink, nil)
	if err != nil {
		t.Fatalf("failed to create permission manager: %v", err)
	}

	// Step 3: Create capability registry with permission manager (capability seam)
	registry := regpkg.NewRegistry()
	registry.UsePermissionManager("test-agent", manager)

	// Register a test tool that requires permission
	tool := &permissionedTestTool{
		name:        "e2e-tool",
		description: "end-to-end test tool",
		category:    "test",
		permissions: policy.NewFileSystemPermissionSet(env.WorkspacePath, permissions.FileSystemRead),
		manager:     manager,
		agent:       "test-agent",
		basePath:    env.WorkspacePath,
	}

	if err := registry.Register(tool); err != nil {
		t.Fatalf("failed to register tool: %v", err)
	}

	// Step 4: Emit telemetry event for agent start (telemetry seam)
	env.TelemetrySink.Emit(telemetry.Event{
		Type:      telemetry.EventAgentStart,
		NodeID:    "agent-node",
		TaskID:    "task-1",
		Message:   "agent started",
		Timestamp: time.Now().UTC(),
		Metadata: map[string]any{
			"agent_id": "test-agent",
			"status":   "running",
		},
	})

	// Step 5: Execute the tool (tool execution seam)
	cap, ok := registry.Get("e2e-tool")
	if !ok {
		t.Fatal("tool not found")
	}

	result, err := cap.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("tool execution failed: %v", err)
	}

	if result == nil {
		t.Error("expected non-nil result")
	}

	// Step 6: Emit telemetry event for tool call
	env.TelemetrySink.Emit(telemetry.Event{
		Type:      telemetry.EventToolCall,
		NodeID:    "tool-node",
		TaskID:    "task-1",
		Message:   "tool called",
		Timestamp: time.Now().UTC(),
		Metadata: map[string]any{
			"tool_name": "e2e-tool",
			"status":    "success",
		},
	})

	// Step 7: Store a knowledge chunk (knowledge seam)
	chunk := knowledge.KnowledgeChunk{
		ID:          knowledge.ChunkID("e2e-chunk-1"),
		WorkspaceID: env.WorkspacePath,
		Body: knowledge.ChunkBody{
			Raw: "end-to-end test chunk",
		},
		Provenance: knowledge.ChunkProvenance{
			Sources: []knowledge.ProvenanceSource{
				{Kind: "user", Ref: "test-ref"},
			},
			Timestamp: time.Now().UTC(),
		},
		Freshness: knowledge.FreshnessValid,
		CreatedAt: time.Now().UTC(),
	}

	savedChunk, err := knowledgeStore.Save(chunk)
	if err != nil {
		t.Fatalf("failed to save chunk: %v", err)
	}

	if savedChunk.ID != chunk.ID {
		t.Errorf("expected chunk ID %s, got %s", chunk.ID, savedChunk.ID)
	}

	// Step 8: Emit telemetry event for agent finish
	env.TelemetrySink.Emit(telemetry.Event{
		Type:      telemetry.EventAgentFinish,
		NodeID:    "agent-node",
		TaskID:    "task-1",
		Message:   "agent finished",
		Timestamp: time.Now().UTC(),
		Metadata: map[string]any{
			"agent_id": "test-agent",
			"status":   "completed",
		},
	})

	// Validate seam transition points
	// Telemetry events were captured (telemetry seam)
	events := env.TelemetrySink.Events()
	if len(events) != 3 {
		t.Errorf("expected 3 telemetry events, got %d", len(events))
	}

	// Audit records were created for tool execution (audit seam)
	auditRecords := env.AuditSink.Records()
	if len(auditRecords) == 0 {
		t.Error("expected audit records for tool execution")
	}

	// Chunk was stored and retrievable (knowledge seam)
	retrievedChunk, ok, err := knowledgeStore.Load(chunk.ID)
	if err != nil {
		t.Fatalf("failed to load chunk: %v", err)
	}
	if !ok {
		t.Error("chunk not found")
	}

	if retrievedChunk.Body.Raw != chunk.Body.Raw {
		t.Errorf("expected chunk body %s, got %s", chunk.Body.Raw, retrievedChunk.Body.Raw)
	}

	// Manifest policy was compiled correctly (manifest seam)
	if len(m.Policy.Permissions.FileSystem) == 0 {
		t.Error("expected filesystem permissions in manifest")
	}

	// Capability was registered and executable (capability seam)
	_, ok = registry.Get("e2e-tool")
	if !ok {
		t.Error("expected tool to be registered")
	}
}
