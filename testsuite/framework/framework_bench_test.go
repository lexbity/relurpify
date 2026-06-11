package framework

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/capability/ports"
	regpkg "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/capability/toolcapabilities"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	"codeburg.org/lexbit/relurpify/governance/authorization"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/model"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// BenchmarkPolicyEvaluation measures the performance of policy evaluation
// through the permission manager for file access checks.
func BenchmarkPolicyEvaluation(b *testing.B) {
	// Create a temporary workspace for benchmarking
	workspace := b.TempDir()

	// Create a test file for permission checks
	testFile := filepath.Join(workspace, "bench-test.txt")
	if err := os.WriteFile(testFile, []byte("benchmark test content"), 0o644); err != nil {
		b.Fatalf("failed to write test file: %v", err)
	}

	// Create permission manager
	perms := policy.NewFileSystemPermissionSet(workspace, permissions.FileSystemRead, permissions.FileSystemList)
	auditSink := &recordingAuditSink{}
	manager, err := authorization.NewPermissionManager(workspace, perms, auditSink, nil)
	if err != nil {
		b.Fatalf("failed to create permission manager: %v", err)
	}

	ctx := context.Background()
	agentID := "bench-agent"

	// Reset the timer to exclude setup time
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		err := manager.CheckFileAccess(ctx, agentID, permissions.FileSystemRead, testFile)
		if err != nil {
			b.Fatalf("permission check failed: %v", err)
		}
	}
}

// BenchmarkCapabilityDispatch measures the performance of tool registration
// and retrieval from the capability registry.
func BenchmarkCapabilityDispatch(b *testing.B) {
	// Create a registry for benchmarking
	registry := regpkg.NewRegistry()

	// Register a tool for benchmarking
	tool := &benchTool{name: "bench-tool"}
	if err := registry.Register(context.Background(), tool); err != nil {
		b.Fatalf("tool registration failed: %v", err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, ok := registry.Get("bench-tool")
		if !ok {
			b.Fatal("tool not found in registry")
		}
	}
}

// BenchmarkWorkspaceScanning measures the performance of workspace file scanning
// through the knowledge ingestion pipeline.
func BenchmarkWorkspaceScanning(b *testing.B) {
	// Create a temporary workspace for benchmarking
	workspace := b.TempDir()

	// Create multiple test files for scanning
	numFiles := 10
	for i := 0; i < numFiles; i++ {
		testFile := filepath.Join(workspace, fmt.Sprintf("bench-file-%d.go", i))
		content := `package bench

func TestFunc` + string(rune('A'+i)) + `() string {
	return "test"
}
`
		if err := os.WriteFile(testFile, []byte(content), 0o644); err != nil {
			b.Fatalf("failed to write test file: %v", err)
		}
	}

	// Create graph engine for knowledge storage
	opts := graphdb.DefaultOptions(workspace)
	graph, err := graphdb.Open(context.Background(), opts)
	if err != nil {
		b.Fatalf("failed to open graph engine: %v", err)
	}
	defer graph.Close(context.Background())

	store := &knowledge.ChunkStore{Graph: graph}
	events := &knowledge.EventBus{}
	ingester := knowledge.NewOutputIngester(store, events)

	envelope := contextdata.NewEnvelope("bench-task", "bench-session")
	ctx := contextdata.WithEnvelope(context.Background(), envelope)

	// Create a simple LLM response for benchmarking
	llmResponse := &model.LLMResponse{
		Text: "benchmark test content",
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := ingester.IngestLLMResponseFull(ctx, llmResponse)
		if err != nil {
			b.Fatalf("ingestion failed: %v", err)
		}
	}
}

// BenchmarkContextStreaming measures the performance of context envelope
// operations including setting and retrieving working values.
func BenchmarkContextStreaming(b *testing.B) {
	envelope := contextdata.NewEnvelope("bench-task", "bench-session")

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key := "bench-key"
		value := "bench-value"
		envelope.SetWorkingValueWithClass(key, value, contextdata.MemoryClassTask)
		_, ok := envelope.GetWorkingValue(key)
		if !ok {
			b.Fatal("working value not found")
		}
	}
}

// BenchmarkTelemetryEmission measures the performance of telemetry event
// emission and capture through the telemetry sink.
func BenchmarkTelemetryEmission(b *testing.B) {
	telemetrySink := &recordingTelemetrySink{}

	event := telemetry.Event{
		Type:      telemetry.EventNodeFinish,
		NodeID:    "bench-node",
		TaskID:    "bench-task",
		Message:   "benchmark event",
		Timestamp: time.Now().UTC(),
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		telemetrySink.Emit(event)
	}
}

// BenchmarkAuditLogging measures the performance of audit record logging
// and retrieval through the audit sink.
func BenchmarkAuditLogging(b *testing.B) {
	auditSink := &recordingAuditSink{}

	ctx := context.Background()
	record := policy.AuditRecord{
		AgentID:    "bench-agent",
		Type:       string(permissions.PermissionTypeFilesystem),
		Action:     "file:read",
		Permission: "/bench/path",
		Result:     "granted",
		Timestamp:  time.Now().UTC(),
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		err := auditSink.Log(ctx, record)
		if err != nil {
			b.Fatalf("audit log failed: %v", err)
		}
	}
}

// benchTool is a minimal tool implementation for benchmarking.
type benchTool struct {
	name string
}

func (b *benchTool) Name() string        { return b.name }
func (b *benchTool) Description() string { return "benchmark tool" }
func (b *benchTool) Category() string    { return "test" }
func (b *benchTool) Parameters() []ports.ToolParameter {
	return []ports.ToolParameter{
		{Name: "input", Type: "string", Description: "input parameter"},
	}
}

func (b *benchTool) Execute(ctx context.Context, args map[string]any) (*ports.ToolResult, error) {
	return &ports.ToolResult{
		Success: true,
		Data: map[string]any{
			"result": "bench",
		},
	}, nil
}

func (b *benchTool) Permissions() ports.ToolPermissions {
	return ports.ToolPermissions{}
}

func (b *benchTool) Tags() []string {
	return []string{toolcapabilities.TagReadOnly}
}

func (b *benchTool) IsAvailable(context.Context) bool { return true }
