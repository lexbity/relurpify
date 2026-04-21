package testutil

import (
	"context"
	"os"
	"path/filepath"

	"codeburg.org/lexbit/relurpify/ayenitd"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
)

// StubModel returns deterministic no-op completions for tests and benchmarks.
type StubModel struct{}

func (StubModel) Generate(context.Context, string, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: `{"thought":"done","action":"complete","complete":true,"summary":"ok"}`}, nil
}

func (StubModel) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (StubModel) Chat(context.Context, []core.Message, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: `{"thought":"done","action":"complete","complete":true,"summary":"ok"}`}, nil
}

func (StubModel) ChatWithTools(context.Context, []core.Message, []core.LLMToolSpec, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: `{"thought":"done","action":"complete","complete":true,"summary":"ok"}`}, nil
}

type FileWriteTool struct{}

func (FileWriteTool) Name() string        { return "file_write" }
func (FileWriteTool) Description() string { return "writes a file" }
func (FileWriteTool) Category() string    { return "file" }
func (FileWriteTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{
		{Name: "path", Type: "string", Required: true},
		{Name: "content", Type: "string", Required: true},
	}
}
func (FileWriteTool) Execute(_ context.Context, _ *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	path := filepath.Clean(args["path"].(string))
	content := args["content"].(string)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return nil, err
	}
	return &core.ToolResult{Success: true, Data: map[string]any{"path": path}}, nil
}
func (FileWriteTool) IsAvailable(_ context.Context, _ *core.Context) bool { return true }
func (FileWriteTool) Permissions() core.ToolPermissions {
	return core.ToolPermissions{Permissions: &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{{Action: core.FileSystemWrite, Path: "."}},
	}}
}
func (FileWriteTool) Tags() []string { return []string{core.TagDestructive, "file", "edit"} }

type TelemetryRecorder struct {
	Events []core.Event
}

func (r *TelemetryRecorder) Emit(event core.Event) {
	r.Events = append(r.Events, event)
}

// EnvMinimal returns a minimal agentenv.AgentEnvironment for tests that use
// the agents/ implementation layer (chainer, goalcon, htn, react, etc.).
func EnvMinimal() agentenv.AgentEnvironment {
	return agentenv.AgentEnvironment{
		Registry: capability.NewRegistry(),
		Config:   &core.Config{Name: "test", Model: "stub", MaxIterations: 1},
	}
}

// Env returns an agentenv.AgentEnvironment for tests that use the agents/
// implementation layer (chainer, goalcon, htn, react, etc.).
func Env(t interface {
	Helper()
	Fatalf(string, ...interface{})
	TempDir() string
}) agentenv.AgentEnvironment {
	t.Helper()
	memStore, err := memory.NewHybridMemory(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create memory store: %v", err)
	}
	return agentenv.AgentEnvironment{
		Model:    StubModel{},
		Registry: capability.NewRegistry(),
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "test", Model: "stub", MaxIterations: 1},
	}
}

// WorkspaceEnv returns an ayenitd.WorkspaceEnvironment for tests that use named
// agents (euclo, rex, etc.) that accept WorkspaceEnvironment directly.
func WorkspaceEnv(t interface {
	Helper()
	Fatalf(string, ...interface{})
	TempDir() string
}) ayenitd.WorkspaceEnvironment {
	t.Helper()
	memStore, err := memory.NewHybridMemory(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create memory store: %v", err)
	}
	reg := capability.NewRegistry()
	_ = reg.Register(FileWriteTool{})
	return ayenitd.WorkspaceEnvironment{
		Model:    StubModel{},
		Registry: reg,
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "test", Model: "stub", MaxIterations: 1},
	}
}
