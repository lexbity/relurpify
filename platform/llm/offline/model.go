package offline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/model"
)

type Model struct{}

func NewModel() Model {
	return Model{}
}

func (Model) Generate(ctx context.Context, prompt string, options *model.LLMOptions) (*model.LLMResponse, error) {
	_ = ctx
	_ = prompt
	_ = options
	return &model.LLMResponse{Text: greetingText(), FinishReason: "stop"}, nil
}

func (Model) GenerateStream(ctx context.Context, prompt string, options *model.LLMOptions) (<-chan string, error) {
	resp, err := (Model{}).Generate(ctx, prompt, options)
	if err != nil {
		return nil, err
	}
	ch := make(chan string, 1)
	ch <- resp.Text
	close(ch)
	return ch, nil
}

func (Model) Chat(ctx context.Context, messages []model.Message, options *model.LLMOptions) (*model.LLMResponse, error) {
	return respond(ctx, messages, nil, options)
}

func (Model) ChatWithTools(ctx context.Context, messages []model.Message, tools []model.LLMToolSpec, options *model.LLMOptions) (*model.LLMResponse, error) {
	return respond(ctx, messages, tools, options)
}

func respond(ctx context.Context, messages []model.Message, tools []model.LLMToolSpec, options *model.LLMOptions) (*model.LLMResponse, error) {
	_ = ctx
	_ = tools
	scenario, err := scenarioFromOptions(options)
	if err != nil {
		return nil, err
	}
	switch scenario.Kind {
	case ScenarioGreeting:
		return &model.LLMResponse{Text: greetingText(), FinishReason: "stop"}, nil
	case ScenarioEcho:
		return echoResponse(messages), nil
	case ScenarioFileRead:
		return toolScenarioResponse("file_read", map[string]any{"path": scenario.ToolArg}, messages, "file_read"), nil
	case ScenarioExecRunCode:
		return toolScenarioResponse("exec_run_code", map[string]any{"command": scenario.ToolArg}, messages, "exec_run_code"), nil
	case ScenarioCliGit:
		return toolScenarioResponse("cli_git", map[string]any{"args": strings.Fields(scenario.ToolArg)}, messages, "cli_git"), nil
	case ScenarioHITL:
		return toolScenarioResponse("approval", map[string]any{"reason": "hitl"}, messages, "hitl"), nil
	case ScenarioMulti:
		return multiResponse(messages, scenario.MultiStep), nil
	case ScenarioError:
		return nil, fmt.Errorf("offline scenario error")
	default:
		return nil, fmt.Errorf("unknown offline scenario %q", scenario.Kind)
	}
}

func scenarioFromOptions(options *model.LLMOptions) (Scenario, error) {
	if options == nil || len(options.Config) == 0 {
		return ParseScenario(nil)
	}
	return ParseScenario(options.Config["offline_scenario"])
}

func greetingText() string {
	return "euclo online (offline backend)."
}

func echoResponse(messages []model.Message) *model.LLMResponse {
	if msg := lastUserMessage(messages); msg != "" {
		return &model.LLMResponse{Text: msg, FinishReason: "stop"}
	}
	return &model.LLMResponse{Text: greetingText(), FinishReason: "stop"}
}

func toolScenarioResponse(name string, args map[string]any, messages []model.Message, label string) *model.LLMResponse {
	if content, ok := lastToolMessage(messages); ok {
		return &model.LLMResponse{
			Text:         fmt.Sprintf("offline %s result: %d bytes", label, len(content)),
			FinishReason: "stop",
		}
	}
	return toolCallResponse(name, args)
}

func toolCallResponse(name string, args map[string]any) *model.LLMResponse {
	return &model.LLMResponse{
		FinishReason: "tool_calls",
		ToolCalls: []model.ToolCall{{
			ID:   toolCallID(name, args),
			Name: name,
			Args: args,
		}},
	}
}

func multiResponse(messages []model.Message, stepCount int) *model.LLMResponse {
	if stepCount <= 0 {
		return &model.LLMResponse{Text: greetingText(), FinishReason: "stop"}
	}
	completed := countToolMessages(messages)
	if completed >= stepCount {
		return &model.LLMResponse{Text: fmt.Sprintf("offline multi complete (%d/%d)", completed, stepCount), FinishReason: "stop"}
	}
	name := fmt.Sprintf("multi_%d", completed+1)
	return toolCallResponse(name, map[string]any{"step": completed + 1})
}

func lastUserMessage(messages []model.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.EqualFold(strings.TrimSpace(messages[i].Role), "user") {
			return strings.TrimSpace(messages[i].Content)
		}
	}
	return ""
}

func lastToolMessage(messages []model.Message) (string, bool) {
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.EqualFold(strings.TrimSpace(messages[i].Role), "tool") {
			return strings.TrimSpace(messages[i].Content), true
		}
	}
	return "", false
}

func countToolMessages(messages []model.Message) int {
	count := 0
	for _, msg := range messages {
		if strings.EqualFold(strings.TrimSpace(msg.Role), "tool") {
			count++
		}
	}
	return count
}

func toolCallID(name string, args map[string]any) string {
	payload, err := json.Marshal(args)
	if err != nil {
		payload = []byte(fmt.Sprint(args))
	}
	sum := sha256.Sum256([]byte(name + ":" + string(payload)))
	return hex.EncodeToString(sum[:8])
}
