package openaicompat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

// Client implements core.LanguageModel for OpenAI-compatible backends.
type Client struct {
	cfg        OpenAICompatConfig
	httpClient *http.Client
}

func NewClient(cfg OpenAICompatConfig) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) Generate(ctx context.Context, prompt string, options *core.LLMOptions) (*core.LLMResponse, error) {
	return c.chat(ctx, []core.Message{{Role: "user", Content: prompt}}, nil, options, false, false, nil)
}

func (c *Client) GenerateStream(ctx context.Context, prompt string, options *core.LLMOptions) (<-chan string, error) {
	out := make(chan string)
	go func() {
		defer close(out)
		_, _ = c.chat(ctx, []core.Message{{Role: "user", Content: prompt}}, nil, options, false, true, func(token string) {
			out <- token
		})
	}()
	return out, nil
}

func (c *Client) Chat(ctx context.Context, messages []core.Message, options *core.LLMOptions) (*core.LLMResponse, error) {
	return c.chat(ctx, messages, nil, options, false, false, nil)
}

func (c *Client) ChatWithTools(ctx context.Context, messages []core.Message, tools []core.LLMToolSpec, options *core.LLMOptions) (*core.LLMResponse, error) {
	if options != nil && options.StreamCallback != nil {
		return c.chat(ctx, messages, tools, options, c.cfg.NativeToolCalling, true, options.StreamCallback)
	}
	return c.chat(ctx, messages, tools, options, c.cfg.NativeToolCalling, false, nil)
}

// ToolRepairStrategy implements core.ProfiledModel.
func (c *Client) ToolRepairStrategy() string {
	return "heuristic-only"
}

// MaxToolsPerCall implements core.ProfiledModel.
func (c *Client) MaxToolsPerCall() int {
	return 0
}

// UsesNativeToolCalling implements core.ProfiledModel.
func (c *Client) UsesNativeToolCalling() bool {
	return c.cfg.NativeToolCalling
}

func (c *Client) ChatStream(ctx context.Context, messages []core.Message, tools []core.LLMToolSpec, options *core.LLMOptions) (<-chan string, error) {
	out := make(chan string)
	go func() {
		defer close(out)
		_, err := c.chat(ctx, messages, tools, options, c.cfg.NativeToolCalling, true, func(token string) {
			out <- token
		})
		_ = err
	}()
	return out, nil
}

func (c *Client) chat(ctx context.Context, messages []core.Message, tools []core.LLMToolSpec, options *core.LLMOptions, includeTools bool, stream bool, tokenSink func(string)) (*core.LLMResponse, error) {
	reqBody := map[string]any{
		"model":    modelFromOptions(options),
		"messages": convertMessages(messages),
		"stream":   stream,
	}
	c.applyOptions(reqBody, options)
	if includeTools && len(tools) > 0 {
		reqBody["tools"] = convertTools(tools)
	}
	if stream {
		return c.doChatStream(ctx, reqBody, tokenSink)
	}
	return c.doChat(ctx, reqBody)
}

func (c *Client) applyOptions(payload map[string]any, options *core.LLMOptions) {
	if options == nil {
		return
	}
	if options.Temperature != 0 {
		payload["temperature"] = options.Temperature
	}
	if options.MaxTokens != 0 {
		payload["max_tokens"] = options.MaxTokens
	}
	if options.TopP != 0 {
		payload["top_p"] = options.TopP
	}
	if len(options.Stop) > 0 {
		payload["stop"] = options.Stop
	}
}

func (c *Client) doChat(ctx context.Context, payload map[string]any) (*core.LLMResponse, error) {
	req, err := c.newRequest(ctx, http.MethodPost, "/v1/chat/completions", payload)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, readHTTPError(resp)
	}
	var raw chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	return decodeChatResponse(raw)
}

func (c *Client) doChatStream(ctx context.Context, payload map[string]any, tokenSink func(string)) (*core.LLMResponse, error) {
	req, err := c.newRequest(ctx, http.MethodPost, "/v1/chat/completions", payload)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, readHTTPError(resp)
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var fullText strings.Builder
	var final chatCompletionChunk
	builders := make(map[int]*toolCallBuilder)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk chatCompletionChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) == 0 {
			final = chunk
			continue
		}
		delta := chunk.Choices[0].Delta
		if delta.Content != "" {
			fullText.WriteString(delta.Content)
			if tokenSink != nil {
				tokenSink(delta.Content)
			}
		}
		for _, tc := range delta.ToolCalls {
			builder := builders[tc.Index]
			if builder == nil {
				builder = &toolCallBuilder{Index: tc.Index}
				builders[tc.Index] = builder
			}
			builder.Merge(tc)
		}
		final = chunk
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	respOut := &core.LLMResponse{
		Text:         fullText.String(),
		FinishReason: firstFinishReason(final),
		Usage:        normalizeUsage(final.Usage),
	}
	respOut.ToolCalls = append(respOut.ToolCalls, buildToolCalls(builders)...)
	respOut.ToolCalls = append(respOut.ToolCalls, parseToolCalls(firstChoiceMessage(final))...)
	return respOut, nil
}

func (c *Client) newRequest(ctx context.Context, method, path string, payload any) (*http.Request, error) {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.cfg.normalizedEndpoint()+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(c.cfg.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.cfg.APIKey))
	}
	return req, nil
}

func (c *Client) ListModels(ctx context.Context) ([]ModelInfo, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/v1/models", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, readHTTPError(resp)
	}
	var raw struct {
		Data []struct {
			ID        string `json:"id"`
			Object    string `json:"object"`
			OwnedBy   string `json:"owned_by"`
			CreatedAt int64  `json:"created"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := make([]ModelInfo, 0, len(raw.Data))
	for _, item := range raw.Data {
		out = append(out, ModelInfo{
			Name:      item.ID,
			OwnedBy:   item.OwnedBy,
			Object:    item.Object,
			UpdatedAt: time.Unix(item.CreatedAt, 0).UTC(),
		})
	}
	return out, nil
}

func (c *Client) SetDebugLogging(bool) {}

func (c *Client) Embedder(model string) *Embedder {
	return &Embedder{client: c, model: model}
}

func modelFromOptions(options *core.LLMOptions) string {
	if options != nil && options.Model != "" {
		return options.Model
	}
	return ""
}

func convertMessages(messages []core.Message) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		entry := map[string]any{
			"role":    msg.Role,
			"content": msg.Content,
		}
		if msg.Name != "" {
			entry["name"] = msg.Name
		}
		if msg.ToolCallID != "" {
			entry["tool_call_id"] = msg.ToolCallID
		}
		if len(msg.ToolCalls) > 0 {
			entry["tool_calls"] = convertMessageToolCalls(msg.ToolCalls)
		}
		out = append(out, entry)
	}
	return out
}

func convertMessageToolCalls(calls []core.ToolCall) []map[string]any {
	out := make([]map[string]any, 0, len(calls))
	for _, call := range calls {
		fn := map[string]any{"name": call.Name}
		if len(call.Args) > 0 {
			fn["arguments"] = call.Args
		} else {
			fn["arguments"] = map[string]any{}
		}
		entry := map[string]any{
			"type":     "function",
			"function": fn,
		}
		if call.ID != "" {
			entry["id"] = call.ID
		}
		out = append(out, entry)
	}
	return out
}

func convertTools(tools []core.LLMToolSpec) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  schemaToJSONSchema(tool.InputSchema),
			},
		})
	}
	return out
}

func schemaToJSONSchema(schema *core.Schema) map[string]any {
	if schema == nil {
		return map[string]any{"type": "object"}
	}
	out := map[string]any{}
	if schema.Type != "" {
		out["type"] = schema.Type
	}
	if schema.Description != "" {
		out["description"] = schema.Description
	}
	if schema.Default != nil {
		out["default"] = schema.Default
	}
	if len(schema.Enum) > 0 {
		out["enum"] = schema.Enum
	}
	if len(schema.Properties) > 0 {
		props := make(map[string]any, len(schema.Properties))
		for name, prop := range schema.Properties {
			props[name] = schemaToJSONSchema(prop)
		}
		out["properties"] = props
	}
	if schema.Items != nil {
		out["items"] = schemaToJSONSchema(schema.Items)
	}
	if len(schema.Required) > 0 {
		out["required"] = schema.Required
	}
	if len(out) == 0 {
		out["type"] = "object"
	}
	return out
}

func parseToolCalls(message *chatMessage) []core.ToolCall {
	if message == nil {
		return nil
	}
	calls := message.ToolCalls
	out := make([]core.ToolCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, core.ToolCall{
			ID:   call.ID,
			Name: call.Function.Name,
			Args: parseArgs(call.Function.Arguments),
		})
	}
	return out
}

func parseArgs(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err == nil {
		return obj
	}
	return map[string]any{"_raw": raw}
}

func decodeChatResponse(raw chatCompletionResponse) (*core.LLMResponse, error) {
	resp := &core.LLMResponse{}
	if len(raw.Choices) == 0 {
		resp.Usage = normalizeUsage(raw.Usage)
		return resp, nil
	}
	choice := raw.Choices[0]
	if choice.Message != nil {
		resp.Text = choice.Message.Content
		resp.ToolCalls = append(resp.ToolCalls, parseToolCalls(choice.Message)...)
	}
	resp.FinishReason = choice.FinishReason
	resp.Usage = normalizeUsage(raw.Usage)
	return resp, nil
}

func firstChoiceMessage(chunk chatCompletionChunk) *chatMessage {
	if len(chunk.Choices) == 0 {
		return nil
	}
	return chunk.Choices[0].Message
}

func firstFinishReason(chunk chatCompletionChunk) string {
	if len(chunk.Choices) == 0 {
		return ""
	}
	return chunk.Choices[0].FinishReason
}

func normalizeUsage(usage map[string]any) map[string]int {
	if len(usage) == 0 {
		return nil
	}
	out := make(map[string]int, len(usage))
	for k, v := range usage {
		switch value := v.(type) {
		case float64:
			out[k] = int(value)
		case int:
			out[k] = value
		case int64:
			out[k] = int(value)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func readHTTPError(resp *http.Response) error {
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	detail := strings.TrimSpace(string(msg))
	if detail != "" {
		return fmt.Errorf("openai-compatible error: %s: %s", resp.Status, detail)
	}
	return fmt.Errorf("openai-compatible error: %s", resp.Status)
}

type chatCompletionResponse struct {
	Choices []chatChoice   `json:"choices"`
	Usage   map[string]any `json:"usage"`
}

type chatCompletionChunk struct {
	Choices []chatChunkChoice `json:"choices"`
	Usage   map[string]any    `json:"usage"`
}

type chatChoice struct {
	Message      *chatMessage `json:"message"`
	FinishReason string       `json:"finish_reason"`
}

type chatChunkChoice struct {
	Delta        chatDelta    `json:"delta"`
	Message      *chatMessage `json:"message"`
	FinishReason string       `json:"finish_reason"`
}

type chatMessage struct {
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	ToolCalls []chatToolCall `json:"tool_calls"`
}

type chatDelta struct {
	Role      string              `json:"role"`
	Content   string              `json:"content"`
	ToolCalls []chatToolCallDelta `json:"tool_calls"`
}

type chatToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function chatFunction `json:"function"`
}

type chatToolCallDelta struct {
	Index    int               `json:"index"`
	ID       string            `json:"id"`
	Type     string            `json:"type"`
	Function chatFunctionDelta `json:"function"`
}

type chatFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatFunctionDelta struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type toolCallBuilder struct {
	Index int
	ID    string
	Type  string
	Name  string
	Args  strings.Builder
}

func (b *toolCallBuilder) Merge(delta chatToolCallDelta) {
	if delta.ID != "" {
		b.ID = delta.ID
	}
	if delta.Type != "" {
		b.Type = delta.Type
	}
	if delta.Function.Name != "" {
		b.Name = delta.Function.Name
	}
	if delta.Function.Arguments != "" {
		b.Args.WriteString(delta.Function.Arguments)
	}
}

func buildToolCalls(builders map[int]*toolCallBuilder) []core.ToolCall {
	if len(builders) == 0 {
		return nil
	}
	keys := make([]int, 0, len(builders))
	for k := range builders {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	out := make([]core.ToolCall, 0, len(keys))
	for _, k := range keys {
		builder := builders[k]
		out = append(out, core.ToolCall{
			ID:   builder.ID,
			Name: builder.Name,
			Args: parseArgs(builder.Args.String()),
		})
	}
	return out
}
