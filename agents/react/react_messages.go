package react

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

const reactMessagesKey = "react.messages"

// envelopeGet retrieves a value from envelope, checking working memory first, then streamed context.
func envelopeGet(state *contextdata.Envelope, key string) (any, bool) {
	if state == nil {
		return nil, false
	}
	// Check working memory first
	if val, ok := state.GetWorkingValue(key); ok {
		return val, true
	}
	// TODO: Check streamed context references if needed
	return nil, false
}

// envelopeSet stores a value in envelope working memory with task scope.
func envelopeSet(state *contextdata.Envelope, key string, value any) {
	if state == nil {
		return
	}
	state.SetWorkingValue(key, value, contextdata.MemoryClassTask)
}

// envelopeGetString retrieves a value and converts it to string.
func envelopeGetString(state *contextdata.Envelope, key string) string {
	if state == nil {
		return ""
	}
	raw, ok := envelopeGet(state, key)
	if !ok || raw == nil {
		return ""
	}
	return fmt.Sprint(raw)
}

// getWorkingValueAsString retrieves a working value and converts it to string.
func getWorkingValueAsString(state *contextdata.Envelope, key string) string {
	if state == nil {
		return ""
	}
	raw, ok := state.GetWorkingValue(key)
	if !ok || raw == nil {
		return ""
	}
	return fmt.Sprint(raw)
}

// envelopeGetContextForLLM retrieves LLM-formatted context from envelope.
// Assembles streamed context references into a formatted string for LLM consumption.
// Note: ChunkReference contains IDs only; content resolution happens at assembly time.
func envelopeGetContextForLLM(state *contextdata.Envelope) string {
	if state == nil {
		return ""
	}

	var sections []string

	// Add retrieval context if available (QueryText contains the actual query)
	for _, ref := range state.References.Retrieval {
		if ref.QueryText != "" {
			sections = append(sections, fmt.Sprintf("Retrieved: %s", ref.QueryText))
		}
	}

	// Note: StreamedContext ChunkReferences are resolved by the compiler
	// during envelope assembly. The references indicate which chunks were
	// included, but the actual content was already written to working memory
	// or is accessed via the streaming trigger results.

	if len(sections) == 0 {
		return ""
	}

	return strings.Join(sections, "\n\n")
}

// envelopeGetFullHistory retrieves full chat history from envelope working memory.
// Returns compressed summary interactions and full interactions from the envelope.
func envelopeGetFullHistory(state *contextdata.Envelope) ([]CompressedInteraction, []Interaction) {
	if state == nil {
		return nil, nil
	}

	// Retrieve interactions from working memory
	var interactions []Interaction
	if raw, ok := state.GetWorkingValue("_interactions"); ok {
		if arr, ok := raw.([]map[string]any); ok {
			for _, item := range arr {
				role, _ := item["role"].(string)
				content, _ := item["content"].(string)
				if role != "" && content != "" {
					interactions = append(interactions, Interaction{
						Role:    role,
						Content: content,
					})
				}
			}
		}
	}

	// Retrieve compressed history if available
	var compressed []CompressedInteraction
	if raw, ok := state.GetWorkingValue("react.history_compressed"); ok {
		if arr, ok := raw.([]CompressedInteraction); ok {
			compressed = arr
		}
	}

	return compressed, interactions
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
func getReactMessages(state *contextdata.Envelope) []contracts.Message {
	raw, ok := state.GetWorkingValue(reactMessagesKey)
	if !ok {
		return nil
	}
	messages, ok := raw.([]contracts.Message)
	if !ok || len(messages) == 0 {
		return nil
	}
	copyMessages := make([]contracts.Message, len(messages))
	copy(copyMessages, messages)
	return copyMessages
}

// saveReactMessages overwrites the stored transcript with a defensive copy.
func saveReactMessages(state *contextdata.Envelope, messages []contracts.Message) {
	if len(messages) == 0 {
		state.SetWorkingValue(reactMessagesKey, []contracts.Message{}, contextdata.MemoryClassTask)
		return
	}
	copyMessages := make([]contracts.Message, len(messages))
	copy(copyMessages, messages)
	state.SetWorkingValue(reactMessagesKey, copyMessages, contextdata.MemoryClassTask)
}

func appendAssistantMessage(state *contextdata.Envelope, resp *contracts.LLMResponse) {
	if state == nil || resp == nil {
		return
	}
	messages := getReactMessages(state)
	if len(messages) == 0 {
		return
	}
	messages = append(messages, contracts.Message{
		Role:      "assistant",
		Content:   resp.Text,
		ToolCalls: append([]contracts.ToolCall{}, resp.ToolCalls...),
	})
	saveReactMessages(state, messages)
}

// appendToolMessage records tool responses in the transcript so the LLM can
// observe prior results when tool calling is used.
func appendToolMessage(agent *ReActAgent, task *core.Task, state *contextdata.Envelope, call contracts.ToolCall, res *contracts.ToolResult, envelope *core.CapabilityResultEnvelope) {
	messages := getReactMessages(state)
	if len(messages) == 0 || res == nil {
		return
	}
	content, ok := renderInsertionFilteredSummary(agent, task, call.Name, res, envelope)
	if !ok {
		return
	}
	messages = append(messages, contracts.Message{
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
	raw, ok := state.GetWorkingValue("react.tool_observations")
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

func summarizeToolResult(state *contextdata.Envelope, call contracts.ToolCall, res *contracts.ToolResult) ToolObservation {
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
	summary, data := compactToolData(call, res)
	observation.Summary = summary
	observation.Data = data
	return observation
}

func compactToolData(call contracts.ToolCall, res *contracts.ToolResult) (string, map[string]interface{}) {
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

func reactTaskScope(state *contextdata.Envelope) string {
	if state == nil {
		return ""
	}
	return getWorkingValueAsString(state, "task.id")
}
