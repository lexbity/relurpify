package pattern

import (
	"context"
	"errors"
	"fmt"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/toolsys"
	"github.com/stretchr/testify/assert"
	"testing"
)

type stubLLM struct {
	responses      []*core.LLMResponse
	idx            int
	generateCalls  int
	withToolsCalls int
}

// Generate returns the next queued LLM response for deterministic tests.
func (s *stubLLM) Generate(ctx context.Context, prompt string, options *core.LLMOptions) (*core.LLMResponse, error) {
	s.generateCalls++
	return s.nextResponse()
}

// GenerateStream is unused in tests.
func (s *stubLLM) GenerateStream(ctx context.Context, prompt string, options *core.LLMOptions) (<-chan string, error) {
	return nil, errors.New("not implemented")
}

// Chat is unused in tests.
func (s *stubLLM) Chat(ctx context.Context, messages []core.Message, options *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, errors.New("not implemented")
}

// ChatWithTools returns the next queued response and increments instrumentation.
func (s *stubLLM) ChatWithTools(ctx context.Context, messages []core.Message, tools []core.Tool, options *core.LLMOptions) (*core.LLMResponse, error) {
	s.withToolsCalls++
	return s.nextResponse()
}

// nextResponse pops the next canned response or returns an error when empty.
func (s *stubLLM) nextResponse() (*core.LLMResponse, error) {
	if s.idx >= len(s.responses) {
		return nil, errors.New("no response")
	}
	resp := s.responses[s.idx]
	s.idx++
	return resp, nil
}

type stubTool struct {
	name string
}

// Name returns the tool identifier used in tool calls.
func (t stubTool) Name() string { return t.name }

// Description provides a friendly label for CLI output.
func (t stubTool) Description() string { return "stub tool" }

// Category groups the tool in mock registries.
func (t stubTool) Category() string { return "test" }

// Parameters exposes the single optional argument accepted by the stub.
func (t stubTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{
		{Name: "value", Type: "string", Required: false},
	}
}

// Execute echoes the provided "value" argument to simulate tool output.
func (t stubTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"echo": args["value"],
		},
	}, nil
}

// IsAvailable always returns true for simplicity.
func (t stubTool) IsAvailable(ctx context.Context, state *core.Context) bool { return true }

// Permissions returns a read-only filesystem grant.
func (t stubTool) Permissions() core.ToolPermissions {
	return core.ToolPermissions{Permissions: &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{
			{Action: core.FileSystemRead, Path: "**"},
		},
	}}
}

// TestReActAgentExecute validates a minimal think-act-observe pass.
func TestReActAgentExecute(t *testing.T) {
	llm := &stubLLM{
		responses: []*core.LLMResponse{
			{Text: `{"thought":"finished","tool":"none","arguments":{},"complete":true}`},
		},
	}
	registry := toolsys.NewToolRegistry()
	assert.NoError(t, registry.Register(stubTool{name: "echo"}))
	agent := &ReActAgent{
		Model: llm,
		Tools: registry,
	}
	cfg := &core.Config{Model: "test-model", MaxIterations: 2, OllamaToolCalling: true}
	assert.NoError(t, agent.Initialize(cfg))

	task := &core.Task{ID: "task-1", Instruction: "do something"}
	state := core.NewContext()
	state.Set("task.id", task.ID)

	think := &reactThinkNode{id: "think", agent: agent, task: task}
	act := &reactActNode{id: "act", agent: agent}
	observe := &reactObserveNode{id: "observe", agent: agent, task: task}
	terminal := graph.NewTerminalNode("done")

	graph := graph.NewGraph()
	assert.NoError(t, graph.AddNode(think))
	assert.NoError(t, graph.AddNode(act))
	assert.NoError(t, graph.AddNode(observe))
	assert.NoError(t, graph.AddNode(terminal))
	assert.NoError(t, graph.SetStart("think"))
	assert.NoError(t, graph.AddEdge("think", "act", nil, false))
	assert.NoError(t, graph.AddEdge("act", "observe", nil, false))
	assert.NoError(t, graph.AddEdge("observe", "done", nil, false))

	// run single pass (no loop) to validate node behavior
	result, err := graph.Execute(context.Background(), state)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "done", result.NodeID)

	final, ok := state.Get("react.final_output")
	assert.True(t, ok, "final output should be stored in context")
	assert.Contains(t, final.(map[string]interface{})["summary"], "Iteration")
	assert.Equal(t, 1, llm.withToolsCalls)
	assert.Equal(t, 0, llm.generateCalls)
}

// TestReActAgentToolCallingDisabled ensures the agent falls back to plain text.
func TestReActAgentToolCallingDisabled(t *testing.T) {
	llm := &stubLLM{
		responses: []*core.LLMResponse{
			{Text: `{"thought":"finished","tool":"none","arguments":{},"complete":true}`},
		},
	}
	registry := toolsys.NewToolRegistry()
	assert.NoError(t, registry.Register(stubTool{name: "echo"}))
	agent := &ReActAgent{
		Model: llm,
		Tools: registry,
	}
	cfg := &core.Config{Model: "test-model", MaxIterations: 2, OllamaToolCalling: false}
	assert.NoError(t, agent.Initialize(cfg))

	task := &core.Task{ID: "task-2", Instruction: "do something"}
	state := core.NewContext()
	state.Set("task.id", task.ID)

	think := &reactThinkNode{id: "think", agent: agent, task: task}
	act := &reactActNode{id: "act", agent: agent}
	observe := &reactObserveNode{id: "observe", agent: agent, task: task}
	terminal := graph.NewTerminalNode("done")

	graph := graph.NewGraph()
	assert.NoError(t, graph.AddNode(think))
	assert.NoError(t, graph.AddNode(act))
	assert.NoError(t, graph.AddNode(observe))
	assert.NoError(t, graph.AddNode(terminal))
	assert.NoError(t, graph.SetStart("think"))
	assert.NoError(t, graph.AddEdge("think", "act", nil, false))
	assert.NoError(t, graph.AddEdge("act", "observe", nil, false))
	assert.NoError(t, graph.AddEdge("observe", "done", nil, false))

	result, err := graph.Execute(context.Background(), state)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "done", result.NodeID)
	assert.Equal(t, 0, llm.withToolsCalls)
	assert.Equal(t, 1, llm.generateCalls)
}

// TestReActAgentToolCallingFallbackExecutesParsedToolCalls ensures that
// tool calls parsed from plain text are still executed when tool calling is off.
func TestReActAgentToolCallingFallbackExecutesParsedToolCalls(t *testing.T) {
	llm := &stubLLM{
		responses: []*core.LLMResponse{
			{Text: `{"tool":"echo","arguments":{"value":"hi"}}`},
		},
	}
	registry := toolsys.NewToolRegistry()
	assert.NoError(t, registry.Register(stubTool{name: "echo"}))
	agent := &ReActAgent{
		Model: llm,
		Tools: registry,
	}
	cfg := &core.Config{Model: "test-model", MaxIterations: 2, OllamaToolCalling: false}
	assert.NoError(t, agent.Initialize(cfg))

	task := &core.Task{ID: "task-3", Instruction: "use tool via fallback"}
	state := core.NewContext()
	state.Set("task.id", task.ID)

	think := &reactThinkNode{id: "think", agent: agent, task: task}
	act := &reactActNode{id: "act", agent: agent}
	observe := &reactObserveNode{id: "observe", agent: agent, task: task}
	terminal := graph.NewTerminalNode("done")

	graph := graph.NewGraph()
	assert.NoError(t, graph.AddNode(think))
	assert.NoError(t, graph.AddNode(act))
	assert.NoError(t, graph.AddNode(observe))
	assert.NoError(t, graph.AddNode(terminal))
	assert.NoError(t, graph.SetStart("think"))
	assert.NoError(t, graph.AddEdge("think", "act", nil, false))
	assert.NoError(t, graph.AddEdge("act", "observe", nil, false))
	assert.NoError(t, graph.AddEdge("observe", "done", nil, false))

	result, err := graph.Execute(context.Background(), state)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "done", result.NodeID)
	assert.Equal(t, 0, llm.withToolsCalls)
	assert.Equal(t, 1, llm.generateCalls)

	lastToolRes, ok := state.Get("react.last_tool_result")
	assert.True(t, ok)
	assert.Contains(t, fmt.Sprint(lastToolRes.(map[string]interface{})["echo"]), "hi")
}

// TestReActAgentToolCalling verifies tool call handling and transcript storage.
func TestReActAgentToolCalling(t *testing.T) {
	llm := &stubLLM{
		responses: []*core.LLMResponse{
			{Text: "", ToolCalls: []core.ToolCall{{Name: "echo", Args: map[string]interface{}{"value": "hi"}}}},
			{Text: "all done"},
		},
	}
	registry := toolsys.NewToolRegistry()
	assert.NoError(t, registry.Register(stubTool{name: "echo"}))
	agent := &ReActAgent{
		Model: llm,
		Tools: registry,
	}
	cfg := &core.Config{Model: "test-model", MaxIterations: 3, OllamaToolCalling: true}
	assert.NoError(t, agent.Initialize(cfg))

	task := &core.Task{ID: "task-2", Instruction: "use tool"}
	state := core.NewContext()
	state.Set("task.id", task.ID)

	think := &reactThinkNode{id: "think", agent: agent, task: task}
	act := &reactActNode{id: "act", agent: agent}
	observe := &reactObserveNode{id: "observe", agent: agent, task: task}
	terminal := graph.NewTerminalNode("done")

	graph := graph.NewGraph()
	assert.NoError(t, graph.AddNode(think))
	assert.NoError(t, graph.AddNode(act))
	assert.NoError(t, graph.AddNode(observe))
	assert.NoError(t, graph.AddNode(terminal))
	assert.NoError(t, graph.SetStart("think"))
	assert.NoError(t, graph.AddEdge("think", "act", nil, false))
	assert.NoError(t, graph.AddEdge("act", "observe", nil, false))
	assert.NoError(t, graph.AddEdge("observe", "done", nil, false))

	result, err := graph.Execute(context.Background(), state)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "done", result.NodeID)
	assert.Equal(t, 1, llm.withToolsCalls)

	lastToolRes, ok := state.Get("react.last_tool_result")
	assert.True(t, ok)
	assert.Contains(t, fmt.Sprint(lastToolRes.(map[string]interface{})["echo"]), "hi")

	messagesVal, ok := state.Get("react.messages")
	assert.True(t, ok)
	messages, ok := messagesVal.([]core.Message)
	assert.True(t, ok)
	var toolMessages int
	for _, msg := range messages {
		if msg.Role == "tool" {
			toolMessages++
			assert.Equal(t, "echo", msg.Name)
			assert.Contains(t, msg.Content, "success")
		}
	}
	assert.Equal(t, 1, toolMessages)
}
