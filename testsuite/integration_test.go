package testsuite

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

func TestGraphToolExecutionIntegration(t *testing.T) {
	t.Helper()
	base := t.TempDir()
	path := filepath.Join(base, "note.txt")
	content := "integration payload for the tool registry"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}

	perms := core.NewFileSystemPermissionSet(base, contracts.FileSystemRead, contracts.FileSystemList)
	manager, err := authorization.NewPermissionManager(base, perms, nil, nil)
	if err != nil {
		t.Fatalf("permission manager: %v", err)
	}

	registry := capability.NewRegistry()
	fileTool := &integrationFileTool{
		name: "read_note",
		base: base,
		path: path,
	}
	if err := registry.Register(fileTool); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	registry.UsePermissionManager("agent-integration", manager)
	tool, ok := registry.Get("read_note")
	if !ok {
		t.Fatalf("wrapped tool missing")
	}

	g := graph.NewGraph()
	telemetry := &recordingTelemetry{}
	g.SetTelemetry(telemetry)

	llmNode := &llmPlanNode{
		name:   "planner",
		model:  &scriptedLLM{text: "Plan: inspect workspace note"},
		prompt: "Summarize task",
	}
	toolNode := &toolExecNode{
		name: "use-tool",
		tool: tool,
		args: map[string]interface{}{"path": path},
	}
	condNode := &stateConditionalNode{
		name: "gate",
		decide: func(state *contextdata.Envelope) (string, error) {
			status, _ := state.GetWorkingValue("use-tool.status")
			if fmt.Sprint(status) == "ok" {
				return "done", nil
			}
			return "", fmt.Errorf("tool status missing")
		},
	}
	terminal := graph.NewTerminalNode("done")

	for _, node := range []graph.Node{llmNode, toolNode, condNode, terminal} {
		if err := g.AddNode(node); err != nil {
			t.Fatalf("add node %s: %v", node.ID(), err)
		}
	}
	if err := g.SetStart(llmNode.ID()); err != nil {
		t.Fatalf("set start: %v", err)
	}
	if err := g.AddEdge(llmNode.ID(), toolNode.ID(), nil, false); err != nil {
		t.Fatalf("edge planner->tool: %v", err)
	}
	if err := g.AddEdge(toolNode.ID(), condNode.ID(), nil, false); err != nil {
		t.Fatalf("edge tool->gate: %v", err)
	}
	if err := g.AddEdge(condNode.ID(), terminal.ID(), func(result *core.Result, _ *contextdata.Envelope) bool {
		next, _ := result.Data["next"].(string)
		return next == "done"
	}, false); err != nil {
		t.Fatalf("edge gate->done: %v", err)
	}

	state := contextdata.NewEnvelope("graph-tool", "integration")
	result, err := g.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("graph execute: %v", err)
	}
	if result == nil || result.NodeID != "done" {
		t.Fatalf("unexpected result: %+v", result)
	}

	if val, _ := state.GetWorkingValue("use-tool.content"); val != content {
		t.Fatalf("expected tool content stored, got %v", val)
	}
	if len(state.GetInteractions()) < 2 {
		t.Fatalf("expected llm and tool interactions recorded, got %d", len(state.GetInteractions()))
	}
	if telemetry.count(core.EventGraphStart) != 1 || telemetry.count(core.EventGraphFinish) != 1 {
		t.Fatalf("graph telemetry mismatch: %+v", telemetry.events)
	}
	if telemetry.count(core.EventNodeStart) != 4 || telemetry.count(core.EventNodeFinish) != 4 {
		t.Fatalf("node telemetry mismatch: %+v", telemetry.events)
	}
}

func TestHybridSearchFeedsSharedContext(t *testing.T) {
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
	codeIndex := &astCodeIndex{store: store}
	vector := &stubVectorStore{
		results: []search.VectorMatch{{
			ID:      "semantic-1",
			Content: "notes mention highlight integration",
			Metadata: map[string]any{
				"path": mdFile,
			},
			Score: 0.91,
		}},
	}
	engine := search.NewSearchEngine(vector, codeIndex)
	results, err := engine.Search(search.SearchQuery{
		Text:       "HighlightFeature",
		Mode:       search.SearchHybrid,
		MaxResults: 4,
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected hybrid search results")
	}

	shared := contextdata.NewEnvelope("integration", "integration")
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

type recordingTelemetry struct {
	mu     sync.Mutex
	events []core.Event
}

func (r *recordingTelemetry) Emit(event core.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *recordingTelemetry) count(eventType core.EventType) int {
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

type integrationFileTool struct {
	name string
	base string
	path string
}

func (t *integrationFileTool) Name() string        { return t.name }
func (t *integrationFileTool) Description() string { return "reads a workspace note" }
func (t *integrationFileTool) Category() string    { return "filesystem" }
func (t *integrationFileTool) Parameters() []contracts.ToolParameter {
	return []contracts.ToolParameter{
		{Name: "path", Type: "string", Description: "file to read"},
	}
}

func (t *integrationFileTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
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

func (t *integrationFileTool) IsAvailable(context.Context) bool { return true }

func (t *integrationFileTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{
		Permissions: core.NewFileSystemPermissionSet(t.base, contracts.FileSystemRead),
	}
}
func (t *integrationFileTool) Tags() []string { return nil }

type stubVectorStore struct {
	results []search.VectorMatch
}

func (s *stubVectorStore) Query(context.Context, string, int) ([]search.VectorMatch, error) {
	return s.results, nil
}

type scriptedLLM struct {
	text string
}

func (s *scriptedLLM) Generate(ctx context.Context, prompt string, options *contracts.LLMOptions) (*contracts.LLMResponse, error) {
	return &contracts.LLMResponse{Text: s.text}, nil
}

func (s *scriptedLLM) GenerateStream(context.Context, string, *contracts.LLMOptions) (<-chan string, error) {
	return nil, fmt.Errorf("streaming not supported")
}

func (s *scriptedLLM) Chat(context.Context, []contracts.Message, *contracts.LLMOptions) (*contracts.LLMResponse, error) {
	return nil, fmt.Errorf("chat not supported")
}

func (s *scriptedLLM) ChatWithTools(context.Context, []contracts.Message, []contracts.LLMToolSpec, *contracts.LLMOptions) (*contracts.LLMResponse, error) {
	return nil, fmt.Errorf("chat tools not supported")
}

type llmPlanNode struct {
	name   string
	model  contracts.LanguageModel
	prompt string
}

func (n *llmPlanNode) ID() string { return n.name }

func (n *llmPlanNode) Type() graph.NodeType { return graph.NodeTypeSystem }

func (n *llmPlanNode) Execute(ctx context.Context, state *contextdata.Envelope) (*core.Result, error) {
	if n.model == nil {
		return nil, fmt.Errorf("llm model missing")
	}
	resp, err := n.model.Generate(ctx, n.prompt, nil)
	if err != nil {
		return nil, err
	}
	state.AddInteraction(map[string]any{"actor": "assistant", "content": resp.Text, "node": n.name})
	return &core.Result{
		NodeID:  n.name,
		Success: true,
		Data: map[string]interface{}{
			"text": resp.Text,
		},
	}, nil
}

type toolExecNode struct {
	name string
	tool contracts.Tool
	args map[string]interface{}
}

func (n *toolExecNode) ID() string { return n.name }

func (n *toolExecNode) Type() graph.NodeType { return graph.NodeTypeTool }

func (n *toolExecNode) Execute(ctx context.Context, state *contextdata.Envelope) (*core.Result, error) {
	if n.tool == nil {
		return nil, fmt.Errorf("tool missing")
	}
	if !n.tool.IsAvailable(ctx) {
		return nil, fmt.Errorf("tool %s unavailable", n.tool.Name())
	}
	res, err := n.tool.Execute(ctx, n.args)
	if err != nil {
		return nil, err
	}
	data := make(map[string]interface{})
	if res != nil && res.Data != nil {
		for k, v := range res.Data {
			data[k] = v
		}
	}
	success := true
	if res != nil {
		success = res.Success
	}
	var execErr error
	if res != nil && res.Error != "" {
		execErr = fmt.Errorf("%s", res.Error)
	}
	if content, ok := data["content"].(string); ok {
		state.SetWorkingValue("use-tool.content", content, contextdata.MemoryClassTask)
	}
	state.AddInteraction(map[string]any{"actor": "tool:" + n.name, "result": data})
	return &core.Result{
		NodeID:  n.name,
		Success: success,
		Data:    data,
		Error: func() string {
			if execErr != nil {
				return execErr.Error()
			}
			return ""
		}(),
	}, nil
}

type stateConditionalNode struct {
	name   string
	decide func(*contextdata.Envelope) (string, error)
}

func (n *stateConditionalNode) ID() string { return n.name }

func (n *stateConditionalNode) Type() graph.NodeType { return graph.NodeTypeConditional }

func (n *stateConditionalNode) Execute(ctx context.Context, state *contextdata.Envelope) (*core.Result, error) {
	if n.decide == nil {
		return nil, fmt.Errorf("conditional missing decision function")
	}
	next, err := n.decide(state)
	if err != nil {
		return nil, err
	}
	return &core.Result{
		NodeID:  n.name,
		Success: true,
		Data: map[string]interface{}{
			"next": next,
		},
	}, nil
}
