package retrieval

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

// RetrievalQuery is the caller-facing retrieval contract used by prefiltering and retrieval.
type RetrievalQuery struct {
	Text          string
	Scope         string
	SourceTypes   []string
	AllowChunkIDs []string
	MaxTokens     int
	UpdatedAfter  *time.Time
	PolicyTags    []string
	Limit         int
}

// PrefilterChunk captures active chunk metadata that passed the SQL prefilter.
type PrefilterChunk struct {
	ChunkID         string
	DocID           string
	VersionID       string
	CanonicalURI    string
	SourceType      string
	CorpusScope     string
	StructuralKey   string
	SourceUpdatedAt time.Time
}

// PrefilterResult captures the chunk allowlist and diagnostic counts.
type PrefilterResult struct {
	AllowChunkIDs []string
	Chunks        []PrefilterChunk
	ActiveChunks  int
	FilteredIn    int
	FilterSummary string
}

// MetadataPrefilter narrows retrieval candidates using indexed metadata before scoring.
type MetadataPrefilter struct {
	db        *sql.DB
	schemaErr error
}

// RankedCandidate is a fused retrieval result prior to context packing.
type RankedCandidate struct {
	ChunkID         string
	DocID           string
	VersionID       string
	StructuralKey   string
	Text            string
	FusedScore      float64
	SparseScore     float64
	DenseScore      float64
	SparseRank      int
	DenseRank       int
	MatchedBySparse bool
	MatchedByDense  bool
	Derivation      *core.DerivationChain
}

// RetrievalResult captures the prefilter output, raw index candidates, and fused ranking.
type RetrievalResult struct {
	Prefilter       *PrefilterResult
	Sparse          []SearchCandidate
	Dense           []SearchCandidate
	Fused           []RankedCandidate
	ExcludedReasons map[string]string
}

// Retriever composes metadata prefiltering, sparse search, dense search, and RRF fusion.
type Retriever struct {
	prefilter *MetadataPrefilter
	sparse    *SparseIndex
	dense     *DenseIndex
	sparseTop int
	denseTop  int
	rrfK      int
}

// NewMetadataPrefilter constructs a prefilter over the retrieval SQLite store.
func NewMetadataPrefilter(db *sql.DB) *MetadataPrefilter {
	return &MetadataPrefilter{
		db:        db,
		schemaErr: ensureRuntimeSchema(context.Background(), db),
	}
}

// NewRetriever constructs the phase-5 retriever over the retrieval SQLite store.
func NewRetriever(db *sql.DB, embedder Embedder) *Retriever {
	return &Retriever{
		prefilter: NewMetadataPrefilter(db),
		sparse:    NewSparseIndex(db),
		dense:     NewDenseIndex(db, embedder),
		sparseTop: 50,
		denseTop:  50,
		rrfK:      60,
	}
}

// Prefilter returns the active chunk allowlist that matches the supplied metadata constraints.
func (m *MetadataPrefilter) Prefilter(ctx context.Context, q RetrievalQuery) (*PrefilterResult, error) {
	if m == nil || m.db == nil {
		return nil, errors.New("metadata prefilter db required")
	}
	if m.schemaErr != nil {
		return nil, m.schemaErr
	}
	activeCount, err := currentActiveChunkCount(ctx, m.db)
	if err != nil {
		return nil, err
	}

	base := `SELECT c.chunk_id, c.doc_id
			FROM retrieval_chunks c
			JOIN retrieval_documents d
				ON d.doc_id = c.doc_id
		JOIN retrieval_document_versions dv
			ON dv.version_id = c.active_version_id AND dv.superseded = 0
		JOIN retrieval_chunk_versions cv
			ON cv.chunk_id = c.chunk_id AND cv.version_id = c.active_version_id
		WHERE c.tombstoned = 0
		  AND cv.tombstoned = 0
		  AND c.active_version_id <> ''`
	args := make([]any, 0, len(q.SourceTypes)+len(q.AllowChunkIDs)+len(q.PolicyTags)+2)

	scope := normalizeScope(q.Scope)
	if scope != "" {
		base += " AND d.corpus_scope = ?"
		args = append(args, scope)
	}
	if len(q.SourceTypes) > 0 {
		base += " AND " + sqlInClause("d.source_type", len(q.SourceTypes))
		for _, sourceType := range q.SourceTypes {
			args = append(args, strings.TrimSpace(sourceType))
		}
	}
	if len(q.AllowChunkIDs) > 0 {
		base += " AND " + sqlInClause("c.chunk_id", len(q.AllowChunkIDs))
		for _, chunkID := range q.AllowChunkIDs {
			args = append(args, strings.TrimSpace(chunkID))
		}
	}
	if q.UpdatedAfter != nil {
		base += " AND d.source_updated_at >= ?"
		args = append(args, q.UpdatedAfter.UTC().Format(time.RFC3339Nano))
	}
	for _, tag := range q.PolicyTags {
		base += ` AND EXISTS (
			SELECT 1
			FROM retrieval_document_policy_tags dpt
			WHERE dpt.doc_id = d.doc_id
			  AND dpt.tag = ?
		)`
		args = append(args, tag)
	}
	base += " ORDER BY d.source_updated_at DESC, c.doc_id ASC, c.chunk_id ASC"

	rows, err := m.db.QueryContext(ctx, base, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	resultCapacity := activeCount
	if len(q.AllowChunkIDs) > 0 && len(q.AllowChunkIDs) < resultCapacity {
		resultCapacity = len(q.AllowChunkIDs)
	}
	if resultCapacity < 0 {
		resultCapacity = 0
	}
	result := &PrefilterResult{
		AllowChunkIDs: make([]string, 0, resultCapacity),
		Chunks:        make([]PrefilterChunk, 0, resultCapacity),
		ActiveChunks:  activeCount,
	}
	for rows.Next() {
		var chunk PrefilterChunk
		if err := rows.Scan(&chunk.ChunkID, &chunk.DocID); err != nil {
			return nil, err
		}
		result.AllowChunkIDs = append(result.AllowChunkIDs, chunk.ChunkID)
		result.Chunks = append(result.Chunks, chunk)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result.FilteredIn = len(result.AllowChunkIDs)
	result.FilterSummary = formatFilterSummary(q, result.ActiveChunks, result.FilteredIn)
	return result, nil
}

func formatFilterSummary(q RetrievalQuery, active, filtered int) string {
	parts := []string{fmt.Sprintf("active=%d", active), fmt.Sprintf("filtered=%d", filtered)}
	parts = append(parts, "scope="+normalizeScope(q.Scope))
	if len(q.SourceTypes) > 0 {
		parts = append(parts, "source_types="+strings.Join(q.SourceTypes, ","))
	}
	if len(q.AllowChunkIDs) > 0 {
		parts = append(parts, fmt.Sprintf("allow_chunk_ids=%d", len(q.AllowChunkIDs)))
	}
	if q.UpdatedAfter != nil {
		parts = append(parts, "updated_after="+q.UpdatedAfter.UTC().Format(time.RFC3339))
	}
	if len(q.PolicyTags) > 0 {
		parts = append(parts, "policy_tags="+strings.Join(q.PolicyTags, ","))
	}
	return strings.Join(parts, " ")
}

// RetrieveCandidates runs prefiltering, sparse search, dense search, and RRF fusion.
func (r *Retriever) RetrieveCandidates(ctx context.Context, q RetrievalQuery) (*RetrievalResult, error) {
	if r == nil || r.prefilter == nil || r.sparse == nil {
		return nil, errors.New("retriever not configured")
	}
	prefilter, err := r.prefilter.Prefilter(ctx, q)
	if err != nil {
		return nil, err
	}
	result := &RetrievalResult{Prefilter: prefilter}
	if len(prefilter.AllowChunkIDs) == 0 {
		return result, nil
	}
	result.ExcludedReasons = make(map[string]string)

	sparseLimit := r.sparseTop
	sparseResults, err := r.sparse.Search(ctx, IndexQuery{
		Text:          q.Text,
		AllowChunkIDs: prefilter.AllowChunkIDs,
		Limit:         sparseLimit,
	})
	if err != nil {
		return nil, err
	}
	result.Sparse = sparseResults

	if r.dense != nil && r.dense.embedder != nil {
		denseLimit := r.denseTop
		denseResults, err := r.dense.Search(ctx, IndexQuery{
			Text:          q.Text,
			AllowChunkIDs: prefilter.AllowChunkIDs,
			Limit:         denseLimit,
		})
		if err != nil {
			return nil, err
		}
		result.Dense = denseResults
	}

	sparseIDs := candidateIDSet(result.Sparse)
	denseIDs := candidateIDSet(result.Dense)
	for _, chunkID := range prefilter.AllowChunkIDs {
		if _, ok := sparseIDs[chunkID]; ok {
			continue
		}
		if _, ok := denseIDs[chunkID]; ok {
			continue
		}
		appendExcludedReason(result.ExcludedReasons, chunkID, "retrieval:no_index_match")
	}

	fused := fuseCandidatesRRF(result.Sparse, result.Dense, r.rrfK)
	if q.Limit > 0 && len(fused) > q.Limit {
		for _, candidate := range fused[q.Limit:] {
			appendExcludedReason(result.ExcludedReasons, candidate.ChunkID, "fusion:rank_cutoff")
		}
		result.Fused = fused[:q.Limit]
	} else {
		result.Fused = fused
	}
	return result, nil
}

func candidateIDSet(candidates []SearchCandidate) map[string]struct{} {
	if len(candidates) == 0 {
		return nil
	}
	ids := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		ids[candidate.ChunkID] = struct{}{}
	}
	return ids
}

func fuseCandidatesRRF(sparse, dense []SearchCandidate, k int) []RankedCandidate {
	if k <= 0 {
		k = 60
	}
	fused := make(map[string]*RankedCandidate, len(sparse)+len(dense))
	apply := func(results []SearchCandidate, source CandidateSource) {
		for i, candidate := range results {
			rank := i + 1
			item, ok := fused[candidate.ChunkID]
			if !ok {
				item = &RankedCandidate{
					ChunkID:       candidate.ChunkID,
					DocID:         candidate.DocID,
					VersionID:     candidate.VersionID,
					StructuralKey: candidate.StructuralKey,
					Text:          candidate.Text,
				}
				fused[candidate.ChunkID] = item
			}
			item.FusedScore += 1.0 / float64(k+rank)
			switch source {
			case CandidateSourceSparse:
				item.MatchedBySparse = true
				item.SparseRank = rank
				item.SparseScore = candidate.Score
			case CandidateSourceDense:
				item.MatchedByDense = true
				item.DenseRank = rank
				item.DenseScore = candidate.Score
			}
		}
	}
	apply(sparse, CandidateSourceSparse)
	apply(dense, CandidateSourceDense)

	out := make([]RankedCandidate, 0, len(fused))
	for _, item := range fused {
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].FusedScore == out[j].FusedScore {
			if out[i].MatchedBySparse != out[j].MatchedBySparse {
				return out[i].MatchedBySparse
			}
			if out[i].MatchedByDense != out[j].MatchedByDense {
				return out[i].MatchedByDense
			}
			return out[i].ChunkID < out[j].ChunkID
		}
		return out[i].FusedScore > out[j].FusedScore
	})

	// Stamp RRF fusion derivation on candidates
	for i := range out {
		origin := core.OriginDerivation("retrieval")
		// Record sparse and dense scores in detail
		detail := fmt.Sprintf("sparse_score=%.4f sparse_rank=%d dense_score=%.4f dense_rank=%d fused_score=%.4f",
			out[i].SparseScore, out[i].SparseRank, out[i].DenseScore, out[i].DenseRank, out[i].FusedScore)
		origin = origin.Derive("rrf_fusion", "retrieval", 0.05, detail)
		out[i].Derivation = &origin
	}

	return out
}

func appendExcludedReason(reasons map[string]string, chunkID, reason string) {
	chunkID = strings.TrimSpace(chunkID)
	reason = strings.TrimSpace(reason)
	if chunkID == "" || reason == "" {
		return
	}
	if existing, ok := reasons[chunkID]; ok && existing != "" {
		for _, item := range strings.Split(existing, "|") {
			if item == reason {
				return
			}
		}
		reasons[chunkID] = existing + "|" + reason
		return
	}
	reasons[chunkID] = reason
}
