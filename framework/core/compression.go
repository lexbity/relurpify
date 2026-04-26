package core

import (
	"encoding/json"
	"fmt"
	"strings"
)

type SimpleCompressionStrategy struct {
	KeepRecentCount int
}

func NewSimpleCompressionStrategy() *SimpleCompressionStrategy {
	return &SimpleCompressionStrategy{KeepRecentCount: 5}
}

func (s *SimpleCompressionStrategy) KeepRecent() int {
	if s == nil || s.KeepRecentCount <= 0 {
		return 5
	}
	return s.KeepRecentCount
}

func (s *SimpleCompressionStrategy) ShouldCompress(ctx *Context, budget *ArtifactBudget) bool {
	if ctx == nil {
		return false
	}
	if budget != nil && budget.GetCurrentUsage().ArtifactUsagePercent >= 0.8 {
		return true
	}
	return len(ctx.History()) > s.KeepRecent()
}

func (s *SimpleCompressionStrategy) EstimateTokens(cc *CompressedContext) int {
	if cc == nil {
		return 0
	}
	return cc.CompressedTokens
}

func (s *SimpleCompressionStrategy) Compress(interactions []Interaction, llm LanguageModel) (*CompressedContext, error) {
	if len(interactions) == 0 {
		return &CompressedContext{}, nil
	}
	raw := make([]string, 0, len(interactions))
	for _, item := range interactions {
		raw = append(raw, fmt.Sprintf("%s: %s", item.Role, item.Content))
	}
	prompt := "Summarize the following conversation and extract key facts as JSON.\n" + strings.Join(raw, "\n")
	summary := strings.TrimSpace(strings.Join(raw, "\n"))
	var facts []KeyFact
	if llm != nil {
		if resp, err := llm.Generate(nil, prompt, &LLMOptions{Temperature: 0.1, MaxTokens: 400}); err == nil && resp != nil {
			summary, facts = parseCompressedResponse(resp.Text, summary)
		}
	}
	if summary == "" {
		summary = truncateParagraph(strings.Join(raw, "\n"), 200)
	}
	originalTokens := estimateTokens(interactions)
	compressedTokens := estimateTokens(summary) + estimateTokens(facts)
	if compressedTokens >= originalTokens && originalTokens > 0 {
		compressedTokens = maxInt(1, originalTokens/2)
	}
	return &CompressedContext{
		Summary:          summary,
		KeyFacts:         facts,
		OriginalTokens:   originalTokens,
		CompressedTokens: compressedTokens,
	}, nil
}

func parseCompressedResponse(text, fallback string) (string, []KeyFact) {
	text = strings.TrimSpace(text)
	if text == "" {
		return fallback, nil
	}
	lines := strings.Split(text, "\n")
	summary := fallback
	facts := []KeyFact{}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(strings.ToLower(trimmed), "summary:"):
			summary = strings.TrimSpace(trimmed[len("summary:"):])
		case strings.HasPrefix(strings.ToLower(trimmed), "key facts:"):
			raw := strings.TrimSpace(trimmed[len("key facts:"):])
			if raw == "" {
				continue
			}
			var parsed []KeyFact
			if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
				facts = append(facts, parsed...)
				continue
			}
			facts = append(facts, KeyFact{Type: "fact", Content: raw})
		}
	}
	if summary == "" {
		summary = fallback
	}
	return summary, facts
}
