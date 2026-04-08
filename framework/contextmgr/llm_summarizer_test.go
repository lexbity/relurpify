package contextmgr

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

// stubLM is a minimal LanguageModel that records the prompt and returns a
// preset response. It is used to verify that LLMSummarizer sends the right
// prompt and uses the returned text as the summary.
type stubLM struct {
	response   string
	err        error
	lastPrompt string
}

func (s *stubLM) Generate(_ context.Context, prompt string, _ *core.LLMOptions) (*core.LLMResponse, error) {
	s.lastPrompt = prompt
	if s.err != nil {
		return nil, s.err
	}
	return &core.LLMResponse{Text: s.response}, nil
}
func (s *stubLM) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	return nil, nil
}
func (s *stubLM) Chat(context.Context, []core.Message, *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, nil
}
func (s *stubLM) ChatWithTools(context.Context, []core.Message, []core.LLMToolSpec, *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, nil
}

// TestLLMSummarizerSummarize verifies the prompt includes the content and level hint.
func TestLLMSummarizerSummarize(t *testing.T) {
	lm := &stubLM{response: "a short summary"}
	s := NewLLMSummarizer(lm)

	result, err := s.Summarize("some long content here", core.SummaryConcise)
	require.NoError(t, err)
	require.Equal(t, "a short summary", result)
	require.Contains(t, lm.lastPrompt, "some long content here")
	require.Contains(t, lm.lastPrompt, fmt.Sprint(wordLimit(core.SummaryConcise)))
}

// TestLLMSummarizerSummarizeFullPassthrough verifies SummaryFull skips the LLM.
func TestLLMSummarizerSummarizeFullPassthrough(t *testing.T) {
	lm := &stubLM{response: "should not be used"}
	s := NewLLMSummarizer(lm)

	result, err := s.Summarize("verbatim content", core.SummaryFull)
	require.NoError(t, err)
	require.Equal(t, "verbatim content", result)
	require.Empty(t, lm.lastPrompt) // no LLM call made
}

// TestLLMSummarizerSummarizeFallback verifies fallback to SimpleSummarizer on LLM error.
func TestLLMSummarizerSummarizeFallback(t *testing.T) {
	lm := &stubLM{err: fmt.Errorf("model offline")}
	s := NewLLMSummarizer(lm)

	content := strings.Repeat("word ", 50)
	result, err := s.Summarize(content, core.SummaryMinimal)
	require.NoError(t, err)
	require.NotEmpty(t, result) // SimpleSummarizer returns a truncated snippet
}

// TestLLMSummarizerSummarizeFallbackEmptyResponse verifies fallback when LLM returns empty text.
func TestLLMSummarizerSummarizeFallbackEmptyResponse(t *testing.T) {
	lm := &stubLM{response: "  "} // whitespace-only
	s := NewLLMSummarizer(lm)

	content := strings.Repeat("word ", 50)
	result, err := s.Summarize(content, core.SummaryConcise)
	require.NoError(t, err)
	require.NotEmpty(t, result) // SimpleSummarizer fallback
}

// TestLLMSummarizerSummarizeFile verifies the file path and level hint appear in prompt.
func TestLLMSummarizerSummarizeFile(t *testing.T) {
	lm := &stubLM{response: "reads and writes config files"}
	s := NewLLMSummarizer(lm)

	result, err := s.SummarizeFile("pkg/config/loader.go", "package config\n...", core.SummaryDetailed)
	require.NoError(t, err)
	require.Equal(t, "pkg/config/loader.go", result.Path)
	require.Equal(t, "reads and writes config files", result.Summary)
	require.Contains(t, lm.lastPrompt, "pkg/config/loader.go")
	require.Contains(t, lm.lastPrompt, fmt.Sprint(wordLimit(core.SummaryDetailed)))
}

// TestLLMSummarizerSummarizeDirectory verifies directory path and file summaries in prompt.
func TestLLMSummarizerSummarizeDirectory(t *testing.T) {
	lm := &stubLM{response: "config package handles application settings"}
	s := NewLLMSummarizer(lm)

	files := []core.FileSummary{
		{Path: "config/loader.go", Summary: "loads YAML config"},
		{Path: "config/types.go", Summary: "defines Config struct"},
	}
	result, err := s.SummarizeDirectory("config/", files, core.SummaryConcise)
	require.NoError(t, err)
	require.Equal(t, "config/", result.Path)
	require.Equal(t, "config package handles application settings", result.Summary)
	require.Contains(t, lm.lastPrompt, "config/")
	require.ElementsMatch(t, []string{"config/loader.go", "config/types.go"}, result.Files)
}

// TestLLMSummarizerSummarizeChunk verifies chunk name and level hint appear in prompt.
func TestLLMSummarizerSummarizeChunk(t *testing.T) {
	lm := &stubLM{response: "parses JWT tokens"}
	s := NewLLMSummarizer(lm)

	chunk := core.CodeChunk{ID: "chunk-1", Name: "parseJWT", File: "auth/jwt.go"}
	result, err := s.SummarizeChunk(chunk, "func parseJWT(token string) {...}", core.SummaryMinimal)
	require.NoError(t, err)
	require.Equal(t, "chunk-1", result.ChunkID)
	require.Equal(t, "parses JWT tokens", result.Summary)
	require.Contains(t, lm.lastPrompt, "parseJWT")
}

// TestNewContextPolicyUsesLLMSummarizerWhenModelPresent verifies NewContextPolicy
// selects LLMSummarizer over SimpleSummarizer when a LanguageModel is provided.
func TestNewContextPolicyUsesLLMSummarizerWhenModelPresent(t *testing.T) {
	lm := &stubLM{response: "summary"}
	policy := NewContextPolicy(ContextPolicyConfig{LanguageModel: lm}, nil)
	require.IsType(t, &LLMSummarizer{}, policy.Summarizer)
}

// TestNewContextPolicyFallsBackToSimpleWhenNoModel verifies SimpleSummarizer is
// used when no LanguageModel is in the config.
func TestNewContextPolicyFallsBackToSimpleWhenNoModel(t *testing.T) {
	policy := NewContextPolicy(ContextPolicyConfig{}, nil)
	require.IsType(t, &core.SimpleSummarizer{}, policy.Summarizer)
}

// TestNewContextPolicyRespectsExplicitSummarizer verifies that a caller-supplied
// Summarizer is not overwritten even if a LanguageModel is also present.
func TestNewContextPolicyRespectsExplicitSummarizer(t *testing.T) {
	explicit := &core.SimpleSummarizer{}
	lm := &stubLM{response: "summary"}
	policy := NewContextPolicy(ContextPolicyConfig{
		Summarizer:    explicit,
		LanguageModel: lm,
	}, nil)
	require.Same(t, explicit, policy.Summarizer.(*core.SimpleSummarizer))
}
