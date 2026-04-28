package summarization

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/contextpolicy"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
)

// DefaultDerivationGenerationCap is the maximum number of summarization generations.
const DefaultDerivationGenerationCap = 3

// Route selects the appropriate summarizer for a chunk set and invokes it.
// It enforces generation cap before selecting a summarizer.
func Route(ctx context.Context, chunks []knowledge.KnowledgeChunk, budget int, summarizers []Summarizer, policy *contextpolicy.ContextPolicyBundle) (*SummarizationResult, error) {
	if len(chunks) == 0 {
		return nil, fmt.Errorf("no chunks to summarize")
	}

	// 1. Enforce generation cap
	generationCap := DefaultDerivationGenerationCap
	if policy != nil && policy.BudgetShortfallPolicy != "" {
		// Could be configured via policy - for now use default
	}

	for _, chunk := range chunks {
		if chunk.DerivationGeneration >= generationCap {
			return nil, &GenerationCapError{
				Generation: chunk.DerivationGeneration,
				Cap:        generationCap,
			}
		}
	}

	// 2. Select appropriate summarizer
	if len(summarizers) == 0 {
		// Create default summarizers
		summarizers = []Summarizer{
			NewASTSummarizer(),
			NewHeadingSummarizer(),
		}
	}

	var selectedSummarizer Summarizer
	var selectedKind SummarizerKind

	// Try to find a suitable summarizer
	for _, summarizer := range summarizers {
		if summarizer.CanSummarize(chunks) {
			selectedSummarizer = summarizer
			selectedKind = summarizer.Kind()
			break
		}
	}

	if selectedSummarizer == nil {
		return nil, fmt.Errorf("no suitable summarizer found for chunks")
	}

	// 3. Build request
	req := SummarizationRequest{
		Chunks:            chunks,
		Kind:              selectedKind,
		TargetTokenBudget: budget,
	}

	if len(chunks) > 0 {
		req.SourceOrigin = chunks[0].SourceOrigin
	}

	// 4. Invoke summarizer
	result, err := selectedSummarizer.Summarize(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("summarization failed: %w", err)
	}

	return result, nil
}

// RouteWithKind selects a specific summarizer by kind and invokes it.
func RouteWithKind(ctx context.Context, chunks []knowledge.KnowledgeChunk, kind SummarizerKind, budget int, summarizers []Summarizer, policy *contextpolicy.ContextPolicyBundle) (*SummarizationResult, error) {
	if len(chunks) == 0 {
		return nil, fmt.Errorf("no chunks to summarize")
	}

	// 1. Enforce generation cap
	generationCap := DefaultDerivationGenerationCap
	for _, chunk := range chunks {
		if chunk.DerivationGeneration >= generationCap {
			return nil, &GenerationCapError{
				Generation: chunk.DerivationGeneration,
				Cap:        generationCap,
			}
		}
	}

	// 2. Find the requested summarizer kind
	var selectedSummarizer Summarizer
	for _, summarizer := range summarizers {
		if summarizer.Kind() == kind {
			selectedSummarizer = summarizer
			break
		}
	}

	// If not found, create one
	if selectedSummarizer == nil {
		switch kind {
		case SummarizerKindAST:
			selectedSummarizer = NewASTSummarizer()
		case SummarizerKindHeading:
			selectedSummarizer = NewHeadingSummarizer()
		case SummarizerKindProse:
			return nil, fmt.Errorf("prose summarizer requires a language model")
		default:
			return nil, fmt.Errorf("unknown summarizer kind: %s", kind)
		}
	}

	// 3. Build request
	req := SummarizationRequest{
		Chunks:            chunks,
		Kind:              kind,
		TargetTokenBudget: budget,
	}

	if len(chunks) > 0 {
		req.SourceOrigin = chunks[0].SourceOrigin
	}

	// 4. Invoke summarizer
	result, err := selectedSummarizer.Summarize(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("summarization failed: %w", err)
	}

	return result, nil
}

// SelectSummarizer selects the best summarizer for the given content.
func SelectSummarizer(chunks []knowledge.KnowledgeChunk, available []Summarizer) (Summarizer, error) {
	if len(available) == 0 {
		return nil, fmt.Errorf("no summarizers available")
	}

	// Try each summarizer
	for _, summarizer := range available {
		if summarizer.CanSummarize(chunks) {
			return summarizer, nil
		}
	}

	// No suitable summarizer found
	return nil, fmt.Errorf("no suitable summarizer found")
}

// CreateDefaultSummarizers creates a default set of summarizers.
func CreateDefaultSummarizers() []Summarizer {
	return []Summarizer{
		NewASTSummarizer(),
		NewHeadingSummarizer(),
	}
}
