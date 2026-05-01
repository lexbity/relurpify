package contracts

import (
	"context"
	"time"
)

// UsageObserver is implemented by framework components that want to observe
// token usage after each LLM call. Stored in context by the framework layer;
// retrieved by InstrumentedModel without importing framework packages.
type UsageObserver interface {
	RecordTokenUsage(usage TokenUsageReport)
	// ConsumeResetNotice returns an opaque snapshot value (suitable for telemetry
	// metadata) and true when a session reset is warranted. Returns nil, false otherwise.
	ConsumeResetNotice() (any, bool)
}

// SnapshotObserver is implemented by framework components that want to record
// periodic budget snapshots. Called after every LLM response.
type SnapshotObserver interface {
	Observe()
}

// ResponseIngester is implemented by framework components that want to index
// LLM responses into the knowledge graph as durable chunks.
type ResponseIngester interface {
	IngestLLMResponse(ctx context.Context, resp *LLMResponse) error
}

type (
	usageObserverKey    struct{}
	snapshotObserverKey struct{}
	responseIngesterKey struct{}
)

// WithUsageObserver attaches a UsageObserver to the context.
func WithUsageObserver(ctx context.Context, obs UsageObserver) context.Context {
	return context.WithValue(ctx, usageObserverKey{}, obs)
}

// UsageObserverFromContext extracts the UsageObserver from context, or nil.
func UsageObserverFromContext(ctx context.Context) UsageObserver {
	v, _ := ctx.Value(usageObserverKey{}).(UsageObserver)
	return v
}

// WithSnapshotObserver attaches a SnapshotObserver to the context.
func WithSnapshotObserver(ctx context.Context, obs SnapshotObserver) context.Context {
	return context.WithValue(ctx, snapshotObserverKey{}, obs)
}

// SnapshotObserverFromContext extracts the SnapshotObserver from context, or nil.
func SnapshotObserverFromContext(ctx context.Context) SnapshotObserver {
	v, _ := ctx.Value(snapshotObserverKey{}).(SnapshotObserver)
	return v
}

// WithResponseIngester attaches a ResponseIngester to the context.
func WithResponseIngester(ctx context.Context, ing ResponseIngester) context.Context {
	return context.WithValue(ctx, responseIngesterKey{}, ing)
}

// ResponseIngesterFromContext extracts the ResponseIngester from context, or nil.
func ResponseIngesterFromContext(ctx context.Context) ResponseIngester {
	v, _ := ctx.Value(responseIngesterKey{}).(ResponseIngester)
	return v
}

// LLMOptions configures language model calls. Keeping the options struct inside
// the contracts package avoids hard-coding provider-specific fields in agent code.
type LLMOptions struct {
	Model          string
	Temperature    float64
	MaxTokens      int
	Stop           []string
	TopP           float64
	Stream         bool
	StreamCallback func(string) `json:"-"`
}

// ToolCall encodes a function invocation requested by the LLM.
type ToolCall struct {
	ID   string                 `json:"id,omitempty"`
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args"`
}

// TokenUsageReport normalizes token accounting across LLM backends.
type TokenUsageReport struct {
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
	Estimated        bool   `json:"estimated,omitempty"`
	EstimationMethod string `json:"estimation_method,omitempty"`
}

// LLMResponse is the result of a language model invocation.
type LLMResponse struct {
	Text         string           `json:"text,omitempty"`
	FinishReason string           `json:"finish_reason,omitempty"`
	Usage        TokenUsageReport `json:"usage"`
	ToolCalls    []ToolCall       `json:"tool_calls,omitempty"`
}

// Message is used for chat-like interactions.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	Name       string     `json:"name,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

// CapabilityDescriptor describes a capability for LLM tool use.
// Defined in platform/contracts to allow LLM packages to use it without
// importing framework/core.
type CapabilityDescriptor struct {
	ID          string  `json:"id" yaml:"id"`
	Name        string  `json:"name" yaml:"name"`
	Description string  `json:"description,omitempty" yaml:"description,omitempty"`
	InputSchema *Schema `json:"input_schema,omitempty" yaml:"input_schema,omitempty"`
}

// Schema describes the shape of capability inputs and outputs. Moved from
// framework/core to platform/contracts so platform/llm packages can use it
// without importing framework/core.
type Schema struct {
	Type        string             `json:"type,omitempty" yaml:"type,omitempty"`
	Properties  map[string]*Schema `json:"properties,omitempty" yaml:"properties,omitempty"`
	Items       *Schema            `json:"items,omitempty" yaml:"items,omitempty"`
	Required    []string           `json:"required,omitempty" yaml:"required,omitempty"`
	Default     any                `json:"default,omitempty" yaml:"default,omitempty"`
	Enum        []any              `json:"enum,omitempty" yaml:"enum,omitempty"`
	Title       string             `json:"title,omitempty" yaml:"title,omitempty"`
	Description string             `json:"description,omitempty" yaml:"description,omitempty"`
	Format      string             `json:"format,omitempty" yaml:"format,omitempty"`
}

// LLMToolSpec is the provider-agnostic tool definition passed to LLM backends.
// It carries only the fields needed to describe a callable tool to a language
// model: name, description, and input schema. Provider-specific wire formats
// (e.g. Ollama's {"type":"function","function":{...}}) are handled entirely
// inside the platform/llm layer and do not leak into the capability model.
type LLMToolSpec struct {
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	InputSchema *Schema `json:"input_schema,omitempty"`
}

// ProfiledModel is an optional extension for LanguageModel implementations that
// expose active model profile metadata. Callers type-assert to check support:
//
//	if pm, ok := model.(ProfiledModel); ok { ... }
//
// The LanguageModel interface is not changed.
type ProfiledModel interface {
	ToolRepairStrategy() string  // "llm" | "heuristic-only"
	MaxToolsPerCall() int        // 0 = no limit
	UsesNativeToolCalling() bool // true if profile enables native API tool calling
}

// LanguageModel provides the required LLM capabilities.
type LanguageModel interface {
	Generate(ctx context.Context, prompt string, options *LLMOptions) (*LLMResponse, error)
	GenerateStream(ctx context.Context, prompt string, options *LLMOptions) (<-chan string, error)
	Chat(ctx context.Context, messages []Message, options *LLMOptions) (*LLMResponse, error)
	ChatWithTools(ctx context.Context, messages []Message, tools []LLMToolSpec, options *LLMOptions) (*LLMResponse, error)
}

// BackendClass identifies the broad backend family so framework code can make
// transport-vs-native decisions without importing platform-specific packages.
type BackendClass string

const (
	BackendClassTransport BackendClass = "transport"
	BackendClassNative    BackendClass = "native"
)

// BackendCapabilities describes the high-level features a backend exposes.
type BackendCapabilities struct {
	NativeToolCalling    bool
	Streaming            bool
	Embeddings           bool
	ModelListing         bool
	BackendClass         BackendClass
	UsageReporting       bool
	ContextSizeDiscovery bool
}

// LLMToolSpecFromTool builds an LLMToolSpec from a local Tool implementation.
func LLMToolSpecFromTool(t Tool) LLMToolSpec {
	spec := LLMToolSpec{
		Name:        t.Name(),
		Description: t.Description(),
	}
	params := t.Parameters()
	if len(params) > 0 {
		props := make(map[string]*Schema, len(params))
		var required []string
		for _, p := range params {
			prop := &Schema{
				Type:        p.Type,
				Description: p.Description,
			}
			if p.Default != nil {
				prop.Default = p.Default
			}
			props[p.Name] = prop
			if p.Required {
				required = append(required, p.Name)
			}
		}
		spec.InputSchema = &Schema{
			Type:       "object",
			Properties: props,
			Required:   required,
		}
	}
	return spec
}

// LLMToolSpecsFromTools converts a slice of local Tool implementations to
// LLMToolSpec values for passing to ChatWithTools.
func LLMToolSpecsFromTools(tools []Tool) []LLMToolSpec {
	if len(tools) == 0 {
		return nil
	}
	specs := make([]LLMToolSpec, len(tools))
	for i, t := range tools {
		specs[i] = LLMToolSpecFromTool(t)
	}
	return specs
}

// LLMToolSpecFromDescriptor builds an LLMToolSpec from a CapabilityDescriptor.
func LLMToolSpecFromDescriptor(d CapabilityDescriptor) LLMToolSpec {
	return LLMToolSpec{
		Name:        d.Name,
		Description: d.Description,
		InputSchema: d.InputSchema,
	}
}

// EventType categorizes telemetry events.
type EventType string

const (
	EventLLMPrompt            EventType = "llm_prompt"
	EventLLMResponse          EventType = "llm_response"
	EventBudgetSnapshot       EventType = "budget_snapshot"
	EventSessionResetRequired EventType = "session_reset_required"
)

// Event captures structured telemetry data.
type Event struct {
	Type      EventType              `json:"type"`
	TaskID    string                 `json:"task_id,omitempty"`
	Message   string                 `json:"message,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Telemetry captures execution traces emitted by the graph runtime.
type Telemetry interface {
	Emit(event Event)
}
