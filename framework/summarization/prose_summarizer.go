package summarization

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/knowledge"
)

// LanguageModel is the interface for language model operations.
// This is a minimal interface - real implementation would be in core package.
type LanguageModel interface {
	Complete(ctx context.Context, prompt string) (string, error)
	ModelID() string
}

// ProseSummarizer is model-based. Requires an LLM. Records model ID in provenance.
type ProseSummarizer struct {
	Model LanguageModel
}

// Kind returns the kind of summarizer.
func (s *ProseSummarizer) Kind() SummarizerKind {
	return SummarizerKindProse
}

// CanSummarize checks if this summarizer can handle the given chunks.
func (s *ProseSummarizer) CanSummarize(chunks []knowledge.KnowledgeChunk) bool {
	if len(chunks) == 0 {
		return false
	}
	// Can summarize any text content
	for _, chunk := range chunks {
		contentType := s.getContentType(chunk)
		switch contentType {
		case "text", "markdown", "html", "prose", "rst", "asciidoc", "":
			return true
		}
		// Also accept any non-code content as fallback
		if contentType != "go" && contentType != "python" &&
			contentType != "rust" && contentType != "javascript" &&
			contentType != "typescript" && contentType != "java" &&
			contentType != "c" && contentType != "cpp" &&
			contentType != "csharp" && contentType != "json" &&
			contentType != "yaml" {
			return true
		}
	}
	return false
}

// Summarize generates a model-based summary.
func (s *ProseSummarizer) Summarize(ctx context.Context, req SummarizationRequest) (*SummarizationResult, error) {
	if len(req.Chunks) == 0 {
		return nil, fmt.Errorf("no chunks to summarize")
	}

	if s.Model == nil {
		return nil, fmt.Errorf("no language model available")
	}

	// Combine all chunks into a single document
	var contents []string
	var sourceCoverage []knowledge.ChunkID
	var totalTokens int

	for _, chunk := range req.Chunks {
		content := s.getContent(chunk)
		contents = append(contents, content)
		sourceCoverage = append(sourceCoverage, chunk.ID)
		totalTokens += CountTokens(content)
	}

	combinedContent := strings.Join(contents, "\n\n")

	// Build structured prompt
	prompt := s.buildPrompt(combinedContent, req.TargetTokenBudget)

	// Call model
	summary, err := s.Model.Complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("model completion failed: %w", err)
	}

	// Compute coverage hash
	coverageHash := ComputeCoverageHash(req.Chunks)

	// Build derivation method with model ID and prompt fingerprint
	modelID := s.Model.ModelID()
	promptHash := s.computePromptHash(prompt)
	derivationMethod := fmt.Sprintf("prose_summarizer:%s:%s", modelID, promptHash[:8])

	return &SummarizationResult{
		Summary:          summary,
		TokenEstimate:    CountTokens(summary),
		DerivationMethod: derivationMethod,
		SourceCoverage:   sourceCoverage,
		CoverageHash:     coverageHash,
		UsedModel:        true,
	}, nil
}

// buildPrompt constructs a structured prompt for summarization.
func (s *ProseSummarizer) buildPrompt(content string, targetBudget int) string {
	var sb strings.Builder

	sb.WriteString("Please summarize the following content")

	if targetBudget > 0 {
		sb.WriteString(fmt.Sprintf(" in approximately %d tokens", targetBudget))
	}

	sb.WriteString(".\n\n")
	sb.WriteString("Guidelines:\n")
	sb.WriteString("- Preserve the main ideas and key points\n")
	sb.WriteString("- Maintain the overall structure if present\n")
	sb.WriteString("- Remove unnecessary details and examples\n")
	sb.WriteString("- Keep important facts and conclusions\n")
	sb.WriteString("\nContent to summarize:\n")
	sb.WriteString("---\n")
	sb.WriteString(content)
	sb.WriteString("\n---\n\n")
	sb.WriteString("Summary:\n")

	return sb.String()
}

// computePromptHash computes a simple hash of the prompt.
func (s *ProseSummarizer) computePromptHash(prompt string) string {
	hash := 0
	for _, ch := range prompt {
		hash = ((hash << 5) - hash) + int(ch)
	}
	return fmt.Sprintf("%x", hash)
}

// getContent extracts content from a chunk.
func (s *ProseSummarizer) getContent(chunk knowledge.KnowledgeChunk) string {
	return chunk.Body.Raw
}

// getContentType extracts the content type from a chunk.
func (s *ProseSummarizer) getContentType(chunk knowledge.KnowledgeChunk) string {
	if contentType, ok := chunk.Body.Fields["content_type"].(string); ok {
		return contentType
	}
	return ""
}

// NewProseSummarizer creates a new prose summarizer.
func NewProseSummarizer(model LanguageModel) *ProseSummarizer {
	return &ProseSummarizer{
		Model: model,
	}
}

// MockLanguageModel is a mock implementation for testing.
type MockLanguageModel struct {
	ModelIDValue string
	Response     string
}

// Complete implements LanguageModel.
func (m *MockLanguageModel) Complete(ctx context.Context, prompt string) (string, error) {
	if m.Response != "" {
		return m.Response, nil
	}
	return "This is a mock summary of the provided content.", nil
}

// ModelID implements LanguageModel.
func (m *MockLanguageModel) ModelID() string {
	if m.ModelIDValue != "" {
		return m.ModelIDValue
	}
	return "mock-model"
}
