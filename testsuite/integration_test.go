package testsuite

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
	"codeburg.org/lexbit/relurpify/framework/search"
)

func TestGraphToolExecutionIntegration(t *testing.T) {
	t.Helper()
	base := t.TempDir()
	path := filepath.Join(base, "note.txt")
	content := "integration payload for the tool registry"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}

	perms := core.NewFileSystemPermissionSet(base, core.FileSystemRead, core.FileSystemList)
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
		decide: func(state *core.Context) (string, error) {
			status, _ := state.Get("use-tool.status")
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
	if err := g.AddEdge(condNode.ID(), terminal.ID(), func(result *core.Result, _ *core.Context) bool {
		next, _ := result.Data["next"].(string)
		return next == "done"
	}, false); err != nil {
		t.Fatalf("edge gate->done: %v", err)
	}

	state := core.NewContext()
	state.Set("task.id", "graph-tool")
	result, err := g.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("graph execute: %v", err)
	}
	if result == nil || result.NodeID != "done" {
		t.Fatalf("unexpected result: %+v", result)
	}

	if val, _ := state.Get("use-tool.content"); val != content {
		t.Fatalf("expected tool content stored, got %v", val)
	}
	if len(state.History()) < 2 {
		t.Fatalf("expected llm and tool interactions recorded, got %d", len(state.History()))
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

	shared := core.NewSharedContext(core.NewContext(), core.NewContextBudget(4096), &core.SimpleSummarizer{})
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
		lang := "text"
		if strings.HasSuffix(result.File, ".go") {
			lang = "go"
		} else if strings.HasSuffix(strings.ToLower(result.File), ".md") {
			lang = "markdown"
		}
		if _, err := shared.AddFile(result.File, string(data), lang, core.DetailFull); err != nil {
			t.Fatalf("AddFile %s: %v", result.File, err)
		}
		seen[result.File] = struct{}{}
	}
	if len(seen) < 2 {
		t.Fatalf("expected files from both search backends, got %v", seen)
	}

	usage := shared.GetTokenUsage()
	if usage.Total == 0 || usage.BySection["files"] == 0 {
		t.Fatalf("token usage not recorded: %+v", usage)
	}
	if _, ok := shared.GetFile(goFile); !ok {
		t.Fatalf("code file missing from shared context")
	}
	if _, ok := shared.GetFile(mdFile); !ok {
		t.Fatalf("notes file missing from shared context")
	}

	shared.OnBudgetWarning(0.95)
	if fc, ok := shared.GetFile(goFile); ok {
		if fc.Level != core.DetailSummary || fc.Content != "" {
			t.Fatalf("expected go file downgraded to summary, got level=%v content len=%d", fc.Level, len(fc.Content))
		}
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
func (t *integrationFileTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{
		{Name: "path", Type: "string", Description: "file to read"},
	}
}

func (t *integrationFileTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	data, err := os.ReadFile(t.path)
	if err != nil {
		return nil, err
	}
	state.AddInteraction("tool:"+t.name, string(data), nil)
	return &core.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"status":  "ok",
			"content": string(data),
		},
	}, nil
}

func (t *integrationFileTool) IsAvailable(context.Context, *core.Context) bool { return true }

func (t *integrationFileTool) Permissions() core.ToolPermissions {
	return core.ToolPermissions{
		Permissions: core.NewFileSystemPermissionSet(t.base, core.FileSystemRead),
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

func (s *scriptedLLM) Generate(ctx context.Context, prompt string, options *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: s.text}, nil
}

func (s *scriptedLLM) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	return nil, fmt.Errorf("streaming not supported")
}

func (s *scriptedLLM) Chat(context.Context, []core.Message, *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, fmt.Errorf("chat not supported")
}

func (s *scriptedLLM) ChatWithTools(context.Context, []core.Message, []core.LLMToolSpec, *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, fmt.Errorf("chat tools not supported")
}

type llmPlanNode struct {
	name   string
	model  core.LanguageModel
	prompt string
}

func (n *llmPlanNode) ID() string { return n.name }

func (n *llmPlanNode) Type() graph.NodeType { return graph.NodeTypeLLM }

func (n *llmPlanNode) Execute(ctx context.Context, state *core.Context) (*core.Result, error) {
	if n.model == nil {
		return nil, fmt.Errorf("llm model missing")
	}
	resp, err := n.model.Generate(ctx, n.prompt, nil)
	if err != nil {
		return nil, err
	}
	state.AddInteraction("assistant", resp.Text, map[string]interface{}{"node": n.name})
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
	tool core.Tool
	args map[string]interface{}
}

func (n *toolExecNode) ID() string { return n.name }

func (n *toolExecNode) Type() graph.NodeType { return graph.NodeTypeTool }

func (n *toolExecNode) Execute(ctx context.Context, state *core.Context) (*core.Result, error) {
	if n.tool == nil {
		return nil, fmt.Errorf("tool missing")
	}
	if !n.tool.IsAvailable(ctx, state) {
		return nil, fmt.Errorf("tool %s unavailable", n.tool.Name())
	}
	res, err := n.tool.Execute(ctx, state, n.args)
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
	return &core.Result{
		NodeID:  n.name,
		Success: success,
		Data:    data,
		Error:   execErr,
	}, nil
}

type stateConditionalNode struct {
	name   string
	decide func(*core.Context) (string, error)
}

func (n *stateConditionalNode) ID() string { return n.name }

func (n *stateConditionalNode) Type() graph.NodeType { return graph.NodeTypeConditional }

func (n *stateConditionalNode) Execute(ctx context.Context, state *core.Context) (*core.Result, error) {
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
