package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/platform/observability"
)

// stampObservabilityCorrelation populates every correlation field on an
// observability.Event from ctx — including TaskID. It is the single
// sanctioned way to correlate LLM events (NFR-6): emitters must never
// construct correlation fields by hand, and must never smuggle them
// through Metadata.
//
// Merge semantics come from observability.StampCorrelation (single source
// of truth). The fallbacks resolved here are for the fields only the
// envelope knows — SessionID, NodeID, and TaskID — because the envelope's
// private context key is only reachable via the contextdata accessor,
// which keeps the platform/llm → execution import cycle broken.
func stampObservabilityCorrelation(ctx context.Context, ev *observability.Event) {
	if ev == nil {
		return
	}
	observability.StampCorrelation(ctx, ev)
	if env, ok := contextdata.EnvelopeFrom(ctx); ok {
		if ev.NodeID == "" && env.NodeIDSnapshot() != "" {
			ev.NodeID = env.NodeIDSnapshot()
		}
		if ev.SessionID == "" && env.SessionIDSnapshot() != "" {
			ev.SessionID = env.SessionIDSnapshot()
		}
		if ev.TaskID == "" && env.TaskIDSnapshot() != "" {
			ev.TaskID = env.TaskIDSnapshot()
		}
	}
}

// ProfiledModel is re-exported from contracts
type ProfiledModel = model.ProfiledModel

// InstrumentedModel wraps a LanguageModel and emits telemetry for prompts and responses.
type InstrumentedModel struct {
	Inner     LanguageModel
	Telemetry observability.Telemetry
	Debug     bool
}

func NewInstrumentedModel(inner LanguageModel, telemetry observability.Telemetry, debug bool) *InstrumentedModel {
	return &InstrumentedModel{Inner: inner, Telemetry: telemetry, Debug: debug}
}

func (m *InstrumentedModel) Generate(ctx context.Context, prompt string, options *LLMOptions) (*LLMResponse, error) {
	m.emitPrompt(ctx, "generate", map[string]any{
		"model":          modelFromOptions(options),
		"prompt_chars":   len(prompt),
		"prompt_preview": clip(prompt, 1024),
	}, m.Debug, map[string]any{"prompt": clip(prompt, 8192)})
	resp, err := m.Inner.Generate(ctx, prompt, options)
	m.emitResponse(ctx, "generate", resp, err)
	return resp, err
}

func (m *InstrumentedModel) GenerateStream(ctx context.Context, prompt string, options *LLMOptions) (<-chan string, error) {
	m.emitPrompt(ctx, "generate_stream", map[string]any{
		"model":          modelFromOptions(options),
		"prompt_chars":   len(prompt),
		"prompt_preview": clip(prompt, 1024),
	}, m.Debug, map[string]any{"prompt": clip(prompt, 8192)})
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
	base  map[string]any
	debug map[string]any
}

func chatMeta(messages []Message, tools []LLMToolSpec, options *LLMOptions) chatMetaPayload {
	var roles []string
	preview := make([]map[string]any, 0, min(len(messages), 20))
	for i, msg := range messages {
		if i >= 20 {
			break
		}
		roles = append(roles, msg.Role)
		preview = append(preview, map[string]any{
			"role":    msg.Role,
			"name":    msg.Name,
			"content": clip(msg.Content, 512),
		})
	}
	toolNames := make([]string, 0, len(tools))
	for _, t := range tools {
		toolNames = append(toolNames, t.Name)
	}
	base := map[string]any{
		"model":            modelFromOptions(options),
		"message_count":    len(messages),
		"roles":            roles,
		"messages_preview": preview,
		"tool_count":       len(tools),
		"tool_names":       toolNames,
	}
	debug := map[string]any{}
	if len(messages) > 0 {
		full := make([]map[string]any, 0, len(messages))
		for _, msg := range messages {
			full = append(full, map[string]any{
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

func (m *InstrumentedModel) emitPrompt(ctx context.Context, kind string, base map[string]any, debug bool, debugFields map[string]any) {
	if m == nil || m.Telemetry == nil {
		return
	}
	metadata := map[string]any{
		"kind": kind,
	}
	if m.Inner != nil {
		metadata["tool_calling_mode"] = ToolCallingModeLabel(m.Inner)
	}
	for k, v := range base {
		metadata[k] = v
	}
	if debug {
		for k, v := range debugFields {
			metadata[k] = v
		}
	}
	ev := observability.Event{
		Type:      observability.EventLLMPrompt,
		Timestamp: time.Now().UTC(),
		Message:   fmt.Sprintf("llm %s prompt", kind),
		Metadata:  metadata,
	}
	stampObservabilityCorrelation(ctx, &ev)
	m.Telemetry.Emit(ev)
}

func (m *InstrumentedModel) emitResponse(ctx context.Context, kind string, resp *LLMResponse, err error) {
	if m == nil {
		return
	}
	if obs := observability.UsageObserverFromContext(ctx); obs != nil && resp != nil {
		obs.RecordTokenUsage(observability.TokenUsage(resp.Usage))
		if snapshot, ok := obs.ConsumeResetNotice(); ok && m.Telemetry != nil {
			metadata := map[string]any{
				"budget_snapshot": snapshot,
			}
			ev := observability.Event{
				Type:      observability.EventSessionResetRequired,
				Timestamp: time.Now().UTC(),
				Message:   "session reset required",
				Metadata:  metadata,
			}
			stampObservabilityCorrelation(ctx, &ev)
			m.Telemetry.Emit(ev)
		}
	}
	if obs := observability.SnapshotObserverFromContext(ctx); obs != nil {
		obs.Observe()
	}
	if ing := observability.ResponseIngesterFromContext(ctx); ing != nil && resp != nil && err == nil {
		go func() {
			timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			_ = ing.IngestLLMResponse(timeoutCtx, resp)
		}()
	}
	if m.Telemetry == nil {
		return
	}
	metadata := map[string]any{
		"kind": kind,
	}
	if m.Inner != nil {
		metadata["tool_calling_mode"] = ToolCallingModeLabel(m.Inner)
	}
	if resp != nil {
		metadata["finish_reason"] = resp.FinishReason
		metadata["text_preview"] = clip(resp.Text, 1024)
		metadata["usage"] = resp.Usage
		if len(resp.ToolCalls) > 0 {
			toolCalls, err := json.Marshal(resp.ToolCalls)
			if err == nil {
				metadata["tool_calls"] = string(toolCalls)
			}
		}
	}
	if err != nil {
		metadata["error"] = err.Error()
	}
	ev := observability.Event{
		Type:      observability.EventLLMResponse,
		Timestamp: time.Now().UTC(),
		Message:   fmt.Sprintf("llm %s response", kind),
		Metadata:  metadata,
	}
	stampObservabilityCorrelation(ctx, &ev)
	m.Telemetry.Emit(ev)
}

func modelFromOptions(options *LLMOptions) string {
	if options != nil && options.Model != "" {
		return options.Model
	}
	return ""
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
