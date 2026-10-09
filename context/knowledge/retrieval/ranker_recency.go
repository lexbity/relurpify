package retrieval

import (
	"context"
	"math"
	"time"

	"codeburg.org/lexbit/relurpify/context/knowledge"
)

// RecencyRanker prefers recently updated chunks with exponential decay.
type RecencyRanker struct {
	HalfLifeHours float64
	Now           func() time.Time
}

func (r *RecencyRanker) Name() string { return "recency" }

func (r *RecencyRanker) Rank(ctx context.Context, query RetrievalQuery, snap *CorpusSnapshot) ([]knowledge.ChunkID, error) {
	_ = ctx
	_ = query
	if r == nil || snap == nil {
		return nil, nil
	}
	chunks := snap.Chunks
	if len(chunks) == 0 {
		return nil, nil
	}
	halfLife := r.HalfLifeHours
	if halfLife <= 0 {
		halfLife = 24.0
	}
	now := time.Now().UTC()
	if r.Now != nil {
		now = r.Now().UTC()
	}

	scores := make(map[knowledge.ChunkID]float64, len(chunks))
	for _, chunk := range chunks {
		updated := chunk.UpdatedAt
		if updated.IsZero() {
			updated = chunk.CreatedAt
		}
		if updated.IsZero() {
			continue
		}
		ageHours := now.Sub(updated).Hours()
		if ageHours < 0 {
			ageHours = 0
		}
		score := math.Pow(0.5, ageHours/halfLife)
		scores[chunk.ID] = score
	}

	// Tiebreak on recency via the snapshot's O(1) ID index — the comparator
	// no longer rescans the corpus per comparison.
	ids := sortRankedIDs(scores, func(a, b knowledge.ChunkID) bool {
		left, lOk := snap.Lookup(a)
		right, rOk := snap.Lookup(b)
		if lOk && rOk {
			return left.UpdatedAt.After(right.UpdatedAt)
		}
		return a < b
	})
	return ids, nil
}
