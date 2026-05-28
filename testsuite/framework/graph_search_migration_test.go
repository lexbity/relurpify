package framework

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	graph "codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/search"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// TestGraphCapabilityExecutionMigration validates the migrated graph + capability
// seam coverage from the root integration suite.
func TestGraphCapabilityExecutionMigration(t *testing.T) {
	env := NewTestEnvironment(t)

	notePath := filepath.Join(env.WorkspacePath, "note.txt")
	content := "integration payload for the tool registry"
	if err := os.WriteFile(notePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}

	perms := core.NewFileSystemPermissionSet(env.WorkspacePath, contracts.FileSystemRead, contracts.FileSystemList)
	manager, err := authorization.NewPermissionManager(env.WorkspacePath, perms, env.AuditSink, nil)
	if err != nil {
		t.Fatalf("permission manager: %v", err)
	}

	registry := capability.NewRegistry()
	registry.UsePermissionManager("migration-agent", manager)

	tool := &migrationWorkspaceTool{
		name:        "read_note",
		description: "reads a workspace note",
		category:    "filesystem",
		path:        notePath,
		basePath:    env.WorkspacePath,
	}
	if err := registry.Register(tool); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	telemetry := &migrationTelemetry{}
	g := graph.NewGraph()
	g.SetTelemetry(telemetry)

	planner := &migrationPlannerNode{id: "planner", message: "plan workspace note inspection"}
	exec := &migrationCapabilityNode{id: "read-note", registry: registry, toolName: "read_note"}
	gate := &migrationGateNode{id: "gate", expectedKey: "read_note.content"}
	terminal := graph.NewTerminalNode("done")

	for _, node := range []graph.Node{planner, exec, gate, terminal} {
		if err := g.AddNode(node); err != nil {
			t.Fatalf("add node %s: %v", node.ID(), err)
		}
	}
	if err := g.SetStart(planner.ID()); err != nil {
		t.Fatalf("set start: %v", err)
	}
	if err := g.AddEdge(planner.ID(), exec.ID(), nil, false); err != nil {
		t.Fatalf("edge planner->tool: %v", err)
	}
	if err := g.AddEdge(exec.ID(), gate.ID(), nil, false); err != nil {
		t.Fatalf("edge tool->gate: %v", err)
	}
	if err := g.AddEdge(gate.ID(), terminal.ID(), func(result *core.Result, _ *contextdata.Envelope) bool {
		next, _ := core.ResultField(result.Data, "next")
		return next == "done"
	}, false); err != nil {
		t.Fatalf("edge gate->done: %v", err)
	}

	state := contextdata.NewEnvelope("graph-tool", "migration")
	result, err := g.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("graph execute: %v", err)
	}
	if result == nil || result.NodeID != "done" {
		t.Fatalf("unexpected result: %+v", result)
	}

	if got, ok := state.GetWorkingValue("read_note.content"); !ok || got != content {
		t.Fatalf("expected content in envelope, got %v (present=%v)", got, ok)
	}
	if got, ok := state.GetWorkingValue("read_note.status"); !ok || got != "ok" {
		t.Fatalf("expected tool status in envelope, got %v (present=%v)", got, ok)
	}
	if len(state.GetInteractions()) < 2 {
		t.Fatalf("expected planner and tool interactions, got %d", len(state.GetInteractions()))
	}
	if telemetry.count(core.EventGraphStart) != 1 || telemetry.count(core.EventGraphFinish) != 1 {
		t.Fatalf("graph telemetry mismatch: %+v", telemetry.events)
	}
	if telemetry.count(core.EventNodeStart) != 4 || telemetry.count(core.EventNodeFinish) != 4 {
		t.Fatalf("node telemetry mismatch: %+v", telemetry.events)
	}
}

// TestHybridSearchFeedsSharedContextMigration validates the migrated hybrid search
// and shared-context flow from the root integration suite.
func TestHybridSearchFeedsSharedContextMigration(t *testing.T) {
	temp := t.TempDir()
	goFile := filepath.Join(temp, "service.go")
	goSource := "package service\n\nfunc HighlightFeature() string {\n\treturn \"ready\"\n}\n\n"
	goSource += strings.Repeat("// highlight integration coverage\n", 200)
	if err := os.WriteFile(goFile, []byte(goSource), 0o644); err != nil {
		t.Fatalf("write go source: %v", err)
	}
	mdFile := filepath.Join(temp, "NOTES.md")
	mdSource := strings.Repeat("highlight integration behavior\n", 80)
	if err := os.WriteFile(mdFile, []byte(mdSource), 0o644); err != nil {
		t.Fatalf("write notes: %v", err)
	}

	store, err := ast.NewSQLiteStore(filepath.Join(temp, "idx.db"))
	if err != nil {
		t.Fatalf("sqlite: %v", err)
	}
	defer store.Close()
	indexer := ast.NewIndexManager(store, ast.IndexConfig{WorkspacePath: temp})
	if err := indexer.IndexWorkspace(); err != nil {
		t.Fatalf("index workspace: %v", err)
	}

	engine := search.NewSearchEngine(&migrationVectorStore{results: []search.VectorMatch{{
		ID:      "semantic-1",
		Content: "notes mention highlight integration",
		Metadata: map[string]any{
			"path": mdFile,
		},
		Score: 0.91,
	}}}, &migrationCodeIndex{store: store})
	results, err := engine.Search(search.SearchQuery{Text: "HighlightFeature", Mode: search.SearchHybrid, MaxResults: 4})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected hybrid search results")
	}

	shared := contextdata.NewEnvelope("migration", "migration")
	seen := make(map[string]struct{})
	for _, result := range results {
		if result.File == "" {
			continue
		}
		if _, exists := seen[result.File]; exists {
			continue
		}
		data, err := os.ReadFile(result.File)
		if err != nil {
			t.Fatalf("read %s: %v", result.File, err)
		}
		shared.SetWorkingValue(result.File, string(data), contextdata.MemoryClassTask)
		seen[result.File] = struct{}{}
	}
	if len(seen) < 2 {
		t.Fatalf("expected files from both search backends, got %v", seen)
	}
	if _, ok := shared.GetWorkingValue(goFile); !ok {
		t.Fatalf("code file missing from shared context")
	}
	if _, ok := shared.GetWorkingValue(mdFile); !ok {
		t.Fatalf("notes file missing from shared context")
	}
}

type migrationTelemetry struct {
	mu     sync.Mutex
	events []core.Event
}

func (r *migrationTelemetry) Emit(event core.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *migrationTelemetry) count(eventType core.EventType) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	total := 0
	for _, event := range r.events {
		if event.Type == eventType {
			total++
		}
	}
	return total
}

type migrationWorkspaceTool struct {
	name        string
	description string
	category    string
	basePath    string
	path        string
	manager     *authorization.PermissionManager
	agent       string
}

func (t *migrationWorkspaceTool) Name() string                          { return t.name }
func (t *migrationWorkspaceTool) Description() string                   { return t.description }
func (t *migrationWorkspaceTool) Category() string                      { return t.category }
func (t *migrationWorkspaceTool) Parameters() []contracts.ToolParameter { return nil }
func (t *migrationWorkspaceTool) SetPermissionManager(manager *authorization.PermissionManager, agentID string) {
	t.manager = manager
	t.agent = agentID
}
func (t *migrationWorkspaceTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	if t.manager != nil {
		if err := t.manager.CheckFileAccess(ctx, t.agent, contracts.FileSystemRead, t.path); err != nil {
			return nil, err
		}
	}
	data, err := os.ReadFile(t.path)
	if err != nil {
		return nil, err
	}
	return &contracts.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"status":  "ok",
			"content": string(data),
		},
	}, nil
}
func (t *migrationWorkspaceTool) IsAvailable(context.Context) bool { return true }
func (t *migrationWorkspaceTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{Permissions: core.NewFileSystemPermissionSet(t.basePath, contracts.FileSystemRead)}
}
func (t *migrationWorkspaceTool) Tags() []string { return nil }

type migrationPlannerNode struct {
	id      string
	message string
}

func (n *migrationPlannerNode) ID() string           { return n.id }
func (n *migrationPlannerNode) Type() graph.NodeType { return graph.NodeTypeSystem }
func (n *migrationPlannerNode) Execute(ctx context.Context, state *contextdata.Envelope) (*core.Result, error) {
	state.AddInteraction(map[string]any{"actor": "assistant", "content": n.message, "node": n.id})
	return &core.Result{
		NodeID:  n.id,
		Success: true,
		Data: core.NewToolResultPayload(map[string]any{
			"next":    "run-tool",
			"message": n.message,
		}),
	}, nil
}

type migrationCapabilityNode struct {
	id       string
	registry *capability.Registry
	toolName string
}

func (n *migrationCapabilityNode) ID() string           { return n.id }
func (n *migrationCapabilityNode) Type() graph.NodeType { return graph.NodeTypeTool }
func (n *migrationCapabilityNode) Execute(ctx context.Context, state *contextdata.Envelope) (*core.Result, error) {
	capabilityTool, ok := n.registry.Get(n.toolName)
	if !ok {
		return nil, fmt.Errorf("capability %q not found", n.toolName)
	}
	result, err := capabilityTool.Execute(ctx, nil)
	if err != nil {
		return nil, err
	}
	if result != nil {
		fields := result.Data
		if status, ok := fields["status"].(string); ok {
			state.SetWorkingValue(n.toolName+".status", status, contextdata.MemoryClassTask)
		}
		if content, ok := fields["content"].(string); ok {
			state.SetWorkingValue(n.toolName+".content", content, contextdata.MemoryClassTask)
		}
	}
	state.AddInteraction(map[string]any{"actor": "tool:" + n.toolName, "result": result.Data})
	return &core.Result{
		NodeID:  n.id,
		Success: true,
		Data: core.NewToolResultPayload(map[string]any{
			"next": "done",
		}),
	}, nil
}

type migrationGateNode struct {
	id          string
	expectedKey string
}

func (n *migrationGateNode) ID() string           { return n.id }
func (n *migrationGateNode) Type() graph.NodeType { return graph.NodeTypeConditional }
func (n *migrationGateNode) Execute(ctx context.Context, state *contextdata.Envelope) (*core.Result, error) {
	if n.expectedKey != "" {
		value, ok := state.GetWorkingValue(n.expectedKey)
		if !ok {
			return nil, fmt.Errorf("expected envelope key %q to be present", n.expectedKey)
		}
		if value == "" {
			return nil, fmt.Errorf("expected envelope key %q to carry a non-empty value", n.expectedKey)
		}
	}
	return &core.Result{NodeID: n.id, Success: true, Data: core.NewToolResultPayload(map[string]any{"next": "done"})}, nil
}

type migrationVectorStore struct {
	results []search.VectorMatch
}

func (s *migrationVectorStore) Query(context.Context, string, int) ([]search.VectorMatch, error) {
	return s.results, nil
}

type migrationCodeIndex struct {
	store *ast.SQLiteStore
}

func (a *migrationCodeIndex) GetFileMetadata(string) (any, bool) { return nil, false }
func (a *migrationCodeIndex) ListFiles() []string                { return nil }
func (a *migrationCodeIndex) GetSymbolsByName(string) ([]search.SymbolLocation, error) {
	return nil, nil
}
func (a *migrationCodeIndex) GetSymbolDefinition(string) (*search.SymbolLocation, error) {
	return nil, nil
}
func (a *migrationCodeIndex) GetSymbolReferences(string) ([]search.SymbolLocation, error) {
	return nil, nil
}
func (a *migrationCodeIndex) GetFileDependencies(string) []string { return nil }
func (a *migrationCodeIndex) GetDependents(string) []string       { return nil }
func (a *migrationCodeIndex) GetChunksForFile(string) []*search.CodeChunk {
	return nil
}
func (a *migrationCodeIndex) GetChunkByID(string) (*search.CodeChunk, bool) { return nil, false }
func (a *migrationCodeIndex) FindChunksByName(string) []*search.CodeChunk   { return nil }
func (a *migrationCodeIndex) FindChunksByFileAndRange(string, int, int) []*search.CodeChunk {
	return nil
}
func (a *migrationCodeIndex) SearchChunks(query string, limit int) []*search.CodeChunk {
	nodes, err := a.store.SearchNodes(ast.NodeQuery{})
	if err != nil {
		return nil
	}
	query = strings.ToLower(query)
	results := make([]*search.CodeChunk, 0, len(nodes))
	seen := make(map[string]struct{})
	for _, node := range nodes {
		if node.Name == "" || !strings.Contains(strings.ToLower(node.Name), query) {
			continue
		}
		meta, err := a.store.GetFile(node.FileID)
		if err != nil || meta == nil {
			continue
		}
		chunkID := fmt.Sprintf("%s:%s:%d", node.FileID, node.Name, node.StartLine)
		if _, exists := seen[chunkID]; exists {
			continue
		}
		seen[chunkID] = struct{}{}
		lineSpan := node.EndLine - node.StartLine + 1
		if lineSpan <= 0 {
			lineSpan = 1
		}
		results = append(results, &search.CodeChunk{
			ID:         chunkID,
			File:       meta.Path,
			Kind:       search.ChunkFunction,
			Name:       node.Name,
			StartLine:  node.StartLine,
			EndLine:    node.EndLine,
			Summary:    node.DocString,
			Preview:    node.Name,
			TokenCount: lineSpan,
		})
		if limit > 0 && len(results) >= limit {
			break
		}
	}
	return results
}
