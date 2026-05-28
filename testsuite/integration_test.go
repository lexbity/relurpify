package testsuite

import (
	"context"
	"fmt"
	"os"
	"sync"

	graph "codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/search"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

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
		Data: core.NewToolResultPayload(map[string]any{
			"text": resp.Text,
		}),
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
		Data:    core.NewToolResultPayload(data),
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
		Data: core.NewToolResultPayload(map[string]any{
			"next": next,
		}),
	}, nil
}
