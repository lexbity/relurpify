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
func (r *RankerRegistry) Admitted(policy *contextpolicy.ContextPolicyBundle) []AdmittedRanker {
	if r == nil {
		return nil
	}
	if len(r.rankers) == 0 {
		return nil
	}

	allowed := make(map[string]float64)
	allowedOrder := make([]string, 0, len(r.rankers))
	addAllowed := func(id string, weight float64) {
		if id == "" {
			return
		}
		if weight <= 0 {
			weight = 1.0
		}
		if _, exists := allowed[id]; exists {
			return
		}
		allowed[id] = weight
		allowedOrder = append(allowedOrder, id)
	}

	if policy != nil {
		for _, ranker := range policy.Rankers {
			addAllowed(ranker.ID, float64(ranker.Priority))
		}
		for _, rankerID := range policy.SkillContributions.AdmittedRankers {
			addAllowed(rankerID, 1.0)
		}
	}

	if len(allowedOrder) == 0 {
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

	result := make([]AdmittedRanker, 0, len(allowedOrder))
	for _, name := range allowedOrder {
		ranker, ok := r.rankers[name]
		if !ok || ranker == nil {
			continue
		}
		priority := int(allowed[name])
		if priority <= 0 {
			priority = 1
		}
		result = append(result, AdmittedRanker{
			Ranker:   ranker,
			Weight:   allowed[name],
			Priority: priority,
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
