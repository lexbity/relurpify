package pretask

import (
	"context"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/retrieval"
)

// HypotheticalGenerator generates a grounded vocabulary sketch.
// It requires Stage 1 results to be populated — never called cold.
type HypotheticalGenerator struct {
	model    core.LanguageModel
	embedder retrieval.Embedder
	config   HypotheticalConfig
}

type HypotheticalConfig struct {
	MaxTokens   int     // cap on generated sketch (default 120)
	Temperature float64 // low temperature for consistency (default 0.1)
}

// Generate produces a vocabulary sketch grounded in Stage 1 evidence.
//
// Prompt strategy:
//   "Given this question and the following code signatures/knowledge from
//    this codebase, list the additional function names, types, and packages
//    that are likely relevant. Be terse. Use names that exist in this codebase."
//
// The prompt includes:
//   - The original query
//   - Signatures from Stage 1 CodeEvidence (DetailSignatureOnly — cheap tokens)
//   - Titles/summaries from Stage 1 KnowledgeEvidence
//
// The output is embedded immediately. Both Text and Embedding are returned.
//
// If model is nil, stage1 is empty, or generation fails, returns a
// HypotheticalSketch with Grounded=false. The pipeline continues gracefully.
func (g *HypotheticalGenerator) Generate(
	ctx context.Context,
	query string,
	stage1 Stage1Result,
) (HypotheticalSketch, error) {
	// Check if we have the necessary components
	if g.model == nil || g.embedder == nil {
		return HypotheticalSketch{Grounded: false}, nil
	}

	// Skip if stage1 has no evidence
	if len(stage1.CodeEvidence) == 0 && len(stage1.KnowledgeEvidence) == 0 {
		return HypotheticalSketch{Grounded: false}, nil
	}

	// Build the prompt
	prompt := g.buildPrompt(query, stage1)

	// Generate with the model
	response, err := g.model.Generate(ctx, prompt, &core.LLMOptions{
		Model:       "",
		Temperature: g.config.Temperature,
		MaxTokens:   g.config.MaxTokens,
		Stop:        []string{"\n\n", "---"},
	})
	if err != nil {
		return HypotheticalSketch{Grounded: false}, nil // Graceful degradation
	}

	text := strings.TrimSpace(response.Text)
	if text == "" {
		return HypotheticalSketch{Grounded: false}, nil
	}

	// Embed the generated text
	embeddings, err := g.embedder.Embed(ctx, []string{text})
	if err != nil || len(embeddings) == 0 {
		// If embedding fails, still return the text but mark as not grounded
		return HypotheticalSketch{
			Text:      text,
			Embedding: nil,
			Grounded:  false,
			TokenCount: len(strings.Fields(text)),
		}, nil
	}

	return HypotheticalSketch{
		Text:       text,
		Embedding:  embeddings[0],
		Grounded:   true,
		TokenCount: len(strings.Fields(text)),
	}, nil
}

func (g *HypotheticalGenerator) buildPrompt(query string, stage1 Stage1Result) string {
	var builder strings.Builder
	
	builder.WriteString("Given this question and the following code signatures/knowledge from ")
	builder.WriteString("this codebase, list the additional function names, types, and packages ")
	builder.WriteString("that are likely relevant. Be terse. Use names that exist in this codebase.\n\n")
	
	builder.WriteString("Question: ")
	builder.WriteString(query)
	builder.WriteString("\n\n")
	
	// Add code evidence summaries
	if len(stage1.CodeEvidence) > 0 {
		builder.WriteString("Code signatures:\n")
		for i, item := range stage1.CodeEvidence {
			if i >= 5 { // Limit to top 5
				break
			}
			builder.WriteString("- ")
			builder.WriteString(item.Summary)
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}
	
	// Add knowledge evidence summaries
	if len(stage1.KnowledgeEvidence) > 0 {
		builder.WriteString("Related knowledge:\n")
		for i, item := range stage1.KnowledgeEvidence {
			if i >= 3 { // Limit to top 3
				break
			}
			builder.WriteString("- ")
			builder.WriteString(item.Title)
			if item.Summary != "" {
				builder.WriteString(": ")
				builder.WriteString(item.Summary)
			}
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}
	
	builder.WriteString("Relevant vocabulary (function names, types, packages):\n")
	
	return builder.String()
}
