package react

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/capability/ports"
	capresult "codeburg.org/lexbit/relurpify/capability/result"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/model"
)

const reactMessagesKey = "react.messages"

// envelopeGet retrieves a value from envelope, checking working memory first, then streamed context.
func envelopeGet(state *contextdata.Envelope, key string) (any, bool) {
	if state == nil {
		return nil, false
	}
	// Check working memory first
	if val, ok := contextdata.GetTyped[any](state, key); ok {
		return val, true
	}
	return nil, false
}

// getWorkingValueAsString retrieves a working value and converts it to string.
func getWorkingValueAsString(state *contextdata.Envelope, key string) string {
	if state == nil {
		return ""
	}
	raw, ok := contextdata.GetTyped[any](state, key)
	if !ok || raw == nil {
		return ""
	}
	return fmt.Sprint(raw)
}

// CompressedInteraction represents a compressed chat interaction.
type CompressedInteraction struct {
	Summary string
}

// Interaction represents a chat interaction.
type Interaction struct {
	Role    string
	Content string
}

// getReactMessages reads a copy of the stored chat transcript.
func getReactMessages(state *contextdata.Envelope) []model.Message {
	raw, ok := contextdata.GetTyped[any](state, reactMessagesKey)
	if !ok {
		return nil
	}
	messages, ok := raw.([]model.Message)
	if !ok || len(messages) == 0 {
		return nil
	}
	copyMessages := make([]model.Message, len(messages))
	copy(copyMessages, messages)
	return copyMessages
}

// saveReactMessages overwrites the stored transcript with a defensive copy.
func saveReactMessages(state *contextdata.Envelope, messages []model.Message) {
	if len(messages) == 0 {
		state.SetWorkingValueWithClass(reactMessagesKey, []model.Message{}, contextdata.MemoryClassTask)
		return
	}
	copyMessages := make([]model.Message, len(messages))
	copy(copyMessages, messages)
	state.SetWorkingValueWithClass(reactMessagesKey, copyMessages, contextdata.MemoryClassTask)
}

func appendAssistantMessage(state *contextdata.Envelope, resp *model.LLMResponse) {
	if state == nil || resp == nil {
		return
	}
	messages := getReactMessages(state)
	if len(messages) == 0 {
		return
	}
	messages = append(messages, model.Message{
		Role:      "assistant",
		Content:   resp.Text,
		ToolCalls: append([]model.ToolCall{}, resp.ToolCalls...),
	})
	saveReactMessages(state, messages)
}

// appendToolMessage records tool responses in the transcript so the LLM can
// observe prior results when tool calling is used.
func appendToolMessage(agent *ReActAgent, task *execution.Task, state *contextdata.Envelope, call model.ToolCall, res *ports.ToolResult, envelope *capresult.CapabilityResultEnvelope) {
	messages := getReactMessages(state)
	if len(messages) == 0 || res == nil {
		return
	}
	content, ok := renderInsertionFilteredSummary(agent, task, call.Name, res, envelope)
	if !ok {
		return
	}
	messages = append(messages, model.Message{
		Role:       "tool",
		Name:       call.Name,
		Content:    fmt.Sprintf("success=%t %s", res.Success, content),
		ToolCallID: call.ID,
	})
	saveReactMessages(state, messages)
}

func getToolObservations(state *contextdata.Envelope) []ToolObservation {
	if state == nil {
		return nil
	}
	raw, ok := contextdata.GetTyped[any](state, "react.tool_observations")
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

func summarizeToolResult(state *contextdata.Envelope, call model.ToolCall, res *ports.ToolResult, decision capresult.InsertionDecision) ToolObservation {
	phase := ""
	if state != nil {
		phase = getWorkingValueAsString(state, "react.phase")
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
	summary, data := renderToolData(call, res)
	observation.Summary = summary
	observation.Data = data
	return observation
}

// renderToolData renders a tool result at full fidelity, generically: the
// tool's own structured result speaks for itself — the agent hard-codes no
// tool shapes. Size is bounded only by the envelope's state-size contracts.
func renderToolData(call model.ToolCall, res *ports.ToolResult) (string, map[string]any) {
	if res == nil {
		return fmt.Sprintf("%s returned no result", call.Name), nil
	}
	if res.Error != "" {
		return fmt.Sprintf("%s failed: %s", call.Name, res.Error), map[string]any{
			"error":  res.Error,
			"data":   res.Data,
			"stdout": fmt.Sprint(res.Data["stdout"]),
			"stderr": fmt.Sprint(res.Data["stderr"]),
		}
	}
	if len(res.Data) == 0 {
		return fmt.Sprintf("%s completed", call.Name), nil
	}
	return fmt.Sprintf("%s: %s", call.Name, firstMeaningfulLine(fmt.Sprint(res.Data))), res.Data
}

func firstMeaningfulLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		return trimToBudget(line, 180)
	}
	return ""
}

func trimToBudget(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	return strings.TrimSpace(value[:max]) + "..."
}

// truncateForPrompt was removed. Use trimToBudget for caller-side budget
// limits, or let the InsertionDecision gate what the model sees.

func toolNames(tools []ports.Tool) []string {
	out := make([]string, 0, len(tools))
	for _, tool := range tools {
		out = append(out, tool.Name())
	}
	sort.Strings(out)
	return out
}

func summarizeToolPayload(result *ports.ToolResult) string {
	if result == nil {
		return ""
	}
	if summary, ok := result.Data["summary"].(string); ok && summary != "" {
		return summary
	}
	if result.Error != "" {
		return result.Error
	}
	return fmt.Sprint(result.Data)
}

func toolSummaryBudgetForPhase(phase string) int {
	switch phase {
	case contextmgrPhaseVerify:
		return 6
	case contextmgrPhaseEdit:
		return 4
	default:
		return 5
	}
}
