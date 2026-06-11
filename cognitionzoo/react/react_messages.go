package react

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
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
	if val, ok := state.GetWorkingValue(key); ok {
		return val, true
	}
	// TODO: Check streamed context references if needed
	return nil, false
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
	raw, ok := state.GetWorkingValue(reactMessagesKey)
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
	summary, data := compactToolData(call, res, decision)
	observation.Summary = summary
	observation.Data = data
	return observation
}

// clipSizeForDecision returns the max characters for tool output in
// observations, based on the insertion decision. Direct → generous budget,
// Summarized → moderate, MetadataOnly → minimal.
func clipSizeForDecision(decision capresult.InsertionDecision) int {
	switch decision.Action {
	case agentspec.InsertionActionDirect:
		return 4000
	case agentspec.InsertionActionSummarized:
		return 900
	case agentspec.InsertionActionMetadataOnly:
		return 120
	default:
		return 320
	}
}

func compactToolData(call model.ToolCall, res *ports.ToolResult, decision capresult.InsertionDecision) (string, map[string]any) {
	if res == nil {
		return fmt.Sprintf("%s returned no result", call.Name), nil
	}
	maxClip := clipSizeForDecision(decision)
	if res.Error != "" {
		stdout := trimToBudget(fmt.Sprint(res.Data["stdout"]), maxClip)
		stderr := trimToBudget(fmt.Sprint(res.Data["stderr"]), maxClip)
		reason := strings.TrimSpace(firstMeaningfulLine(stderr))
		if reason == "" {
			reason = strings.TrimSpace(firstMeaningfulLine(stdout))
		}
		if reason == "" {
			reason = trimToBudget(res.Error, maxClip)
		}
		return fmt.Sprintf("%s failed: %s", call.Name, reason), map[string]any{
			"error":  trimToBudget(res.Error, maxClip),
			"stdout": stdout,
			"stderr": stderr,
		}
	}
	switch call.Name {
	case "file_read":
		path := fmt.Sprint(call.Args["path"])
		content := fmt.Sprint(res.Data["content"])
		snippet := trimToBudget(content, maxClip*3)
		return fmt.Sprintf("Read %s", path), map[string]any{"path": path, "snippet": snippet}
	case "file_list":
		files := trimToBudget(fmt.Sprint(res.Data["files"]), maxClip)
		return fmt.Sprintf("Listed files: %s", files), map[string]any{"files": files}
	default:
		stdout := trimToBudget(fmt.Sprint(res.Data["stdout"]), maxClip)
		stderr := trimToBudget(fmt.Sprint(res.Data["stderr"]), maxClip)
		if stdout != "" || stderr != "" {
			summary := strings.TrimSpace(strings.Join([]string{firstMeaningfulLine(stderr), firstMeaningfulLine(stdout)}, " | "))
			if summary == "" {
				summary = trimToBudget(fmt.Sprintf("stdout=%s stderr=%s", stdout, stderr), maxClip)
			}
			return fmt.Sprintf("%s: %s", call.Name, summary), map[string]any{"stdout": stdout, "stderr": stderr}
		}
		if len(res.Data) > 0 {
			summary := trimToBudget(fmt.Sprint(res.Data), maxClip)
			return fmt.Sprintf("%s: %s", call.Name, summary), map[string]any{"summary": summary}
		}
		return fmt.Sprintf("%s completed", call.Name), map[string]any{"summary": fmt.Sprintf("%s completed", call.Name)}
	}
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

// truncateForPrompt is removed in Phase 7. Use trimToBudget for caller-side budget
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
	return trimToBudget(fmt.Sprint(result.Data), 220)
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
