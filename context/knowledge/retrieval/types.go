// Package retrieval provides targeted knowledge query API for scatter-gather
// retrieval operations without triggering full context compilation.
package retrieval

import (
	"context"
	"time"

	"codeburg.org/lexbit/relurpify/context/knowledge"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
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
	order   []string
}

// AdmittedRanker couples a ranker with its policy-derived fusion weight.
type AdmittedRanker struct {
	Ranker   Ranker
	Weight   float64
	Priority int
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
	if r.rankers == nil {
		r.rankers = make(map[string]Ranker)
	}
	name := ranker.Name()
	if _, exists := r.rankers[name]; !exists {
		r.order = append(r.order, name)
	}
	r.rankers[name] = ranker
}

// Admitted returns rankers that are admitted by the context policy.
func (r *RankerRegistry) Admitted(policy *contextports.PolicyBundle) []AdmittedRanker {
	if r == nil {
		return nil
	}
	if len(r.rankers) == 0 {
		return nil
	}

	_ = policy // PolicyBundle does not carry ranker configuration; all registered rankers are admitted.

	result := make([]AdmittedRanker, 0, len(r.order))
	for _, name := range r.order {
		ranker, ok := r.rankers[name]
		if !ok || ranker == nil {
			continue
		}
		result = append(result, AdmittedRanker{
			Ranker:   ranker,
			Weight:   1.0,
			Priority: 1,
		})
	}
	return result
}

// RetrievalQuery is the caller-facing contract.
type RetrievalQuery struct {
	Text        string
	Scope       string
	SourceTypes []knowledge.SourceOrigin
	Anchors     []AnchorRef
	Traversal   *TraversalSpec
	Limit       int
	AfterSeq    uint64 // event log sequence; for cache coherence
}

// TraversalDirection constrains how traversal moves through the graph.
type TraversalDirection string

const (
	TraversalDirectionOut  TraversalDirection = "out"
	TraversalDirectionIn   TraversalDirection = "in"
	TraversalDirectionBoth TraversalDirection = "both"
)

// TraversalSpec requests graph-aware candidate generation.
type TraversalSpec struct {
	AnchorIDs     []string
	EdgeKinds     []string
	Direction     TraversalDirection
	MaxDepth      int
	PreferLatest  bool
	MaxCandidates int // 0 ⇒ fall back to policy / default
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
