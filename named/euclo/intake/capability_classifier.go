package intake

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Local minimal LLM definitions to avoid external dependency.

// LanguageModel abstracts an LLM capable of completing prompts.
type LanguageModel interface {
	Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
}

// CompletionRequest represents a request to the LLM.
type CompletionRequest struct {
	Prompt      string
	MaxTokens   int
	Temperature float64
}

// CompletionResponse holds the LLM's generated text.
type CompletionResponse struct {
	Text string
}

// LLMCapabilityClassifier asks a model to select a single capability route target.
type LLMCapabilityClassifier struct {
	model       LanguageModel
	maxTokens   int
	temperature float64
}

// NewLLMCapabilityClassifier creates a new classifier with sensible defaults.
func NewLLMCapabilityClassifier(model LanguageModel) *LLMCapabilityClassifier {
	return &LLMCapabilityClassifier{
		model:       model,
		maxTokens:   256,
		temperature: 0.1,
	}
}

// Classify performs LLM-based capability selection and returns one capability ID.
func (c *LLMCapabilityClassifier) Classify(ctx context.Context, instruction string, familyID string, streamedContext string, negativeConstraints []string) (string, string, error) {
	if c.model == nil {
		return "", "", fmt.Errorf("no language model provided for capability classification")
	}
	prompt := c.buildPrompt(instruction, familyID, streamedContext, negativeConstraints)
	resp, err := c.model.Complete(ctx, CompletionRequest{Prompt: prompt, MaxTokens: c.maxTokens, Temperature: c.temperature})
	if err != nil {
		return "", "", fmt.Errorf("LLM completion failed: %w", err)
	}
	return c.parseResponse(resp.Text)
}

// buildPrompt constructs the prompt sent to the LLM.
func (c *LLMCapabilityClassifier) buildPrompt(instruction, familyID, streamedContext string, negativeConstraints []string) string {
	var b strings.Builder
	b.WriteString("You are a task classifier for a coding assistant. Select one capability ID.\n\n")
	b.WriteString("Task: ")
	b.WriteString(instruction)
	b.WriteString("\n\n")
	if strings.TrimSpace(familyID) != "" {
		b.WriteString("Winning family: ")
		b.WriteString(strings.TrimSpace(familyID))
		b.WriteString("\n\n")
	}
	if streamedContext != "" {
		b.WriteString("Context:\n")
		b.WriteString(streamedContext)
		b.WriteString("\n\n")
	}
	if len(negativeConstraints) > 0 {
		b.WriteString("Constraints (DO NOT use capabilities that violate these):\n")
		for _, cst := range negativeConstraints {
			b.WriteString("- ")
			b.WriteString(cst)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("Respond with ONLY a JSON object in this format:\n")
	b.WriteString(`{"capability_id": "cap_id"}`)
	b.WriteString("\n")
	return b.String()
}

// parseResponse extracts the JSON payload from the LLM response.
func (c *LLMCapabilityClassifier) parseResponse(text string) (string, string, error) {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start == -1 || end == -1 || end <= start {
		return "", "", fmt.Errorf("no JSON object found in LLM response")
	}
	jsonStr := text[start : end+1]
	var payload struct {
		CapabilityID string `json:"capability_id"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &payload); err != nil {
		return "", "", fmt.Errorf("failed to parse JSON response: %w", err)
	}
	capabilityID := strings.TrimSpace(payload.CapabilityID)
	if capabilityID == "" {
		return "", "", fmt.Errorf("no capability id returned by LLM")
	}
	return capabilityID, "llm", nil
}
