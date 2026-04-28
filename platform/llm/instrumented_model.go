package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// ProfiledModel is re-exported from contracts
type ProfiledModel = contracts.ProfiledModel

// Telemetry is re-exported from contracts
type Telemetry = contracts.Telemetry

// InstrumentedModel wraps a LanguageModel and emits telemetry for prompts and responses.
type InstrumentedModel struct {
	Inner     LanguageModel
	Telemetry Telemetry
	Debug     bool
}

func NewInstrumentedModel(inner LanguageModel, telemetry Telemetry, debug bool) *InstrumentedModel {
	return &InstrumentedModel{Inner: inner, Telemetry: telemetry, Debug: debug}
}

func (m *InstrumentedModel) Generate(ctx context.Context, prompt string, options *LLMOptions) (*LLMResponse, error) {
	m.emitPrompt(ctx, "generate", map[string]interface{}{
		"model":          modelFromOptions(options),
		"prompt_chars":   len(prompt),
		"prompt_preview": clip(prompt, 1024),
	}, m.Debug, map[string]interface{}{"prompt": clip(prompt, 8192)})
	resp, err := m.Inner.Generate(ctx, prompt, options)
	m.emitResponse(ctx, "generate", resp, err)
	return resp, err
}

func (m *InstrumentedModel) GenerateStream(ctx context.Context, prompt string, options *LLMOptions) (<-chan string, error) {
	m.emitPrompt(ctx, "generate_stream", map[string]interface{}{
		"model":          modelFromOptions(options),
		"prompt_chars":   len(prompt),
		"prompt_preview": clip(prompt, 1024),
	}, m.Debug, map[string]interface{}{"prompt": clip(prompt, 8192)})
	ch, err := m.Inner.GenerateStream(ctx, prompt, options)
	// For stream, we only emit that a stream started; callers can still see tool calls/results via other telemetry.
	if err != nil {
		m.emitResponse(ctx, "generate_stream", nil, err)
	} else {
		m.emitResponse(ctx, "generate_stream", &LLMResponse{FinishReason: "stream"}, nil)
	}
	return ch, err
}

func (m *InstrumentedModel) Chat(ctx context.Context, messages []Message, options *LLMOptions) (*LLMResponse, error) {
	meta := chatMeta(messages, nil, options)
	m.emitPrompt(ctx, "chat", meta.base, m.Debug, meta.debug)
	resp, err := m.Inner.Chat(ctx, messages, options)
	m.emitResponse(ctx, "chat", resp, err)
	return resp, err
}

func (m *InstrumentedModel) ChatWithTools(ctx context.Context, messages []Message, tools []LLMToolSpec, options *LLMOptions) (*LLMResponse, error) {
	meta := chatMeta(messages, tools, options)
	m.emitPrompt(ctx, "chat_with_tools", meta.base, m.Debug, meta.debug)
	resp, err := m.Inner.ChatWithTools(ctx, messages, tools, options)
	m.emitResponse(ctx, "chat_with_tools", resp, err)
	return resp, err
}

// SetProfile forwards a resolved model profile to the wrapped model when it
// supports profile mutation.
func (m *InstrumentedModel) SetProfile(profile *ModelProfile) {
	if m == nil || m.Inner == nil || profile == nil {
		return
	}
	if setter, ok := m.Inner.(interface{ SetProfile(*ModelProfile) }); ok {
		setter.SetProfile(profile)
	}
}

// ToolRepairStrategy implements ProfiledModel when the wrapped model
// exposes profile metadata.
func (m *InstrumentedModel) ToolRepairStrategy() string {
	if m != nil {
		if profiled, ok := m.Inner.(ProfiledModel); ok {
			return profiled.ToolRepairStrategy()
		}
	}
	return "heuristic-only"
}

// MaxToolsPerCall implements ProfiledModel when the wrapped model
// exposes profile metadata.
func (m *InstrumentedModel) MaxToolsPerCall() int {
	if m != nil {
		if profiled, ok := m.Inner.(ProfiledModel); ok {
			return profiled.MaxToolsPerCall()
		}
	}
	return 0
}

// UsesNativeToolCalling implements ProfiledModel when the wrapped model
// exposes profile metadata.
func (m *InstrumentedModel) UsesNativeToolCalling() bool {
	if m != nil {
		if profiled, ok := m.Inner.(ProfiledModel); ok {
			return profiled.UsesNativeToolCalling()
		}
	}
	return false
}

type chatMetaPayload struct {
	base  map[string]interface{}
	debug map[string]interface{}
}

func chatMeta(messages []Message, tools []LLMToolSpec, options *LLMOptions) chatMetaPayload {
	var roles []string
	preview := make([]map[string]interface{}, 0, min(len(messages), 20))
	for i, msg := range messages {
		if i >= 20 {
			break
		}
		roles = append(roles, msg.Role)
		preview = append(preview, map[string]interface{}{
			"role":    msg.Role,
			"name":    msg.Name,
			"content": clip(msg.Content, 512),
		})
	}
	toolNames := make([]string, 0, len(tools))
	for _, t := range tools {
		toolNames = append(toolNames, t.Name)
	}
	base := map[string]interface{}{
		"model":            modelFromOptions(options),
		"message_count":    len(messages),
		"roles":            roles,
		"messages_preview": preview,
		"tool_count":       len(tools),
		"tool_names":       toolNames,
	}
	debug := map[string]interface{}{}
	if len(messages) > 0 {
		full := make([]map[string]interface{}, 0, len(messages))
		for _, msg := range messages {
			full = append(full, map[string]interface{}{
				"role":    msg.Role,
				"name":    msg.Name,
				"content": clip(msg.Content, 8192),
			})
		}
		debug["messages"] = full
	}
	if len(tools) > 0 {
		debug["tools"] = toolNames
	}
	return chatMetaPayload{base: base, debug: debug}
}

func (m *InstrumentedModel) emitPrompt(ctx context.Context, kind string, base map[string]interface{}, debug bool, debugFields map[string]interface{}) {
	if m == nil || m.Telemetry == nil {
		return
	}
	taskID, taskMeta := taskInfo(ctx)
	metadata := map[string]interface{}{
		"kind": kind,
	}
	for k, v := range base {
		metadata[k] = v
	}
	for k, v := range taskMeta {
		metadata[k] = v
	}
	if debug {
		for k, v := range debugFields {
			metadata[k] = v
		}
	}
	m.Telemetry.Emit(contracts.Event{
		Type:      contracts.EventLLMPrompt,
		TaskID:    taskID,
		Timestamp: time.Now().UTC(),
		Message:   fmt.Sprintf("llm %s prompt", kind),
		Metadata:  metadata,
	})
}

func (m *InstrumentedModel) emitResponse(ctx context.Context, kind string, resp *LLMResponse, err error) {
	if m == nil || m.Telemetry == nil {
		return
	}
	taskID, taskMeta := taskInfo(ctx)
	metadata := map[string]interface{}{
		"kind": kind,
	}
	for k, v := range taskMeta {
		metadata[k] = v
	}
	if resp != nil {
		metadata["finish_reason"] = resp.FinishReason
		metadata["text_preview"] = clip(resp.Text, 1024)
		metadata["usage"] = resp.Usage
		if len(resp.ToolCalls) > 0 {
			toolCalls, _ := json.Marshal(resp.ToolCalls)
			metadata["tool_calls"] = string(toolCalls)
		}
	}
	if err != nil {
		metadata["error"] = err.Error()
	}
	m.Telemetry.Emit(contracts.Event{
		Type:      contracts.EventLLMResponse,
		TaskID:    taskID,
		Timestamp: time.Now().UTC(),
		Message:   fmt.Sprintf("llm %s response", kind),
		Metadata:  metadata,
	})
}

func modelFromOptions(options *LLMOptions) string {
	if options != nil && options.Model != "" {
		return options.Model
	}
	return ""
}

func taskInfo(ctx context.Context) (string, map[string]interface{}) {
	// Task context extraction requires framework/core.TaskContextFrom
	// For now, return empty values to break the import cycle
	return "", nil
}

func clip(s string, max int) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
