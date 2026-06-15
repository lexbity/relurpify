package offline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"codeburg.org/lexbit/relurpify/model"
)

type Model struct{}

func NewModel() Model {
	return Model{}
}

func (Model) Generate(ctx context.Context, prompt string, options *model.LLMOptions) (*model.LLMResponse, error) {
	_ = ctx
	scenario, err := scenarioFromOptions(options)
	if err != nil {
		return nil, err
	}
	return &model.LLMResponse{Text: generateFallbackText(prompt, scenario), FinishReason: "stop"}, nil
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
	case ScenarioFileList:
		return fileListScenarioResponse(scenario, messages), nil
	case ScenarioSearchGrep:
		return searchGrepScenarioResponse(scenario, messages), nil
	case ScenarioFileEdit:
		return fileEditScenarioResponse(scenario, messages), nil
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

func fileListScenarioResponse(scenario Scenario, messages []model.Message) *model.LLMResponse {
	dir := strings.TrimSpace(scenario.ToolArg)
	if dir == "" {
		dir = "."
	}
	switch countToolMessages(messages) {
	case 0:
		return toolCallResponse("file_list", map[string]any{"directory": dir, "pattern": "*"})
	case 1:
		return toolCallResponse("file_read", map[string]any{"path": fileListReadPath(dir)})
	default:
		if content, ok := lastToolMessage(messages); ok {
			return &model.LLMResponse{
				Text:         fmt.Sprintf("offline file_list result: %s", content),
				FinishReason: "stop",
			}
		}
		return &model.LLMResponse{
			Text:         "offline file_list result: traversal complete",
			FinishReason: "stop",
		}
	}
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

func fileListReadPath(dir string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" || dir == "." {
		return "target.txt"
	}
	return strings.TrimSuffix(dir, "/") + "/target.txt"
}

func searchGrepScenarioResponse(scenario Scenario, messages []model.Message) *model.LLMResponse {
	pattern, directory := searchGrepScenarioArgs(scenario.ToolArg)
	switch countToolMessages(messages) {
	case 0:
		return toolCallResponse("search_grep", map[string]any{"pattern": pattern, "directory": directory})
	default:
		if content, ok := lastToolMessage(messages); ok {
			return &model.LLMResponse{
				Text:         fmt.Sprintf("offline search_grep result: %s", content),
				FinishReason: "stop",
			}
		}
		return &model.LLMResponse{Text: "offline search_grep result: grep complete", FinishReason: "stop"}
	}
}

func searchGrepScenarioArgs(raw string) (pattern, directory string) {
	raw = strings.TrimSpace(raw)
	pattern = raw
	directory = "."
	if raw == "" {
		return pattern, directory
	}
	if left, right, ok := strings.Cut(raw, "|"); ok {
		pattern = strings.TrimSpace(left)
		directory = strings.TrimSpace(right)
		if directory == "" {
			directory = "."
		}
	}
	return pattern, directory
}

func fileEditScenarioResponse(scenario Scenario, messages []model.Message) *model.LLMResponse {
	path, oldString, newString, expectedCount := fileEditScenarioArgs(scenario.ToolArg)
	switch countToolMessages(messages) {
	case 0:
		return toolCallResponse("file_edit", map[string]any{
			"path":           path,
			"old_string":     oldString,
			"new_string":     newString,
			"expected_count": expectedCount,
		})
	default:
		if content, ok := lastToolMessage(messages); ok {
			return &model.LLMResponse{
				Text:         fmt.Sprintf("offline file_edit result: %s", content),
				FinishReason: "stop",
			}
		}
		return &model.LLMResponse{Text: "offline file_edit result: edit complete", FinishReason: "stop"}
	}
}

func fileEditScenarioArgs(raw string) (path, oldString, newString string, expectedCount int) {
	raw = strings.TrimSpace(raw)
	expectedCount = 1
	if raw == "" {
		return "", "", "", expectedCount
	}
	parts := strings.SplitN(raw, "|", 4)
	if len(parts) < 3 {
		return raw, "", "", expectedCount
	}
	path = strings.TrimSpace(parts[0])
	oldString = strings.TrimSpace(parts[1])
	newString = strings.TrimSpace(parts[2])
	if len(parts) == 4 {
		if parsed, err := strconv.Atoi(strings.TrimSpace(parts[3])); err == nil {
			expectedCount = parsed
		}
	}
	return path, oldString, newString, expectedCount
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

func generateFallbackText(prompt string, scenario Scenario) string {
	switch scenario.Kind {
	case ScenarioFileRead:
		if content, ok := lastFallbackToolContent(prompt); ok {
			return fmt.Sprintf("offline file_read result: %s", content)
		}
		return fallbackToolJSON("file_read", map[string]any{"path": scenario.ToolArg})
	case ScenarioFileList:
		dir := strings.TrimSpace(scenario.ToolArg)
		if dir == "" {
			dir = "."
		}
		switch countFallbackToolSections(prompt) {
		case 0:
			return fallbackToolJSON("file_list", map[string]any{"directory": dir, "pattern": "*"})
		case 1:
			return fallbackToolJSON("file_read", map[string]any{"path": fileListReadPath(dir)})
		default:
			if content, ok := lastFallbackToolContent(prompt); ok {
				return fmt.Sprintf("offline file_list result: %s", content)
			}
			return "offline file_list result: traversal complete"
		}
	case ScenarioSearchGrep:
		pattern, directory := searchGrepScenarioArgs(scenario.ToolArg)
		switch countFallbackToolSections(prompt) {
		case 0:
			return fallbackToolJSON("search_grep", map[string]any{"pattern": pattern, "directory": directory})
		default:
			if content, ok := lastFallbackToolContent(prompt); ok {
				return fmt.Sprintf("offline search_grep result: %s", content)
			}
			return "offline search_grep result: grep complete"
		}
	case ScenarioFileEdit:
		path, oldString, newString, expectedCount := fileEditScenarioArgs(scenario.ToolArg)
		switch countFallbackToolSections(prompt) {
		case 0:
			return fallbackToolJSON("file_edit", map[string]any{
				"path":           path,
				"old_string":     oldString,
				"new_string":     newString,
				"expected_count": expectedCount,
			})
		default:
			if content, ok := lastFallbackToolContent(prompt); ok {
				return fmt.Sprintf("offline file_edit result: %s", content)
			}
			return "offline file_edit result: edit complete"
		}
	case ScenarioExecRunCode:
		if content, ok := lastFallbackToolContent(prompt); ok {
			return fmt.Sprintf("offline exec_run_code result: %s", content)
		}
		return fallbackToolJSON("exec_run_code", map[string]any{"command": scenario.ToolArg})
	case ScenarioCliGit:
		if content, ok := lastFallbackToolContent(prompt); ok {
			return fmt.Sprintf("offline cli_git result: %s", content)
		}
		return fallbackToolJSON("cli_git", map[string]any{"args": strings.Fields(scenario.ToolArg)})
	case ScenarioHITL:
		if content, ok := lastFallbackToolContent(prompt); ok {
			return fmt.Sprintf("offline approval result: %s", content)
		}
		return fallbackToolJSON("approval", map[string]any{"reason": "hitl"})
	case ScenarioMulti:
		if countFallbackToolSections(prompt) >= scenario.MultiStep {
			return fmt.Sprintf("offline multi complete (%d/%d)", scenario.MultiStep, scenario.MultiStep)
		}
		name := fmt.Sprintf("multi_%d", countFallbackToolSections(prompt)+1)
		return fallbackToolJSON(name, map[string]any{"step": countFallbackToolSections(prompt) + 1})
	case ScenarioEcho:
		if msg := lastUserPromptText(prompt); msg != "" {
			return msg
		}
		return greetingText()
	case ScenarioGreeting:
		fallthrough
	default:
		return greetingText()
	}
}

func fallbackToolJSON(name string, args map[string]any) string {
	payload, err := json.Marshal(map[string]any{
		"tool":      name,
		"arguments": args,
	})
	if err != nil {
		payload = []byte(fmt.Sprintf(`{"tool":%q,"arguments":{}}`, name))
	}
	return "```tool\n" + string(payload) + "\n```"
}

func countFallbackToolSections(prompt string) int {
	count := 0
	for _, line := range strings.Split(prompt, "\n") {
		if strings.EqualFold(strings.TrimSpace(line), "[tool]") {
			count++
		}
	}
	return count
}

func lastFallbackToolContent(prompt string) (string, bool) {
	lines := strings.Split(prompt, "\n")
	inTool := false
	var b strings.Builder
	last := ""
	for _, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		switch {
		case strings.EqualFold(strings.TrimSpace(line), "Available tools:"),
			strings.EqualFold(strings.TrimSpace(line), "Conversation policy:"):
			if inTool {
				last = strings.TrimSpace(b.String())
			}
			inTool = false
		case strings.EqualFold(strings.TrimSpace(line), "[tool]"):
			if inTool {
				last = strings.TrimSpace(b.String())
			}
			inTool = true
			b.Reset()
		case strings.HasPrefix(strings.TrimSpace(line), "[") && strings.HasSuffix(strings.TrimSpace(line), "]"):
			if inTool {
				last = strings.TrimSpace(b.String())
			}
			inTool = false
		default:
			if inTool {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
				b.WriteString(line)
			}
		}
	}
	if inTool {
		last = strings.TrimSpace(b.String())
	}
	if last != "" {
		return last, true
	}
	return "", false
}

func lastUserPromptText(prompt string) string {
	lines := strings.Split(prompt, "\n")
	inUser := false
	var b strings.Builder
	last := ""
	for _, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		switch {
		case strings.EqualFold(strings.TrimSpace(line), "Available tools:"),
			strings.EqualFold(strings.TrimSpace(line), "Conversation policy:"):
			if inUser {
				last = strings.TrimSpace(b.String())
			}
			return last
		case strings.EqualFold(strings.TrimSpace(line), "[user]"):
			inUser = true
			b.Reset()
		case strings.HasPrefix(strings.TrimSpace(line), "[") && strings.HasSuffix(strings.TrimSpace(line), "]"):
			if inUser {
				last = strings.TrimSpace(b.String())
			}
			inUser = false
		default:
			if inUser {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
				b.WriteString(line)
			}
		}
	}
	if inUser {
		last = strings.TrimSpace(b.String())
	}
	return last
}
