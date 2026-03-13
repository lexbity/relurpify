package react

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

const reactMessagesKey = "react.messages"

// getReactMessages reads a copy of the stored chat transcript.
func getReactMessages(state *core.Context) []core.Message {
	raw, ok := state.Get(reactMessagesKey)
	if !ok {
		return nil
	}
	messages, ok := raw.([]core.Message)
	if !ok || len(messages) == 0 {
		return nil
	}
	copyMessages := make([]core.Message, len(messages))
	copy(copyMessages, messages)
	return copyMessages
}

// saveReactMessages overwrites the stored transcript with a defensive copy.
func saveReactMessages(state *core.Context, messages []core.Message) {
	if len(messages) == 0 {
		state.Set(reactMessagesKey, []core.Message{})
		return
	}
	copyMessages := make([]core.Message, len(messages))
	copy(copyMessages, messages)
	state.Set(reactMessagesKey, copyMessages)
}

func appendAssistantMessage(state *core.Context, resp *core.LLMResponse) {
	if state == nil || resp == nil {
		return
	}
	messages := getReactMessages(state)
	if len(messages) == 0 {
		return
	}
	messages = append(messages, core.Message{
		Role:      "assistant",
		Content:   resp.Text,
		ToolCalls: append([]core.ToolCall{}, resp.ToolCalls...),
	})
	saveReactMessages(state, messages)
}

// appendToolMessage records tool responses in the transcript so the LLM can
// observe prior results when tool calling is used.
func appendToolMessage(agent *ReActAgent, task *core.Task, state *core.Context, call core.ToolCall, res *core.ToolResult, envelope *core.CapabilityResultEnvelope) {
	messages := getReactMessages(state)
	if len(messages) == 0 || res == nil {
		return
	}
	content, ok := renderInsertionFilteredSummary(agent, task, call.Name, res, envelope)
	if !ok {
		return
	}
	messages = append(messages, core.Message{
		Role:       "tool",
		Name:       call.Name,
		Content:    fmt.Sprintf("success=%t %s", res.Success, content),
		ToolCallID: call.ID,
	})
	saveReactMessages(state, messages)
}

func getToolObservations(state *core.Context) []ToolObservation {
	if state == nil {
		return nil
	}
	raw, ok := state.Get("react.tool_observations")
	if !ok || raw == nil {
		return nil
	}
	switch values := raw.(type) {
	case []ToolObservation:
		return append([]ToolObservation{}, values...)
	case []any:
		out := make([]ToolObservation, 0, len(values))
		for _, value := range values {
			encoded, err := json.Marshal(value)
			if err != nil {
				continue
			}
			var observation ToolObservation
			if err := json.Unmarshal(encoded, &observation); err == nil {
				out = append(out, observation)
			}
		}
		return out
	default:
		return nil
	}
}

func summarizeToolResult(state *core.Context, call core.ToolCall, res *core.ToolResult) ToolObservation {
	phase := ""
	if state != nil {
		phase = state.GetString("react.phase")
	}
	observation := ToolObservation{
		Tool:      call.Name,
		Phase:     phase,
		Args:      call.Args,
		Success:   res != nil && res.Success,
		Timestamp: time.Now().UTC(),
	}
	if res == nil {
		observation.Summary = fmt.Sprintf("%s returned no result", call.Name)
		return observation
	}
	summary, data := compactToolData(call, res)
	observation.Summary = summary
	observation.Data = data
	return observation
}

func compactToolData(call core.ToolCall, res *core.ToolResult) (string, map[string]interface{}) {
	if res == nil {
		return fmt.Sprintf("%s returned no result", call.Name), nil
	}
	if res.Error != "" {
		stdout := truncateForPrompt(fmt.Sprint(res.Data["stdout"]), 320)
		stderr := truncateForPrompt(fmt.Sprint(res.Data["stderr"]), 320)
		reason := strings.TrimSpace(firstMeaningfulLine(stderr))
		if reason == "" {
			reason = strings.TrimSpace(firstMeaningfulLine(stdout))
		}
		if reason == "" {
			reason = truncateForPrompt(res.Error, 220)
		}
		return fmt.Sprintf("%s failed: %s", call.Name, reason), map[string]interface{}{
			"error":  truncateForPrompt(res.Error, 220),
			"stdout": stdout,
			"stderr": stderr,
		}
	}
	switch call.Name {
	case "file_read":
		path := fmt.Sprint(call.Args["path"])
		content := fmt.Sprint(res.Data["content"])
		snippet := truncateForPrompt(content, 900)
		return fmt.Sprintf("Read %s", path), map[string]interface{}{"path": path, "snippet": snippet}
	case "file_list":
		files := truncateForPrompt(fmt.Sprint(res.Data["files"]), 220)
		return fmt.Sprintf("Listed files: %s", files), map[string]interface{}{"files": files}
	default:
		stdout := truncateForPrompt(fmt.Sprint(res.Data["stdout"]), 320)
		stderr := truncateForPrompt(fmt.Sprint(res.Data["stderr"]), 320)
		if stdout != "" || stderr != "" {
			summary := strings.TrimSpace(strings.Join([]string{firstMeaningfulLine(stderr), firstMeaningfulLine(stdout)}, " | "))
			if summary == "" {
				summary = truncateForPrompt(fmt.Sprintf("stdout=%s stderr=%s", stdout, stderr), 220)
			}
			return fmt.Sprintf("%s: %s", call.Name, summary), map[string]interface{}{"stdout": stdout, "stderr": stderr}
		}
		if len(res.Data) > 0 {
			summary := truncateForPrompt(fmt.Sprint(res.Data), 220)
			return fmt.Sprintf("%s: %s", call.Name, summary), map[string]interface{}{"summary": summary}
		}
		return fmt.Sprintf("%s completed", call.Name), map[string]interface{}{"summary": fmt.Sprintf("%s completed", call.Name)}
	}
}

func firstMeaningfulLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		return truncateForPrompt(line, 180)
	}
	return ""
}

func finalOutputSummary(value interface{}) string {
	switch v := value.(type) {
	case map[string]interface{}:
		return strings.TrimSpace(fmt.Sprint(v["summary"]))
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func reactTaskScope(state *core.Context) string {
	if state == nil {
		return ""
	}
	return state.GetString("task.id")
}
