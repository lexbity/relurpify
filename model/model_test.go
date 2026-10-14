package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelProfileNormalize(t *testing.T) {
	t.Run("nil receiver is a no-op", func(t *testing.T) {
		var profile *ModelProfile
		profile.Normalize()
		assert.Nil(t, profile)
	})

	t.Run("trims and lowercases provider, keeps model case", func(t *testing.T) {
		profile := &ModelProfile{
			Provider: "  Ollama  ",
			Model:    " LLaMA3 ",
			Pattern:  " llama* ",
		}
		profile.Normalize()
		assert.Equal(t, "ollama", profile.Provider)
		assert.Equal(t, "LLaMA3", profile.Model, "model casing is preserved")
		assert.Equal(t, "llama*", profile.Pattern)
	})

	t.Run("fills repair defaults", func(t *testing.T) {
		profile := &ModelProfile{}
		profile.Normalize()
		assert.Equal(t, "heuristic-only", profile.Repair.Strategy)
		assert.Equal(t, 0, profile.Repair.MaxAttempts)
	})

	t.Run("clamps negative max attempts and keeps explicit strategy", func(t *testing.T) {
		profile := &ModelProfile{Repair: ModelRepairConfig{Strategy: "json-repair", MaxAttempts: -3}}
		profile.Normalize()
		assert.Equal(t, "json-repair", profile.Repair.Strategy, "explicit strategy is preserved")
		assert.Equal(t, 0, profile.Repair.MaxAttempts)

		positive := &ModelProfile{Repair: ModelRepairConfig{MaxAttempts: 4}}
		positive.Normalize()
		assert.Equal(t, 4, positive.Repair.MaxAttempts, "non-negative attempts are preserved")
	})
}

func TestModelProfileClone(t *testing.T) {
	assert.Nil(t, (*ModelProfile)(nil).Clone(), "cloning nil yields nil")

	original := &ModelProfile{
		Provider: "ollama",
		Model:    "llama3",
		ToolCalling: ModelToolCalling{
			Aliases: map[string]string{"llama3:latest": "llama3"},
		},
	}
	clone := original.Clone()
	require.NotNil(t, clone)
	require.Equal(t, original, clone)

	// Clone is a struct copy: scalar fields are independent, reference
	// fields (maps, slices) are still shared with the original.
	clone.Provider = "openai"
	assert.Equal(t, "ollama", original.Provider, "scalar fields are independent")

	clone.ToolCalling.Aliases["mutated"] = "value"
	assert.Contains(t, original.ToolCalling.Aliases, "mutated",
		"map fields are shared between clone and original (shallow copy)")
}

func TestModelProfileIsExactModelMatch(t *testing.T) {
	assert.False(t, (*ModelProfile)(nil).IsExactModelMatch())

	testCases := []struct {
		name    string
		profile ModelProfile
		want    bool
	}{
		{"plain model name", ModelProfile{Model: "llama3"}, true},
		{"model with glob meta", ModelProfile{Model: "llama*"}, false},
		{"model with question mark", ModelProfile{Model: "llama?"}, false},
		{"model with bracket", ModelProfile{Model: "llama[3]"}, false},
		{"exact pattern when model empty", ModelProfile{Pattern: "qwen2.5"}, true},
		{"glob pattern when model empty", ModelProfile{Pattern: "qwen*"}, false},
		{"neither model nor pattern", ModelProfile{}, false},
		{"model takes precedence over glob pattern", ModelProfile{Model: "llama3", Pattern: "*"}, true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			profile := tc.profile
			assert.Equal(t, tc.want, profile.IsExactModelMatch())
		})
	}
}

func TestModelProfileMatchPattern(t *testing.T) {
	assert.Equal(t, "", (*ModelProfile)(nil).MatchPattern())

	assert.Equal(t, "llama3", (&ModelProfile{Model: "llama3", Pattern: "pattern"}).MatchPattern(),
		"model wins over pattern")
	assert.Equal(t, "qwen*", (&ModelProfile{Pattern: "qwen*"}).MatchPattern(),
		"pattern is the fallback when model is empty")
	assert.Equal(t, "", (&ModelProfile{}).MatchPattern())
}

func TestProviderFailureSentinelsAreClassifiable(t *testing.T) {
	wrappedContext := fmt.Errorf("generate failed: %w", ErrContextLength)
	wrappedBudget := fmt.Errorf("replay rejected: %w", ErrTokenBudget)

	assert.True(t, errors.Is(wrappedContext, ErrContextLength))
	assert.False(t, errors.Is(wrappedContext, ErrTokenBudget))
	assert.True(t, errors.Is(wrappedBudget, ErrTokenBudget))
	assert.False(t, errors.Is(wrappedBudget, ErrContextLength))
	assert.ErrorIs(t, ErrContextLength, ErrContextLength)
	assert.NotErrorIs(t, errors.New("unrelated"), ErrContextLength)
}

func TestLLMOptionsJSONRoundTrip(t *testing.T) {
	options := LLMOptions{
		Model:          "llama3",
		Temperature:    0.7,
		MaxTokens:      512,
		Stop:           []string{"\n\n"},
		TopP:           0.9,
		Stream:         true,
		ContextWindow:  8192,
		Config:         map[string]any{"num_gpu": 1},
		StreamCallback: func(string) {},
	}

	data, err := json.Marshal(options)
	require.NoError(t, err)

	var decoded LLMOptions
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, options.Model, decoded.Model)
	assert.Equal(t, options.Temperature, decoded.Temperature)
	assert.Equal(t, options.MaxTokens, decoded.MaxTokens)
	assert.Equal(t, options.Stop, decoded.Stop)
	assert.Equal(t, options.TopP, decoded.TopP)
	assert.True(t, decoded.Stream)
	assert.Equal(t, 8192, decoded.ContextWindow)
	decodedConfig := map[string]any{}
	for k, v := range decoded.Config {
		decodedConfig[k] = v
	}
	assert.Equal(t, float64(1), decodedConfig["num_gpu"], "JSON round trip yields numeric types")

	assert.NotContains(t, string(data), "StreamCallback",
		"the stream callback must never serialize")
}

func TestLLMOptionsZeroContextWindowOmitted(t *testing.T) {
	data, err := json.Marshal(LLMOptions{Model: "llama3"})
	require.NoError(t, err)
	assert.NotContains(t, string(data), "ContextWindow",
		"zero context window must be omitted so transports derive their default")
}

func TestSchemaJSONRoundTrip(t *testing.T) {
	schema := &Schema{
		Type: "object",
		Properties: map[string]*Schema{
			"path": {Type: "string", Description: "file path", Title: "Path", Format: "uri"},
			"tags": {
				Type:  "array",
				Items: &Schema{Type: "string"},
				Enum:  []any{"a", "b"},
			},
			"count": {Type: "integer", Default: 1},
		},
		Required: []string{"path"},
	}

	data, err := json.Marshal(schema)
	require.NoError(t, err)

	var decoded Schema
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, "object", decoded.Type)
	require.Contains(t, decoded.Properties, "path")
	assert.Equal(t, "file path", decoded.Properties["path"].Description)
	assert.Equal(t, []string{"path"}, decoded.Required)
	require.NotNil(t, decoded.Properties["tags"].Items)
	assert.Equal(t, "string", decoded.Properties["tags"].Items.Type)
	assert.Equal(t, []any{"a", "b"}, decoded.Properties["tags"].Enum)
	assert.Equal(t, float64(1), decoded.Properties["count"].Default)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.Equal(t, "object", raw["type"])
}

func TestLLMToolSpecJSONShape(t *testing.T) {
	data, err := json.Marshal(LLMToolSpec{
		Name:        "read_file",
		Description: "reads a file",
		InputSchema: &Schema{Type: "object"},
	})
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.Equal(t, "read_file", raw["name"])
	assert.Equal(t, "reads a file", raw["description"])
	assert.Contains(t, raw, "input_schema", "input_schema is the provider-facing key")

	noSchema, err := json.Marshal(LLMToolSpec{Name: "bare"})
	require.NoError(t, err)
	assert.NotContains(t, string(noSchema), "input_schema", "nil schema is omitted")
}

func TestTokenUsageJSONFieldNames(t *testing.T) {
	data, err := json.Marshal(TokenUsage{
		PromptTokens:     10,
		CompletionTokens: 20,
		TotalTokens:      30,
		Estimated:        true,
		EstimationMethod: "chars/4",
	})
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.Equal(t, float64(10), raw["prompt_tokens"])
	assert.Equal(t, float64(20), raw["completion_tokens"])
	assert.Equal(t, float64(30), raw["total_tokens"])
	assert.Equal(t, true, raw["estimated"])
	assert.Equal(t, "chars/4", raw["estimation_method"])
}

func TestBackendClassVocabulary(t *testing.T) {
	assert.Equal(t, "transport", string(BackendClassTransport))
	assert.Equal(t, "native", string(BackendClassNative))
}

// echoModel is a LanguageModel implementation over the model package's own
// ports, proving the interface is implementable and its contract flows.
type echoModel struct {
	generated string
	streamed  []string
	messages  []Message
	tools     []LLMToolSpec
	options   *LLMOptions
}

func (m *echoModel) Generate(_ context.Context, prompt string, options *LLMOptions) (*LLMResponse, error) {
	m.generated = prompt
	m.options = options
	return &LLMResponse{Text: "echo:" + prompt, FinishReason: "stop"}, nil
}

func (m *echoModel) GenerateStream(_ context.Context, prompt string, _ *LLMOptions) (<-chan string, error) {
	ch := make(chan string, 2)
	ch <- prompt
	ch <- prompt
	close(ch)
	return ch, nil
}

func (m *echoModel) Chat(_ context.Context, messages []Message, options *LLMOptions) (*LLMResponse, error) {
	m.messages = messages
	m.options = options
	return &LLMResponse{Text: "chat:" + messages[len(messages)-1].Content}, nil
}

func (m *echoModel) ChatWithTools(_ context.Context, messages []Message, tools []LLMToolSpec, options *LLMOptions) (*LLMResponse, error) {
	m.messages = messages
	m.tools = tools
	m.options = options
	return &LLMResponse{Text: "toolchat", ToolCalls: []ToolCall{{ID: "call-1", Name: tools[0].Name}}}, nil
}

var _ LanguageModel = (*echoModel)(nil)

func TestLanguageModelContractDrivesAllFourMethods(t *testing.T) {
	ctx := context.Background()
	model := &echoModel{}
	options := &LLMOptions{Model: "llama3"}

	response, err := model.Generate(ctx, "hi", options)
	require.NoError(t, err)
	assert.Equal(t, "echo:hi", response.Text)
	assert.Equal(t, "llama3", model.options.Model)

	stream, err := model.GenerateStream(ctx, "go", options)
	require.NoError(t, err)
	for token := range stream {
		model.streamed = append(model.streamed, token)
	}
	assert.Equal(t, []string{"go", "go"}, model.streamed, "stream yields tokens then closes")

	chatResponse, err := model.Chat(ctx, []Message{{Role: "user", Content: "hello"}}, options)
	require.NoError(t, err)
	assert.Equal(t, "chat:hello", chatResponse.Text)

	toolResponse, err := model.ChatWithTools(ctx, []Message{{Role: "user", Content: "read"}}, []LLMToolSpec{{Name: "read_file"}}, options)
	require.NoError(t, err)
	require.Len(t, toolResponse.ToolCalls, 1)
	assert.Equal(t, "read_file", toolResponse.ToolCalls[0].Name)
	assert.Equal(t, []LLMToolSpec{{Name: "read_file"}}, model.tools)
}

// profiledEcho extends echoModel with the ProfiledModel surface.
type profiledEcho struct {
	echoModel
}

func (m *profiledEcho) ToolRepairStrategy() string  { return "json-repair" }
func (m *profiledEcho) MaxToolsPerCall() int        { return 3 }
func (m *profiledEcho) UsesNativeToolCalling() bool { return true }

var _ ProfiledModel = (*profiledEcho)(nil)

func TestProfiledModelExtendsLanguageModel(t *testing.T) {
	profiled := &profiledEcho{}
	assert.Equal(t, "json-repair", profiled.ToolRepairStrategy())
	assert.Equal(t, 3, profiled.MaxToolsPerCall())
	assert.True(t, profiled.UsesNativeToolCalling())

	response, err := profiled.Generate(context.Background(), "hi", nil)
	require.NoError(t, err)
	assert.Equal(t, "echo:hi", response.Text)
}

// stubBackend and recordingSink prove the runtime ports are implementable.
type stubBackend struct {
	model  LanguageModel
	debug  bool
	closed bool
}

func (b *stubBackend) Model() LanguageModel    { return b.model }
func (b *stubBackend) Close() error            { b.closed = true; return nil }
func (b *stubBackend) SetDebugLogging(on bool) { b.debug = on }

var _ ModelBackend = (*stubBackend)(nil)

type recordingSink struct {
	events []any
}

func (s *recordingSink) Emit(_ context.Context, event any) { s.events = append(s.events, event) }

var _ EventSink = (*recordingSink)(nil)

func TestModelBackendAndEventSinkPorts(t *testing.T) {
	backend := &stubBackend{model: &echoModel{}}
	backend.SetDebugLogging(true)
	assert.True(t, backend.debug)
	assert.NotNil(t, backend.Model())
	require.NoError(t, backend.Close())
	assert.True(t, backend.closed)

	sink := &recordingSink{}
	sink.Emit(context.Background(), "event-1")
	assert.Equal(t, []any{"event-1"}, sink.events)
}

func TestModelFactoryProducesConfiguredModel(t *testing.T) {
	var factory ModelFactory = func(sink EventSink, debug bool) LanguageModel {
		sink.Emit(context.Background(), "factory:configured")
		return &echoModel{}
	}

	sink := &recordingSink{}
	model := factory(sink, true)
	require.NotNil(t, model)

	response, err := model.Generate(context.Background(), "ping", nil)
	require.NoError(t, err)
	assert.Equal(t, "echo:ping", response.Text)
	assert.Equal(t, []any{"factory:configured"}, sink.events)
}

func TestModelProductBundlesBackendAndFactory(t *testing.T) {
	backend := &stubBackend{model: &echoModel{}}
	product := ModelProduct{Backend: backend, ModelFactory: func(EventSink, bool) LanguageModel { return backend.model }}

	assert.Same(t, backend, product.Backend)
	require.NotNil(t, product.ModelFactory)
	assert.NotNil(t, product.ModelFactory(nil, false))
}

func TestLLMResponseCarriesThinkingAndUsage(t *testing.T) {
	response := LLMResponse{
		Text:         "answer",
		Thinking:     "reasoning trace",
		FinishReason: "length",
		Usage:        TokenUsage{PromptTokens: 5, CompletionTokens: 7, TotalTokens: 12, Estimated: true, EstimationMethod: "chars/4"},
		ToolCalls:    []ToolCall{{ID: "c1", Name: "tool", Args: map[string]any{"k": "v"}}},
	}
	assert.Equal(t, "reasoning trace", response.Thinking)
	assert.Equal(t, "length", response.FinishReason)
	assert.Equal(t, 12, response.Usage.TotalTokens)
	assert.Equal(t, "tool", response.ToolCalls[0].Name)

	message := Message{Role: "tool", Content: "result", Name: "tool", ToolCallID: "c1", ToolCalls: response.ToolCalls}
	assert.Equal(t, "c1", message.ToolCallID)
	assert.Len(t, message.ToolCalls, 1)
}

func TestBackendCapabilitiesAndModelProfileStructures(t *testing.T) {
	caps := BackendCapabilities{
		NativeToolCalling:    true,
		Streaming:            true,
		Embeddings:           false,
		ModelListing:         true,
		BackendClass:         BackendClassNative,
		UsageReporting:       true,
		ContextSizeDiscovery: true,
	}
	assert.True(t, caps.NativeToolCalling && caps.Streaming && caps.ModelListing && caps.UsageReporting && caps.ContextSizeDiscovery)
	assert.False(t, caps.Embeddings)
	assert.Equal(t, BackendClassNative, caps.BackendClass)

	profile := ModelProfile{
		Provider:    "ollama",
		Model:       "llama3",
		Pattern:     "llama*",
		ContextSize: 8192,
		ToolCalling: ModelToolCalling{
			NativeAPI: true, MaxToolsPerCall: 5, Intent: "agentic",
			ToolChoiceSupported: true, ParallelToolCalls: true, StrictSchema: true,
			Aliases: map[string]string{"llama3:latest": "llama3"},
		},
		Schema:     ModelSchemaConfig{FlattenNested: true, MaxDescriptionLen: 200, Style: "flat"},
		SourcePath: "profiles/ollama.yaml",
	}
	assert.Equal(t, 8192, profile.ContextSize)
	assert.True(t, profile.ToolCalling.NativeAPI)
	assert.Equal(t, map[string]string{"llama3:latest": "llama3"}, profile.ToolCalling.Aliases)
	assert.Equal(t, "flat", profile.Schema.Style)
	assert.Equal(t, 200, profile.Schema.MaxDescriptionLen)
}
