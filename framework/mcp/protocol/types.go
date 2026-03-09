package protocol

import "strings"

const (
	Revision20250618 = "2025-06-18"
	Revision20251125 = "2025-11-25"
)

type PeerInfo struct {
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
}

type InitializeRequest struct {
	ProtocolVersion string         `json:"protocolVersion"`
	ClientInfo      PeerInfo       `json:"clientInfo"`
	Capabilities    map[string]any `json:"capabilities,omitempty"`
}

type InitializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	ServerInfo      PeerInfo       `json:"serverInfo"`
	Capabilities    map[string]any `json:"capabilities,omitempty"`
	Instructions    string         `json:"instructions,omitempty"`
}

type Tool struct {
	Name         string         `json:"name"`
	Title        string         `json:"title,omitempty"`
	Description  string         `json:"description,omitempty"`
	InputSchema  map[string]any `json:"inputSchema,omitempty"`
	OutputSchema map[string]any `json:"outputSchema,omitempty"`
}

type Prompt struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
}

type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	MIMEType    string `json:"mimeType,omitempty"`
	Size        int64  `json:"size,omitempty"`
}

type ListToolsResult struct {
	Tools []Tool `json:"tools"`
}

type ListPromptsResult struct {
	Prompts []Prompt `json:"prompts"`
}

type ListResourcesResult struct {
	Resources []Resource `json:"resources"`
}

type GetPromptParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

type GetPromptResult struct {
	Description string         `json:"description,omitempty"`
	Messages    []ContentBlock `json:"messages,omitempty"`
}

type ReadResourceParams struct {
	URI string `json:"uri"`
}

type ReadResourceResult struct {
	Contents []ContentBlock `json:"contents,omitempty"`
}

type ResourceSubscribeParams struct {
	URI string `json:"uri"`
}

type ResourceUpdatedParams struct {
	URI string `json:"uri"`
}

type CallToolParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

type ContentBlock struct {
	Type     string         `json:"type"`
	Text     string         `json:"text,omitempty"`
	Data     map[string]any `json:"data,omitempty"`
	URI      string         `json:"uri,omitempty"`
	MIMEType string         `json:"mimeType,omitempty"`
	Blob     string         `json:"blob,omitempty"`
	Name     string         `json:"name,omitempty"`
}

type CallToolResult struct {
	Content           []ContentBlock `json:"content,omitempty"`
	StructuredContent map[string]any `json:"structuredContent,omitempty"`
	IsError           bool           `json:"isError,omitempty"`
}

type SamplingMessage struct {
	Role    string       `json:"role"`
	Content ContentBlock `json:"content"`
}

type SamplingModelHint struct {
	Name string `json:"name,omitempty"`
}

type SamplingModelPreferences struct {
	Hints                []SamplingModelHint `json:"hints,omitempty"`
	CostPriority         float64             `json:"costPriority,omitempty"`
	SpeedPriority        float64             `json:"speedPriority,omitempty"`
	IntelligencePriority float64             `json:"intelligencePriority,omitempty"`
}

type CreateMessageParams struct {
	Messages         []SamplingMessage         `json:"messages"`
	ModelPreferences *SamplingModelPreferences `json:"modelPreferences,omitempty"`
	SystemPrompt     string                    `json:"systemPrompt,omitempty"`
	MaxTokens        int                       `json:"maxTokens,omitempty"`
	Temperature      float64                   `json:"temperature,omitempty"`
	StopSequences    []string                  `json:"stopSequences,omitempty"`
}

type CreateMessageResult struct {
	Role       string       `json:"role"`
	Content    ContentBlock `json:"content"`
	Model      string       `json:"model,omitempty"`
	StopReason string       `json:"stopReason,omitempty"`
}

type ElicitationParams struct {
	Message         string         `json:"message"`
	RequestedSchema map[string]any `json:"requestedSchema,omitempty"`
}

type ElicitationResult struct {
	Action  string         `json:"action"`
	Content map[string]any `json:"content,omitempty"`
}

func NormalizeRevision(value string) string {
	return strings.TrimSpace(value)
}
