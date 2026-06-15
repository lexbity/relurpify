package ollama

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/platform/observability"
)

// ErrNativeToolCallingRequired is returned when ChatWithTools is invoked on a
// client that cannot speak native structured tool calling.
var ErrNativeToolCallingRequired = errors.New("native tool calling is required for ChatWithTools")

// Client implements LanguageModel for Ollama.
type Client struct {
	Endpoint          string
	Model             string
	client            *http.Client
	Debug             bool
	apiKey            string
	profile           *ModelProfile
	nativeToolCalling bool
}

type toolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type toolDef struct {
	Type     string       `json:"type"`
	Function toolFunction `json:"function"`
}

type ollamaToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Type      string          `json:"type"`
	Arguments json.RawMessage `json:"arguments"`
	Function  struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"function"`
}

type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls"`
}

type ollamaResponse struct {
	Text            string           `json:"text"`
	Response        string           `json:"response"`
	Message         *ollamaMessage   `json:"message"`
	ToolCalls       []ollamaToolCall `json:"tool_calls"`
	DoneReason      string           `json:"done_reason"`
	Usage           map[string]int   `json:"usage"`
	EvalCount       int              `json:"eval_count"`
	PromptEvalCount int              `json:"prompt_eval_count"`
}

// NewClientWithProfile builds a new Ollama client with an active model profile.
func NewClientWithProfile(endpoint, model string, p *ModelProfile) *Client {
	c := NewClient(endpoint, model, "")
	c.profile = p
	return c
}

// NewClient builds a new Ollama client.
func NewClient(endpoint, model, apiKey string) *Client {
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}
	return &Client{
		Endpoint:          endpoint,
		Model:             model,
		apiKey:            strings.TrimSpace(apiKey),
		nativeToolCalling: true,
		client: &http.Client{
			Timeout: 3 * time.Minute,
		},
	}
}

// Generate implements single prompt completion.
func (c *Client) Generate(ctx context.Context, prompt string, options *LLMOptions) (*LLMResponse, error) {
	payload := map[string]any{
		"model":  c.model(options),
		"prompt": prompt,
		"stream": false,
	}
	c.applyOptions(payload, options)
	return c.doRequest(ctx, "/api/generate", payload)
}

// GenerateStream returns a simple streaming channel.
func (c *Client) GenerateStream(ctx context.Context, prompt string, options *LLMOptions) (<-chan string, error) {
	payload := map[string]any{
		"model":  c.model(options),
		"prompt": prompt,
		"stream": true,
	}
	c.applyOptions(payload, options)
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.ollamaAPIEndpoint()+"/api/generate", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.getHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	ch := make(chan string)
	go func() {
		scanner := bufio.NewScanner(bytes.NewReader(bodyBytes))
		for scanner.Scan() {
			ch <- scanner.Text()
		}
		close(ch)
	}()
	return ch, nil
}

// Chat implements chat style conversation.
func (c *Client) Chat(ctx context.Context, messages []Message, options *LLMOptions) (*LLMResponse, error) {
	payload := map[string]any{
		"model":    c.model(options),
		"messages": convertMessages(messages),
		"stream":   false,
	}
	c.applyOptions(payload, options)
	return c.doRequest(ctx, "/api/chat", payload)
}

// ChatWithTools handles tool calling metadata.
func (c *Client) ChatWithTools(ctx context.Context, messages []Message, tools []LLMToolSpec, options *LLMOptions) (*LLMResponse, error) {
	if !c.nativeToolCallingEnabled() {
		return nil, ErrNativeToolCallingRequired
	}
	payload := map[string]any{
		"model":    c.model(options),
		"tools":    convertLLMToolSpecs(tools),
		"stream":   false,
		"messages": convertMessages(messages),
	}
	c.applyOptions(payload, options)
	if options != nil && options.StreamCallback != nil {
		return c.doRequestStream(ctx, "/api/chat", payload, options.StreamCallback)
	}
	return c.doRequest(ctx, "/api/chat", payload)
}

// SetDebugLogging enables or disables verbose logging for requests/responses.
func (c *Client) SetDebugLogging(enabled bool) {
	c.Debug = enabled
}

// SetProfile sets the model profile for this client.
func (c *Client) SetProfile(p *ModelProfile) {
	c.profile = p
}

// ContextSize reports the profile override when present.
func (c *Client) ContextSize() int {
	if c == nil || c.profile == nil {
		return 0
	}
	return c.profile.ContextSize
}

// SetNativeToolCalling updates the transport capability flag.
func (c *Client) SetNativeToolCalling(enabled bool) {
	c.nativeToolCalling = enabled
}

// ToolRepairStrategy implements ProfiledModel.
func (c *Client) ToolRepairStrategy() string {
	if c.profile == nil {
		return "heuristic-only"
	}
	if c.profile.Repair.Strategy == "" {
		return "heuristic-only"
	}
	return c.profile.Repair.Strategy
}

// MaxToolsPerCall implements ProfiledModel.
func (c *Client) MaxToolsPerCall() int {
	if c.profile == nil {
		return 0
	}
	return c.profile.ToolCalling.MaxToolsPerCall
}

// UsesNativeToolCalling implements ProfiledModel.
func (c *Client) UsesNativeToolCalling() bool {
	return c.nativeToolCallingEnabled()
}

func (c *Client) nativeToolCallingEnabled() bool {
	if c == nil {
		return false
	}
	if !c.nativeToolCalling {
		return false
	}
	if c.profile == nil {
		return true
	}
	return c.profile.ToolCalling.NativeAPI
}

func (c *Client) getHTTPClient() *http.Client {
	if c.client != nil {
		return c.client
	}
	c.client = &http.Client{Timeout: 60 * time.Second}
	return c.client
}

func (c *Client) model(options *LLMOptions) string {
	if options != nil && options.Model != "" {
		return options.Model
	}
	if c.Model != "" {
		return c.Model
	}
	return "codellama"
}

func (c *Client) applyOptions(payload map[string]any, options *LLMOptions) {
	if options == nil {
		return
	}
	opts := map[string]any{}
	if options.Temperature != 0 {
		opts["temperature"] = options.Temperature
	}
	if options.MaxTokens != 0 {
		opts["num_predict"] = options.MaxTokens
	}
	if options.Stop != nil {
		opts["stop"] = options.Stop
	}
	if options.TopP != 0 {
		opts["top_p"] = options.TopP
	}
	if len(opts) > 0 {
		payload["options"] = opts
	}
}

func (c *Client) ollamaAPIEndpoint() string {
	endpoint := strings.TrimSpace(c.Endpoint)
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return strings.TrimRight(endpoint, "/")
	}
	if parsed.Path != "" {
		clean := path.Clean(parsed.Path)
		if clean == "/v1" {
			parsed.Path = ""
		}
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}

func (c *Client) doRequest(ctx context.Context, apiPath string, payload any) (*LLMResponse, error) {
	promptTokens := estimatePromptTokensFromPayload(payload)
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	c.logPayload(apiPath, body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.ollamaAPIEndpoint()+apiPath, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.getHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		detail := strings.TrimSpace(string(msg))
		if detail != "" {
			return nil, fmt.Errorf("ollama error: %s: %s", resp.Status, detail)
		}
		return nil, fmt.Errorf("ollama error: %s", resp.Status)
	}
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	c.logResponse(apiPath, responseBody)
	return c.decodeLLMResponse(bytes.NewReader(responseBody), promptTokens)
}

func (c *Client) doRequestStream(ctx context.Context, apiPath string, payload map[string]any, callback func(string)) (*LLMResponse, error) {
	payload["stream"] = true
	promptTokens := estimatePromptTokensFromPayload(payload)
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	c.logPayload(apiPath, body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.ollamaAPIEndpoint()+apiPath, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.getHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		detail := strings.TrimSpace(string(msg))
		if detail != "" {
			return nil, fmt.Errorf("ollama error: %s: %s", resp.Status, detail)
		}
		return nil, fmt.Errorf("ollama error: %s", resp.Status)
	}
	var fullText strings.Builder
	var finalChunk ollamaResponse
	scanner := bufio.NewScanner(resp.Body)
	scanBuf := make([]byte, 0, 64*1024)
	scanner.Buffer(scanBuf, 512*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var chunk ollamaResponse
		if err := json.Unmarshal(line, &chunk); err != nil {
			continue
		}
		token := ""
		if chunk.Message != nil {
			token = chunk.Message.Content
		}
		if token != "" {
			fullText.WriteString(token)
			if callback != nil {
				callback(token)
			}
		}
		finalChunk = chunk
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	result := &LLMResponse{
		Text:         fullText.String(),
		FinishReason: finalChunk.DoneReason,
		Usage:        normalizeUsage(finalChunk, promptTokens, fullText.String()),
	}
	if finalChunk.Message != nil {
		result.ToolCalls = append(result.ToolCalls, c.parseToolCalls(finalChunk.Message.ToolCalls)...)
	}
	result.ToolCalls = append(result.ToolCalls, c.parseToolCalls(finalChunk.ToolCalls)...)
	return result, nil
}

func convertMessages(messages []Message) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		m := map[string]any{
			"role":    msg.Role,
			"content": msg.Content,
		}
		if msg.Name != "" {
			m["name"] = msg.Name
			if msg.Role == "tool" {
				m["tool_name"] = msg.Name
			}
		}
		if msg.ToolCallID != "" {
			m["tool_call_id"] = msg.ToolCallID
		}
		if len(msg.ToolCalls) > 0 {
			calls := make([]map[string]any, 0, len(msg.ToolCalls))
			for _, call := range msg.ToolCalls {
				fn := map[string]any{
					"name": call.Name,
				}
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
				calls = append(calls, entry)
			}
			m["tool_calls"] = calls
		}
		out = append(out, m)
	}
	return out
}

func convertLLMToolSpecs(specs []LLMToolSpec) []toolDef {
	res := make([]toolDef, 0, len(specs))
	for _, spec := range specs {
		res = append(res, toolDef{
			Type: "function",
			Function: toolFunction{
				Name:        spec.Name,
				Description: spec.Description,
				Parameters:  schemaToOllamaParameters(spec.InputSchema),
			},
		})
	}
	return res
}

func schemaToOllamaParameters(schema *Schema) map[string]any {
	props := make(map[string]any)
	var required []string
	if schema != nil && schema.Type == "object" {
		for name, prop := range schema.Properties {
			if prop == nil {
				continue
			}
			p := map[string]any{
				"type":        prop.Type,
				"description": prop.Description,
			}
			if prop.Default != nil {
				p["default"] = prop.Default
			}
			props[name] = p
		}
		required = append(required, schema.Required...)
	}
	parameters := map[string]any{
		"type":       "object",
		"properties": props,
	}
	if len(required) > 0 {
		parameters["required"] = required
	}
	return parameters
}

func (c *Client) decodeLLMResponse(body io.Reader, promptTokens int) (*LLMResponse, error) {
	var raw ollamaResponse
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return nil, err
	}
	resp := &LLMResponse{
		Text:         cmp.Or(raw.Text, raw.Response),
		FinishReason: raw.DoneReason,
		Usage:        normalizeUsage(raw, promptTokens, cmp.Or(raw.Text, raw.Response)),
	}
	if resp.Text == "" && raw.Message != nil {
		resp.Text = raw.Message.Content
		resp.Usage = normalizeUsage(raw, promptTokens, resp.Text)
	}
	resp.ToolCalls = append(resp.ToolCalls, c.parseToolCalls(raw.ToolCalls)...)
	if raw.Message != nil {
		resp.ToolCalls = append(resp.ToolCalls, c.parseToolCalls(raw.Message.ToolCalls)...)
	}
	return resp, nil
}

func (c *Client) parseToolCalls(calls []ollamaToolCall) []model.ToolCall {
	results := make([]model.ToolCall, 0, len(calls))
	for _, call := range calls {
		name := call.Name
		args := call.Arguments
		if call.Function.Name != "" {
			name = call.Function.Name
		}
		if len(call.Function.Arguments) > 0 {
			args = call.Function.Arguments
		}
		parsedArgs := c.parseArguments(args)
		results = append(results, model.ToolCall{
			ID:   call.ID,
			Name: name,
			Args: parsedArgs,
		})
	}
	return results
}

func (c *Client) parseArguments(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil {
		return obj
	}
	if c.profile != nil && c.profile.ToolCalling.DoubleEncodedArgs {
		var str string
		if err := json.Unmarshal(raw, &str); err == nil {
			var nested map[string]any
			if err := json.Unmarshal([]byte(str), &nested); err == nil {
				return nested
			}
			return map[string]any{"value": str}
		}
	}
	return map[string]any{"_raw": string(raw)}
}

func normalizeUsage(raw ollamaResponse, promptTokens int, responseText string) model.TokenUsage {
	report := model.TokenUsage{}
	if len(raw.Usage) > 0 {
		for k, v := range raw.Usage {
			switch strings.ToLower(k) {
			case "prompt_tokens", "prompt_eval_count":
				report.PromptTokens = v
			case "completion_tokens", "eval_count":
				report.CompletionTokens = v
			case "total_tokens":
				report.TotalTokens = v
			}
		}
		if report.TotalTokens == 0 {
			report.TotalTokens = report.PromptTokens + report.CompletionTokens
		}
		if report.PromptTokens > 0 || report.CompletionTokens > 0 || report.TotalTokens > 0 {
			return report
		}
	}
	if raw.EvalCount > 0 {
		report.CompletionTokens = raw.EvalCount
	}
	if raw.PromptEvalCount > 0 {
		report.PromptTokens = raw.PromptEvalCount
	}
	if report.PromptTokens > 0 || report.CompletionTokens > 0 {
		report.TotalTokens = report.PromptTokens + report.CompletionTokens
		return report
	}
	return estimateUsage(promptTokens, responseText)
}

func estimatePromptTokensFromPayload(payload any) int {
	switch p := payload.(type) {
	case map[string]any:
		if prompt, ok := p["prompt"].(string); ok && prompt != "" {
			return observability.EstimateTokens(prompt)
		}
		return estimatePromptTokensFromMessages(p["messages"])
	default:
		return 0
	}
}

func estimatePromptTokensFromMessages(value any) int {
	switch msgs := value.(type) {
	case []map[string]any:
		total := 0
		for _, msg := range msgs {
			if content, ok := msg["content"].(string); ok {
				total += observability.EstimateTokens(content)
			}
		}
		return total
	case []any:
		total := 0
		for _, item := range msgs {
			msg, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if content, ok := msg["content"].(string); ok {
				total += observability.EstimateTokens(content)
			}
		}
		return total
	default:
		return 0
	}
}

func estimateUsage(promptTokens int, responseText string) model.TokenUsage {
	completionTokens := observability.EstimateTokens(responseText)
	return model.TokenUsage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
		Estimated:        true,
		EstimationMethod: "char_div_4",
	}
}

func (c *Client) logPayload(path string, payload []byte) {
	if !c.Debug {
		return
	}
	c.logf("request %s payload: %s", path, truncate(string(payload), 2048))
}

func (c *Client) logResponse(path string, resp []byte) {
	if !c.Debug {
		return
	}
	c.logf("response %s payload: %s", path, truncate(string(resp), 2048))
}

func (c *Client) logf(format string, args ...any) {
	if !c.Debug {
		return
	}
	log.Printf("[ollama] "+format, args...)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}
