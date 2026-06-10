package testsuite

import (
	"context"
	"fmt"
	"os"
	"sync"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge/search"
	execution "codeburg.org/lexbit/relurpify/execution"
	graph "codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/model"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

type recordingTelemetry struct {
	mu     sync.Mutex
	events []telemetry.Event
}

func (r *recordingTelemetry) Emit(event telemetry.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *recordingTelemetry) count(eventType telemetry.EventType) int {
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
func (t *integrationFileTool) Parameters() []ports.ToolParameter {
	return []ports.ToolParameter{
		{Name: "path", Type: "string", Description: "file to read"},
	}
}

func (t *integrationFileTool) Execute(ctx context.Context, args map[string]any) (*ports.ToolResult, error) {
	data, err := os.ReadFile(t.path)
	if err != nil {
		return nil, err
	}
	return &ports.ToolResult{
		Success: true,
		Data: map[string]any{
			"status":  "ok",
			"content": string(data),
		},
	}, nil
}

func (t *integrationFileTool) IsAvailable(context.Context) bool { return true }

func (t *integrationFileTool) Permissions() ports.ToolPermissions {
	return ports.ToolPermissions{
		Permissions: policy.NewFileSystemPermissionSet(t.base, permissions.FileSystemRead),
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

func (s *scriptedLLM) Generate(ctx context.Context, prompt string, options *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: s.text}, nil
}

func (s *scriptedLLM) GenerateStream(context.Context, string, *model.LLMOptions) (<-chan string, error) {
	return nil, fmt.Errorf("streaming not supported")
}

func (s *scriptedLLM) Chat(context.Context, []model.Message, *model.LLMOptions) (*model.LLMResponse, error) {
	return nil, fmt.Errorf("chat not supported")
}

func (s *scriptedLLM) ChatWithTools(context.Context, []model.Message, []model.LLMToolSpec, *model.LLMOptions) (*model.LLMResponse, error) {
	return nil, fmt.Errorf("chat tools not supported")
}

type llmPlanNode struct {
	name   string
	model  model.LanguageModel
	prompt string
}

func (n *llmPlanNode) ID() string { return n.name }

func (n *llmPlanNode) Type() graph.NodeType { return graph.NodeTypeSystem }

func (n *llmPlanNode) Execute(ctx context.Context, state *contextdata.Envelope) (*execution.Result, error) {
	if n.model == nil {
		return nil, fmt.Errorf("llm model missing")
	}
	resp, err := n.model.Generate(ctx, n.prompt, nil)
	if err != nil {
		return nil, err
	}
	state.AddInteraction(map[string]any{"actor": "assistant", "content": resp.Text, "node": n.name})
	return &execution.Result{
		NodeID:  n.name,
		Success: true,
		Data: execution.NewToolResultPayload(map[string]any{
			"text": resp.Text,
		}),
	}, nil
}

type toolExecNode struct {
	name string
	tool ports.Tool
	args map[string]any
}

func (n *toolExecNode) ID() string { return n.name }

func (n *toolExecNode) Type() graph.NodeType { return graph.NodeTypeTool }

func (n *toolExecNode) Execute(ctx context.Context, state *contextdata.Envelope) (*execution.Result, error) {
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
	data := make(map[string]any)
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
		state.SetWorkingValueWithClass("use-tool.content", content, contextdata.MemoryClassTask)
	}
	state.AddInteraction(map[string]any{"actor": "tool:" + n.name, "result": data})
	return &execution.Result{
		NodeID:  n.name,
		Success: success,
		Data:    execution.NewToolResultPayload(data),
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

func (n *stateConditionalNode) Execute(ctx context.Context, state *contextdata.Envelope) (*execution.Result, error) {
	if n.decide == nil {
		return nil, fmt.Errorf("conditional missing decision function")
	}
	next, err := n.decide(state)
	if err != nil {
		return nil, err
	}
	return &execution.Result{
		NodeID:  n.name,
		Success: true,
		Data: execution.NewToolResultPayload(map[string]any{
			"next": next,
		}),
	}, nil
}
