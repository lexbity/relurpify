package model

import (
	"context"
	"strings"
)

// LanguageModel is the primary contract every LLM backend satisfies.
type LanguageModel interface {
	Generate(ctx context.Context, prompt string, options *LLMOptions) (*LLMResponse, error)
	GenerateStream(ctx context.Context, prompt string, options *LLMOptions) (<-chan string, error)
	Chat(ctx context.Context, messages []Message, options *LLMOptions) (*LLMResponse, error)
	ChatWithTools(ctx context.Context, messages []Message, tools []LLMToolSpec, options *LLMOptions) (*LLMResponse, error)
}

// LLMOptions configures a single model invocation.
type LLMOptions struct {
	Model          string
	Temperature    float64
	MaxTokens      int
	Stop           []string
	TopP           float64
	Stream         bool
	Config         map[string]any `json:",omitempty"`
	StreamCallback func(string)   `json:"-"`
}

// LLMResponse is the result of a model invocation.
type LLMResponse struct {
	Text         string
	FinishReason string
	Usage        TokenUsage
	ToolCalls    []ToolCall
}

// Message is a single turn in a chat conversation.
type Message struct {
	Role       string
	Content    string
	Name       string
	ToolCallID string
	ToolCalls  []ToolCall
}

// ToolCall represents a tool invocation requested by the LLM.
type ToolCall struct {
	ID   string
	Name string
	Args map[string]any
}

// ProfiledModel extends LanguageModel with profile metadata.
type ProfiledModel interface {
	LanguageModel
	ToolRepairStrategy() string
	MaxToolsPerCall() int
	UsesNativeToolCalling() bool
}

// BackendClass distinguishes transport-level from native tool-calling backends.
type BackendClass string

const (
	BackendClassTransport BackendClass = "transport"
	BackendClassNative    BackendClass = "native"
)

// BackendCapabilities declares what a back-end supports.
type BackendCapabilities struct {
	NativeToolCalling    bool
	Streaming            bool
	Embeddings           bool
	ModelListing         bool
	BackendClass         BackendClass
	UsageReporting       bool
	ContextSizeDiscovery bool
}

// ModelProfile carries model-specific quirks and metadata for provider/backend selection.
type ModelProfile struct {
	Provider    string
	Model       string
	Pattern     string
	ContextSize int
	ToolCalling ModelToolCalling
	Repair      ModelRepairConfig
	Schema      ModelSchemaConfig
	SourcePath  string
}

// ModelToolCalling describes tool-calling capabilities.
type ModelToolCalling struct {
	NativeAPI               bool
	DoubleEncodedArgs       bool
	MultilineStringLiterals bool
	MaxToolsPerCall         int
	Intent                  string
	ToolChoiceSupported     bool
	ParallelToolCalls       bool
	StrictSchema            bool
	Aliases                 map[string]string
}

// ModelRepairConfig controls tool-call repair behavior.
type ModelRepairConfig struct {
	Strategy    string
	MaxAttempts int
}

// ModelSchemaConfig controls schema generation.
type ModelSchemaConfig struct {
	FlattenNested     bool
	MaxDescriptionLen int
	Style             string
}

func (p *ModelProfile) Normalize() {
	if p == nil {
		return
	}
	p.Provider = strings.ToLower(strings.TrimSpace(p.Provider))
	p.Model = strings.TrimSpace(p.Model)
	p.Pattern = strings.TrimSpace(p.Pattern)
	if p.Repair.Strategy == "" {
		p.Repair.Strategy = "heuristic-only"
	}
	if p.Repair.MaxAttempts < 0 {
		p.Repair.MaxAttempts = 0
	}
}

func (p *ModelProfile) Clone() *ModelProfile {
	if p == nil {
		return nil
	}
	clone := *p
	return &clone
}

func (p *ModelProfile) IsExactModelMatch() bool {
	if p == nil {
		return false
	}
	if p.Model != "" {
		return !hasGlobMeta(p.Model)
	}
	return p.Pattern != "" && !hasGlobMeta(p.Pattern)
}

func (p *ModelProfile) MatchPattern() string {
	if p == nil {
		return ""
	}
	if p.Model != "" {
		return p.Model
	}
	return p.Pattern
}

func hasGlobMeta(s string) bool {
	return strings.ContainsAny(s, "*?[")
}

// LLMToolSpec describes a tool in the format expected by LLM providers.
type LLMToolSpec struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	InputSchema *Schema `json:"input_schema,omitempty"`
}

// Schema is a JSON Schema type descriptor for LLM tool parameter definitions.
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

// TokenUsage records token consumption for a model invocation.
type TokenUsage struct {
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
	Estimated        bool   `json:"estimated"`
	EstimationMethod string `json:"estimation_method,omitempty"`
}
