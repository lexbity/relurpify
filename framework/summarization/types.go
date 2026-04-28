// Package summarization implements all summarization algorithms.
// Called by the compiler write direction and by explicit agent capability invocations.
// Does not decide whether summarization is permitted — that is contextpolicy's job.
package summarization

import (
	"context"
	"crypto/sha256"
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/knowledge"
)

// SummarizerKind identifies the type of summarizer.
type SummarizerKind string

const (
	SummarizerKindAST     SummarizerKind = "ast"     // code, deterministic
	SummarizerKindHeading SummarizerKind = "heading" // structured documents
	SummarizerKindProse   SummarizerKind = "prose"   // free text, model-based
)

// SummarizationRequest is the input for summarization.
type SummarizationRequest struct {
	Chunks            []knowledge.KnowledgeChunk
	Kind              SummarizerKind
	SourceOrigin      knowledge.SourceOrigin
	TargetTokenBudget int
	ModelID           string // for prose only; empty = use workspace default
}

// SummarizationResult is the output of summarization.
type SummarizationResult struct {
	Summary          string
	TokenEstimate    int
	DerivationMethod string // summarizer ID + fingerprint
	SourceCoverage   []knowledge.ChunkID
	CoverageHash     string
	UsedModel        bool // true if a language model was invoked
}

// Summarizer is the interface for all summarizers.
type Summarizer interface {
	Kind() SummarizerKind
	Summarize(ctx context.Context, req SummarizationRequest) (*SummarizationResult, error)
	CanSummarize(chunks []knowledge.KnowledgeChunk) bool
}

// GenerationCapError is returned when generation cap is exceeded.
type GenerationCapError struct {
	Generation int
	Cap        int
}

func (e *GenerationCapError) Error() string {
	return fmt.Sprintf("generation cap exceeded: generation %d >= cap %d", e.Generation, e.Cap)
}

// ComputeCoverageHash computes a hash of source chunk IDs and versions.
func ComputeCoverageHash(chunks []knowledge.KnowledgeChunk) string {
	h := sha256.New()
	for _, chunk := range chunks {
		h.Write([]byte(string(chunk.ID)))
		h.Write([]byte(fmt.Sprintf("%d", chunk.Version)))
	}
	return fmt.Sprintf("%x", h.Sum(nil)[:16])
}

// CountTokens estimates token count from text (rough approximation: 1 token ≈ 4 bytes).
func CountTokens(text string) int {
	return len(text) / 4
}
