package testutil

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
)

var _ core.LanguageModel = ErrorModel{}
var _ core.Tool = EchoTool{}
var _ graph.WorkflowExecutor = (*NoopExecutor)(nil)

type ErrorModel struct {
	Err error
}

func (m ErrorModel) Generate(context.Context, string, *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, m.Err
}

func (m ErrorModel) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	return nil, m.Err
}

func (m ErrorModel) Chat(context.Context, []core.Message, *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, m.Err
}

func (m ErrorModel) ChatWithTools(context.Context, []core.Message, []core.LLMToolSpec, *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, m.Err
}

type EchoTool struct {
	ToolName string
}

func (t EchoTool) Name() string {
	if t.ToolName != "" {
		return t.ToolName
	}
	return "echo"
}

func (t EchoTool) Description() string { return "echoes the first provided argument" }
func (t EchoTool) Category() string    { return "test" }
func (t EchoTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{{Name: "value", Type: "string", Required: false}}
}

func (t EchoTool) Execute(_ context.Context, _ *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	var echo interface{}
	if value, ok := args["value"]; ok {
		echo = value
	} else {
		for _, value := range args {
			echo = value
			break
		}
	}
	if echo == nil {
		echo = ""
	}
	return &core.ToolResult{Success: true, Data: map[string]interface{}{"echo": echo}}, nil
}

func (t EchoTool) IsAvailable(context.Context, *core.Context) bool { return true }
func (t EchoTool) Permissions() core.ToolPermissions               { return core.ToolPermissions{} }
func (t EchoTool) Tags() []string                                  { return nil }

type NoopExecutor struct {
	Calls int
	Tasks []*core.Task
}

func (e *NoopExecutor) Initialize(*core.Config) error { return nil }
func (e *NoopExecutor) Capabilities() []core.Capability {
	return nil
}

func (e *NoopExecutor) BuildGraph(*core.Task) (*graph.Graph, error) {
	g := graph.NewGraph()
	done := graph.NewTerminalNode("done")
	if err := g.AddNode(done); err != nil {
		return nil, err
	}
	if err := g.SetStart(done.ID()); err != nil {
		return nil, err
	}
	return g, nil
}

func (e *NoopExecutor) Execute(_ context.Context, task *core.Task, _ *core.Context) (*core.Result, error) {
	e.Calls++
	e.Tasks = append(e.Tasks, task)
	return &core.Result{Success: true, Data: map[string]any{}}, nil
}
