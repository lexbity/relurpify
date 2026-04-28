package summarization

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/knowledge"
)

// HeadingSummarizer preserves heading structure and ledes;
// elides paragraph bodies.
type HeadingSummarizer struct {
	SentencesPerSection int // default 2
}

// Kind returns the kind of summarizer.
func (s *HeadingSummarizer) Kind() SummarizerKind {
	return SummarizerKindHeading
}

// CanSummarize checks if this summarizer can handle the given chunks.
func (s *HeadingSummarizer) CanSummarize(chunks []knowledge.KnowledgeChunk) bool {
	if len(chunks) == 0 {
		return false
	}
	// Check if any chunk has markdown or structured content type
	for _, chunk := range chunks {
		contentType := s.getContentType(chunk)
		switch contentType {
		case "markdown", "text", "html", "rst", "asciidoc":
			return true
		}
		// Also check if content looks like markdown (has headings)
		content := s.getContent(chunk)
		if s.hasMarkdownHeadings(content) {
			return true
		}
	}
	return false
}

// Summarize generates a summary preserving headings and first sentences.
func (s *HeadingSummarizer) Summarize(ctx context.Context, req SummarizationRequest) (*SummarizationResult, error) {
	if len(req.Chunks) == 0 {
		return nil, fmt.Errorf("no chunks to summarize")
	}

	if s.SentencesPerSection <= 0 {
		s.SentencesPerSection = 2
	}

	var summaries []string
	var sourceCoverage []knowledge.ChunkID

	for _, chunk := range req.Chunks {
		content := s.getContent(chunk)
		summary := s.summarizeWithHeadings(content)
		summaries = append(summaries, summary)
		sourceCoverage = append(sourceCoverage, chunk.ID)
	}

	combined := strings.Join(summaries, "\n\n")

	// Compute coverage hash
	coverageHash := ComputeCoverageHash(req.Chunks)

	// Build derivation method fingerprint
	derivationMethod := fmt.Sprintf("heading_summarizer:%d:%s", s.SentencesPerSection, coverageHash[:8])

	return &SummarizationResult{
		Summary:          combined,
		TokenEstimate:    CountTokens(combined),
		DerivationMethod: derivationMethod,
		SourceCoverage:   sourceCoverage,
		CoverageHash:     coverageHash,
		UsedModel:        false,
	}, nil
}

// summarizeWithHeadings parses and summarizes content with headings.
func (s *HeadingSummarizer) summarizeWithHeadings(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	var currentSection []string
	var inParagraph bool

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check for heading (markdown style)
		if s.isHeading(trimmed) {
			// Finish previous section
			if len(currentSection) > 0 {
				result = append(result, s.summarizeSection(currentSection))
				currentSection = nil
			}
			// Add heading
			result = append(result, line)
			inParagraph = false
			continue
		}

		// List items - preserve structure but summarize content
		if strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "*") ||
			strings.HasPrefix(trimmed, "1.") || strings.HasPrefix(trimmed, "2.") {
			currentSection = append(currentSection, line)
			inParagraph = false
			continue
		}

		// Paragraph content
		if trimmed != "" {
			if !inParagraph {
				currentSection = append(currentSection, line)
				inParagraph = true
			} else {
				currentSection = append(currentSection, line)
			}
		} else {
			// Empty line - paragraph break
			if inParagraph {
				inParagraph = false
			}
		}
	}

	// Handle final section
	if len(currentSection) > 0 {
		result = append(result, s.summarizeSection(currentSection))
	}

	return strings.Join(result, "\n")
}

// isHeading checks if a line is a markdown heading.
func (s *HeadingSummarizer) isHeading(line string) bool {
	if strings.HasPrefix(line, "#") {
		return true
	}
	// Check for underline style headings (e.g., "====" or "----")
	if strings.HasPrefix(line, "=") || strings.HasPrefix(line, "-") {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) > 3 && strings.Trim(trimmed, "=") == "" {
			return true
		}
		if len(trimmed) > 3 && strings.Trim(trimmed, "-") == "" {
			return true
		}
	}
	return false
}

// hasMarkdownHeadings checks if content contains markdown headings.
func (s *HeadingSummarizer) hasMarkdownHeadings(content string) bool {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if s.isHeading(strings.TrimSpace(line)) {
			return true
		}
	}
	return false
}

// summarizeSection summarizes a section of content.
func (s *HeadingSummarizer) summarizeSection(lines []string) string {
	// Extract first N sentences from the section
	content := strings.Join(lines, " ")
	sentences := s.extractSentences(content)

	var result []string

	// Preserve list items
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "*") ||
			strings.HasPrefix(trimmed, "1.") || strings.HasPrefix(trimmed, "2.") {
			result = append(result, line)
		}
	}

	// Add first N sentences
	if len(sentences) > 0 {
		numSentences := s.SentencesPerSection
		if numSentences > len(sentences) {
			numSentences = len(sentences)
		}
		firstSentences := strings.Join(sentences[:numSentences], " ")
		result = append(result, firstSentences)
	}

	return strings.Join(result, "\n")
}

// extractSentences extracts sentences from text.
func (s *HeadingSummarizer) extractSentences(text string) []string {
	// Simple sentence extraction - split on period followed by space or end
	var sentences []string
	current := ""

	for i, ch := range text {
		current += string(ch)
		if ch == '.' || ch == '!' || ch == '?' {
			// Check if next char is space or end of string
			if i == len(text)-1 || text[i+1] == ' ' || text[i+1] == '\n' {
				sentences = append(sentences, strings.TrimSpace(current))
				current = ""
			}
		}
	}

	if strings.TrimSpace(current) != "" {
		sentences = append(sentences, strings.TrimSpace(current))
	}

	return sentences
}

// getContent extracts content from a chunk.
func (s *HeadingSummarizer) getContent(chunk knowledge.KnowledgeChunk) string {
	return chunk.Body.Raw
}

// getContentType extracts the content type from a chunk.
func (s *HeadingSummarizer) getContentType(chunk knowledge.KnowledgeChunk) string {
	if contentType, ok := chunk.Body.Fields["content_type"].(string); ok {
		return contentType
	}
	return ""
}

// NewHeadingSummarizer creates a new heading summarizer.
func NewHeadingSummarizer() *HeadingSummarizer {
	return &HeadingSummarizer{
		SentencesPerSection: 2,
	}
}
