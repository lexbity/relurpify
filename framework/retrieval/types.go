// Package retrieval provides targeted knowledge query API for scatter-gather
// retrieval operations without triggering full context compilation.
package retrieval

import (
	"context"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextpolicy"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
)

// Ranker produces an ordered list of chunk IDs for a query.
// Rank position only; no scores (scores are not on the same scale across ranker types).
type Ranker interface {
	Name() string
	Rank(ctx context.Context, query RetrievalQuery, store *knowledge.ChunkStore) ([]knowledge.ChunkID, error)
}

// RankerRegistry holds admitted rankers for a compilation.
type RankerRegistry struct {
	rankers map[string]Ranker
}

// NewRankerRegistry creates a new ranker registry.
func NewRankerRegistry() *RankerRegistry {
	return &RankerRegistry{
		rankers: make(map[string]Ranker),
	}
}

// Register adds a ranker to the registry.
func (r *RankerRegistry) Register(ranker Ranker) {
	if r == nil || ranker == nil {
		return
	}
	r.rankers[ranker.Name()] = ranker
}

// Admitted returns rankers that are admitted by the context policy.
func (r *RankerRegistry) Admitted(policy *contextpolicy.ContextPolicyBundle) []Ranker {
	if r == nil {
		return nil
	}
	// For now, return all registered rankers
	// Policy filtering can be added later based on policy rules
	result := make([]Ranker, 0, len(r.rankers))
	for _, ranker := range r.rankers {
		result = append(result, ranker)
	}
	return result
}

// RetrievalQuery is the caller-facing contract.
type RetrievalQuery struct {
	Text        string
	Scope       string
	SourceTypes []knowledge.SourceOrigin
	Limit       int
	AfterSeq    uint64 // event log sequence; for cache coherence
}

// RetrievalResult contains the retrieved chunks and metadata.
type RetrievalResult struct {
	Query       RetrievalQuery
	Ranked      []RankedChunk
	TotalFound  int
	FilteredOut int
	Freshness   map[knowledge.ChunkID]knowledge.FreshnessState
}

// RankedChunk represents a chunk with its ranking information.
type RankedChunk struct {
	ChunkID knowledge.ChunkID
	Rank    int
	Score   float64
	Source  string // name of the ranker that contributed this
}

// RetrievalEvent captures retrieval operation metadata.
type RetrievalEvent struct {
	Query     RetrievalQuery
	Results   []RetrievalResult
	Timestamp time.Time
	Duration  time.Duration
}
