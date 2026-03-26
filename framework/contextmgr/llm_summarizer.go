package contextmgr

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

const defaultSummarizerTimeout = 30 * time.Second

// LLMSummarizer implements core.Summarizer by issuing single-turn LLM calls.
// It embeds SimpleSummarizer as a fallback for errors and for SummaryFull
// (which returns content verbatim without a model call).
type LLMSummarizer struct {
	core.SimpleSummarizer
	lm      core.LanguageModel
	timeout time.Duration
}

// NewLLMSummarizer creates an LLMSummarizer backed by the given model.
func NewLLMSummarizer(lm core.LanguageModel) *LLMSummarizer {
	return &LLMSummarizer{lm: lm, timeout: defaultSummarizerTimeout}
}

// wordLimit maps a SummaryLevel to a word-count hint for the prompt.
func wordLimit(level core.SummaryLevel) int {
	switch level {
	case core.SummaryDetailed:
		return 100
	case core.SummaryConcise:
		return 50
	case core.SummaryMinimal:
		return 20
	default:
		return 150
	}
}

// generate issues a single-turn inference call and returns the trimmed response text.
func (s *LLMSummarizer) generate(prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	resp, err := s.lm.Generate(ctx, prompt, &core.LLMOptions{
		MaxTokens:   256,
		Temperature: 0.3,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Text), nil
}

// Summarize implements core.Summarizer.
// SummaryFull returns the content verbatim. All other levels issue an LLM call
// and fall back to SimpleSummarizer on error.
func (s *LLMSummarizer) Summarize(content string, level core.SummaryLevel) (string, error) {
	if content == "" {
		return "", nil
	}
	if level == core.SummaryFull {
		return content, nil
	}
	limit := wordLimit(level)
	prompt := fmt.Sprintf(
		"Summarize the following content in %d words or fewer. Return only the summary text, no preamble.\n\n%s",
		limit, content,
	)
	text, err := s.generate(prompt)
	if err != nil || text == "" {
		return s.SimpleSummarizer.Summarize(content, level)
	}
	return text, nil
}

// SummarizeFile implements core.Summarizer.
func (s *LLMSummarizer) SummarizeFile(path string, content string, level core.SummaryLevel) (*core.FileSummary, error) {
	if level == core.SummaryFull {
		return s.SimpleSummarizer.SummarizeFile(path, content, level)
	}
	limit := wordLimit(level)
	prompt := fmt.Sprintf(
		"Summarize the file %q in %d words or fewer. Focus on its purpose, key types or functions, and dependencies. Return only the summary text.\n\n%s",
		path, limit, content,
	)
	text, err := s.generate(prompt)
	if err != nil || text == "" {
		return s.SimpleSummarizer.SummarizeFile(path, content, level)
	}
	return &core.FileSummary{
		Path:       path,
		Level:      level,
		Summary:    text,
		TokenCount: len(text) / 4,
	}, nil
}

// SummarizeDirectory implements core.Summarizer.
func (s *LLMSummarizer) SummarizeDirectory(path string, files []core.FileSummary, level core.SummaryLevel) (*core.DirectorySummary, error) {
	if level == core.SummaryFull {
		return s.SimpleSummarizer.SummarizeDirectory(path, files, level)
	}
	var sb strings.Builder
	names := make([]string, 0, len(files))
	for _, f := range files {
		names = append(names, f.Path)
		if f.Summary != "" {
			fmt.Fprintf(&sb, "%s: %s\n", f.Path, f.Summary)
		}
	}
	limit := wordLimit(level)
	prompt := fmt.Sprintf(
		"Summarize the directory %q in %d words or fewer. Focus on the overall purpose and organisation of its files. Return only the summary text.\n\n%s",
		path, limit, sb.String(),
	)
	text, err := s.generate(prompt)
	if err != nil || text == "" {
		return s.SimpleSummarizer.SummarizeDirectory(path, files, level)
	}
	return &core.DirectorySummary{
		Path:       path,
		Level:      level,
		Summary:    text,
		Files:      names,
		TokenCount: len(text) / 4,
	}, nil
}

// SummarizeChunk implements core.Summarizer.
func (s *LLMSummarizer) SummarizeChunk(chunk core.CodeChunk, content string, level core.SummaryLevel) (*core.ChunkSummary, error) {
	if level == core.SummaryFull {
		return s.SimpleSummarizer.SummarizeChunk(chunk, content, level)
	}
	name := chunk.ID
	if chunk.Name != "" {
		name = chunk.Name
	}
	limit := wordLimit(level)
	prompt := fmt.Sprintf(
		"Summarize the code chunk %q in %d words or fewer. Focus on what it does. Return only the summary text.\n\n%s",
		name, limit, content,
	)
	text, err := s.generate(prompt)
	if err != nil || text == "" {
		return s.SimpleSummarizer.SummarizeChunk(chunk, content, level)
	}
	return &core.ChunkSummary{
		ChunkID:    chunk.ID,
		Level:      level,
		Summary:    text,
		TokenCount: len(text) / 4,
		Version:    chunk.ID,
	}, nil
}
