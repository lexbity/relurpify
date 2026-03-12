package retrieval

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
)

// CandidateSource identifies which index produced a candidate.
type CandidateSource string

const (
	CandidateSourceSparse CandidateSource = "sparse"
	CandidateSourceDense  CandidateSource = "dense"
)

// SearchCandidate is the shared result shape emitted by sparse and dense indexes.
type SearchCandidate struct {
	ChunkID       string
	DocID         string
	VersionID     string
	StructuralKey string
	Text          string
	Score         float64
	Source        CandidateSource
}

// IndexQuery describes a constrained candidate search request.
type IndexQuery struct {
	Text          string
	AllowChunkIDs []string
	Limit         int
}

// SparseIndex searches lexical matches over active chunk text.
type SparseIndex struct {
	db *sql.DB
}

// DenseIndex searches vector similarity over active embeddings.
type DenseIndex struct {
	db       *sql.DB
	embedder Embedder
}

// NewSparseIndex constructs a sparse index over the retrieval SQLite store.
func NewSparseIndex(db *sql.DB) *SparseIndex {
	return &SparseIndex{db: db}
}

// NewDenseIndex constructs an exact dense index over persisted embeddings.
func NewDenseIndex(db *sql.DB, embedder Embedder) *DenseIndex {
	return &DenseIndex{db: db, embedder: embedder}
}

// Search queries the sparse index using FTS5 when available and a LIKE fallback otherwise.
func (s *SparseIndex) Search(ctx context.Context, q IndexQuery) ([]SearchCandidate, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("sparse index db required")
	}
	if err := EnsureSchema(ctx, s.db); err != nil {
		return nil, err
	}
	queryText := strings.TrimSpace(q.Text)
	if queryText == "" {
		return nil, nil
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 10
	}
	fts, err := isFTSBacked(ctx, s.db)
	if err != nil {
		return nil, err
	}
	if fts {
		return s.searchFTS(ctx, queryText, q.AllowChunkIDs, limit)
	}
	return s.searchFallback(ctx, queryText, q.AllowChunkIDs, limit)
}

// Search queries the dense index using exact cosine similarity over active embeddings.
func (d *DenseIndex) Search(ctx context.Context, q IndexQuery) ([]SearchCandidate, error) {
	if d == nil || d.db == nil {
		return nil, errors.New("dense index db required")
	}
	if d.embedder == nil {
		return nil, errors.New("dense index embedder required")
	}
	if err := EnsureSchema(ctx, d.db); err != nil {
		return nil, err
	}
	queryText := strings.TrimSpace(q.Text)
	if queryText == "" {
		return nil, nil
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 10
	}
	vectors, err := d.embedder.Embed(ctx, []string{queryText})
	if err != nil {
		return nil, err
	}
	if len(vectors) != 1 {
		return nil, fmt.Errorf("embedder returned %d vectors for dense query", len(vectors))
	}
	queryVector := vectors[0]

	base := `SELECT cv.chunk_id, cv.doc_id, cv.version_id, cv.structural_key, cv.text, e.vector
		FROM retrieval_embeddings e
		JOIN retrieval_chunk_versions cv
			ON cv.chunk_id = e.chunk_id AND cv.version_id = e.version_id
		JOIN retrieval_chunks c
			ON c.chunk_id = cv.chunk_id AND c.active_version_id = cv.version_id
		JOIN retrieval_document_versions dv
			ON dv.version_id = cv.version_id AND dv.superseded = 0
		WHERE c.tombstoned = 0
		  AND cv.tombstoned = 0
		  AND e.model_id = ?`
	args := []any{d.embedder.ModelID()}
	if len(q.AllowChunkIDs) > 0 {
		base += " AND " + sqlInClause("cv.chunk_id", len(q.AllowChunkIDs))
		for _, id := range q.AllowChunkIDs {
			args = append(args, id)
		}
	}
	rows, err := d.db.QueryContext(ctx, base, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]SearchCandidate, 0)
	for rows.Next() {
		var candidate SearchCandidate
		var vectorBlob []byte
		if err := rows.Scan(&candidate.ChunkID, &candidate.DocID, &candidate.VersionID, &candidate.StructuralKey, &candidate.Text, &vectorBlob); err != nil {
			return nil, err
		}
		candidate.Score = cosineSimilarityFloat32(queryVector, decodeVector(vectorBlob))
		if candidate.Score <= 0 {
			continue
		}
		candidate.Source = CandidateSourceDense
		results = append(results, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].ChunkID < results[j].ChunkID
		}
		return results[i].Score > results[j].Score
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (s *SparseIndex) searchFTS(ctx context.Context, queryText string, allowChunkIDs []string, limit int) ([]SearchCandidate, error) {
	base := `SELECT cv.chunk_id, cv.doc_id, cv.version_id, cv.structural_key, cv.text, bm25(retrieval_chunks_fts) AS score
		FROM retrieval_chunks_fts
		JOIN retrieval_chunk_versions cv
			ON cv.chunk_id = retrieval_chunks_fts.chunk_id AND cv.version_id = retrieval_chunks_fts.version_id
		JOIN retrieval_chunks c
			ON c.chunk_id = cv.chunk_id AND c.active_version_id = cv.version_id
		JOIN retrieval_document_versions dv
			ON dv.version_id = cv.version_id AND dv.superseded = 0
		WHERE retrieval_chunks_fts MATCH ?
		  AND c.tombstoned = 0
		  AND cv.tombstoned = 0`
	args := []any{fts5OrQuery(queryText)}
	if len(allowChunkIDs) > 0 {
		base += " AND " + sqlInClause("cv.chunk_id", len(allowChunkIDs))
		for _, id := range allowChunkIDs {
			args = append(args, id)
		}
	}
	base += " ORDER BY score ASC, cv.chunk_id ASC LIMIT ?"
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, base, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSparseRows(rows, true)
}

func (s *SparseIndex) searchFallback(ctx context.Context, queryText string, allowChunkIDs []string, limit int) ([]SearchCandidate, error) {
	terms := strings.Fields(strings.ToLower(queryText))
	if len(terms) == 0 {
		return nil, nil
	}
	base := `SELECT cv.chunk_id, cv.doc_id, cv.version_id, cv.structural_key, cv.text
		FROM retrieval_chunks_fts rf
		JOIN retrieval_chunk_versions cv
			ON cv.chunk_id = rf.chunk_id AND cv.version_id = rf.version_id
		JOIN retrieval_chunks c
			ON c.chunk_id = cv.chunk_id AND c.active_version_id = cv.version_id
		JOIN retrieval_document_versions dv
			ON dv.version_id = cv.version_id AND dv.superseded = 0
		WHERE c.tombstoned = 0
		  AND cv.tombstoned = 0`
	args := make([]any, 0, len(terms)+len(allowChunkIDs))
	orParts := make([]string, 0, len(terms))
	for _, term := range terms {
		orParts = append(orParts, "lower(rf.text) LIKE ?")
		args = append(args, "%"+term+"%")
	}
	if len(orParts) > 0 {
		base += " AND (" + strings.Join(orParts, " OR ") + ")"
	}
	if len(allowChunkIDs) > 0 {
		base += " AND " + sqlInClause("cv.chunk_id", len(allowChunkIDs))
		for _, id := range allowChunkIDs {
			args = append(args, id)
		}
	}
	rows, err := s.db.QueryContext(ctx, base, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make([]SearchCandidate, 0)
	for rows.Next() {
		var candidate SearchCandidate
		if err := rows.Scan(&candidate.ChunkID, &candidate.DocID, &candidate.VersionID, &candidate.StructuralKey, &candidate.Text); err != nil {
			return nil, err
		}
		candidate.Score = fallbackSparseScore(candidate.Text, terms)
		if candidate.Score <= 0 {
			continue
		}
		candidate.Source = CandidateSourceSparse
		results = append(results, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].ChunkID < results[j].ChunkID
		}
		return results[i].Score > results[j].Score
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func scanSparseRows(rows *sql.Rows, invertBM25 bool) ([]SearchCandidate, error) {
	results := make([]SearchCandidate, 0)
	for rows.Next() {
		var candidate SearchCandidate
		var rawScore float64
		if err := rows.Scan(&candidate.ChunkID, &candidate.DocID, &candidate.VersionID, &candidate.StructuralKey, &candidate.Text, &rawScore); err != nil {
			return nil, err
		}
		if invertBM25 {
			candidate.Score = 1 / (1 + math.Max(rawScore, 0))
		} else {
			candidate.Score = rawScore
		}
		candidate.Source = CandidateSourceSparse
		results = append(results, candidate)
	}
	return results, rows.Err()
}

func isFTSBacked(ctx context.Context, db *sql.DB) (bool, error) {
	row := db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE name = 'retrieval_chunks_fts'`)
	var sqlText string
	if err := row.Scan(&sqlText); err != nil {
		return false, err
	}
	return strings.Contains(strings.ToLower(sqlText), "using fts5"), nil
}

// fts5OrQuery converts a natural language query string into an FTS5 OR expression
// so that documents matching ANY of the terms are returned, ranked by BM25.
func fts5OrQuery(text string) string {
	terms := strings.Fields(text)
	if len(terms) <= 1 {
		return text
	}
	return strings.Join(terms, " OR ")
}

func sqlInClause(column string, n int) string {
	placeholders := make([]string, n)
	for i := range placeholders {
		placeholders[i] = "?"
	}
	return fmt.Sprintf("%s IN (%s)", column, strings.Join(placeholders, ", "))
}

func fallbackSparseScore(text string, terms []string) float64 {
	if len(terms) == 0 {
		return 0
	}
	lower := strings.ToLower(text)
	score := 0.0
	for _, term := range terms {
		score += float64(strings.Count(lower, term))
	}
	if score == 0 {
		return 0
	}
	return score / float64(len(terms))
}

func cosineSimilarityFloat32(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i] * b[i])
		normA += float64(a[i] * a[i])
		normB += float64(b[i] * b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
