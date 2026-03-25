package retrieval

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	defaultHotWindow   = 6 * time.Hour
	defaultHotChunkCap = 64
)

// ServiceOptions controls runtime retrieval tier behavior.
type ServiceOptions struct {
	Cache     CacheConfig
	HotWindow time.Duration
	HotLimit  int
}

// CompactionOptions controls stale data removal and retention.
type CompactionOptions struct {
	EventRetention time.Duration
}

// CompactionResult reports what physical rows were removed or rebuilt.
type CompactionResult struct {
	DeletedEmbeddings       int
	DeletedChunkVersions    int
	DeletedChunks           int
	DeletedDocumentVersions int
	DeletedEvents           int
	ReindexedRows           int
	DriftEventsDetected     int
}

// EmbeddingRebuildOptions scopes embedding regeneration.
type EmbeddingRebuildOptions struct {
	DocID string
	Force bool
}

// Maintenance provides compaction and rebuild operations over retrieval storage.
type Maintenance struct {
	db        *sql.DB
	embedder  Embedder
	now       func() time.Time
	schemaErr error
}

// NewMaintenance constructs a maintenance operator over retrieval storage.
func NewMaintenance(db *sql.DB, embedder Embedder) *Maintenance {
	return &Maintenance{
		db:        db,
		embedder:  embedder,
		now:       func() time.Time { return time.Now().UTC() },
		schemaErr: ensureRuntimeSchema(context.Background(), db),
	}
}

// Compact removes stale physical rows while preserving active retrieval results.
func (m *Maintenance) Compact(ctx context.Context, opts CompactionOptions) (CompactionResult, error) {
	if m == nil || m.db == nil {
		return CompactionResult{}, errors.New("maintenance db required")
	}
	if m.schemaErr != nil {
		return CompactionResult{}, m.schemaErr
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return CompactionResult{}, err
	}
	defer tx.Rollback()

	result := CompactionResult{}

	// Detect anchor drift before deleting chunk versions
	if driftEventCount, err := m.detectAnchorDrift(ctx, tx); err != nil {
		return CompactionResult{}, err
	} else {
		result.DriftEventsDetected = driftEventCount
	}

	if result.DeletedEmbeddings, err = execDeleteCount(ctx, tx, `DELETE FROM retrieval_embeddings
		WHERE EXISTS (
			SELECT 1
			FROM retrieval_chunks c
			WHERE c.chunk_id = retrieval_embeddings.chunk_id
			  AND (c.tombstoned = 1 OR c.active_version_id <> retrieval_embeddings.version_id)
		)`); err != nil {
		return CompactionResult{}, err
	}
	if result.DeletedChunkVersions, err = execDeleteCount(ctx, tx, `DELETE FROM retrieval_chunk_versions
		WHERE tombstoned = 1
		   OR EXISTS (
			SELECT 1
			FROM retrieval_chunks c
			WHERE c.chunk_id = retrieval_chunk_versions.chunk_id
			  AND (c.tombstoned = 1 OR c.active_version_id <> retrieval_chunk_versions.version_id)
		   )`); err != nil {
		return CompactionResult{}, err
	}
	if result.DeletedChunks, err = execDeleteCount(ctx, tx, `DELETE FROM retrieval_chunks
		WHERE tombstoned = 1
		   OR active_version_id = ''
		   OR NOT EXISTS (
			SELECT 1
			FROM retrieval_chunk_versions cv
			WHERE cv.chunk_id = retrieval_chunks.chunk_id
		   )`); err != nil {
		return CompactionResult{}, err
	}
	if result.DeletedDocumentVersions, err = execDeleteCount(ctx, tx, `DELETE FROM retrieval_document_versions
		WHERE superseded = 1
		  AND NOT EXISTS (
			SELECT 1
			FROM retrieval_chunks c
			WHERE c.active_version_id = retrieval_document_versions.version_id
		  )`); err != nil {
		return CompactionResult{}, err
	}
	if opts.EventRetention > 0 {
		cutoff := m.now().Add(-opts.EventRetention).Format(time.RFC3339Nano)
		deletedRetrievalEvents, err := execDeleteCount(ctx, tx, `DELETE FROM retrieval_events WHERE created_at < ?`, cutoff)
		if err != nil {
			return CompactionResult{}, err
		}
		deletedPackingEvents, err := execDeleteCount(ctx, tx, `DELETE FROM retrieval_packing_events WHERE created_at < ?`, cutoff)
		if err != nil {
			return CompactionResult{}, err
		}
		result.DeletedEvents = deletedRetrievalEvents + deletedPackingEvents
	}
	if err := refreshCorpusMetadataTx(ctx, tx); err != nil {
		return CompactionResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return CompactionResult{}, err
	}
	result.ReindexedRows, err = m.RebuildSearchIndex(ctx)
	if err != nil {
		return CompactionResult{}, err
	}
	return result, nil
}

// RebuildSearchIndex regenerates sparse search rows from active canonical records.
func (m *Maintenance) RebuildSearchIndex(ctx context.Context) (int, error) {
	if m == nil || m.db == nil {
		return 0, errors.New("maintenance db required")
	}
	if m.schemaErr != nil {
		return 0, m.schemaErr
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM retrieval_chunks_fts`); err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO retrieval_chunks_fts (chunk_id, doc_id, version_id, text)
		SELECT cv.chunk_id, cv.doc_id, cv.version_id, cv.text
		FROM retrieval_chunk_versions cv
		JOIN retrieval_chunks c
			ON c.chunk_id = cv.chunk_id AND c.active_version_id = cv.version_id
		JOIN retrieval_document_versions dv
			ON dv.version_id = cv.version_id AND dv.superseded = 0
		WHERE c.tombstoned = 0
		  AND cv.tombstoned = 0`)
	if err != nil {
		return 0, err
	}
	if err := refreshCorpusMetadataTx(ctx, tx); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// RebuildEmbeddings regenerates dense vectors from persisted active chunk records.
func (m *Maintenance) RebuildEmbeddings(ctx context.Context, opts EmbeddingRebuildOptions) (int, error) {
	if m == nil || m.db == nil {
		return 0, errors.New("maintenance db required")
	}
	if m.embedder == nil {
		return 0, errors.New("maintenance embedder required")
	}
	if m.schemaErr != nil {
		return 0, m.schemaErr
	}
	if opts.Force {
		args := []any{m.embedder.ModelID()}
		query := `DELETE FROM retrieval_embeddings WHERE model_id = ?`
		if opts.DocID != "" {
			query += ` AND EXISTS (
				SELECT 1
				FROM retrieval_chunk_versions cv
				WHERE cv.chunk_id = retrieval_embeddings.chunk_id
				  AND cv.version_id = retrieval_embeddings.version_id
				  AND cv.doc_id = ?
			)`
			args = append(args, opts.DocID)
		}
		if _, err := m.db.ExecContext(ctx, query, args...); err != nil {
			return 0, err
		}
	}
	pipeline := NewIngestionPipeline(m.db, m.embedder)
	embeddings, err := pipeline.BackfillEmbeddings(ctx, BackfillRequest{DocID: opts.DocID})
	if err != nil {
		return 0, err
	}
	return len(embeddings), nil
}

func recentHotChunkIDs(ctx context.Context, db *sql.DB, window time.Duration, limit int, now time.Time) ([]string, error) {
	if db == nil || window <= 0 || limit <= 0 {
		return nil, nil
	}
	cutoff := now.Add(-window).Format(time.RFC3339Nano)
	rows, err := db.QueryContext(ctx, `SELECT injected_chunks_json
		FROM retrieval_packing_events
		WHERE created_at >= ?
		ORDER BY created_at DESC`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seen := make(map[string]struct{}, limit)
	ids := make([]string, 0, limit)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var chunks []string
		if err := json.Unmarshal([]byte(raw), &chunks); err != nil {
			return nil, err
		}
		for _, chunkID := range chunks {
			if chunkID == "" {
				continue
			}
			if _, ok := seen[chunkID]; ok {
				continue
			}
			seen[chunkID] = struct{}{}
			ids = append(ids, chunkID)
			if len(ids) >= limit {
				return ids, rows.Err()
			}
		}
	}
	return ids, rows.Err()
}

func execDeleteCount(ctx context.Context, tx *sql.Tx, query string, args ...any) (int, error) {
	res, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// detectAnchorDrift identifies drift in anchored terms by comparing old and new chunk versions.
// For each chunk version about to be deleted, check if anchors reference it and compare
// the anchored term's context between versions using Jaccard similarity.
func (m *Maintenance) detectAnchorDrift(ctx context.Context, tx *sql.Tx) (int, error) {
	if m == nil || m.db == nil {
		return 0, nil
	}

	const driftThreshold = 0.6

	// Query for chunk versions that are about to be deleted (inactive versions)
	rows, err := tx.QueryContext(ctx, `
		SELECT cv.chunk_id, cv.version_id, cv.text
		FROM retrieval_chunk_versions cv
		WHERE cv.tombstoned = 0
		  AND EXISTS (
			SELECT 1
			FROM retrieval_chunks c
			WHERE c.chunk_id = cv.chunk_id
			  AND c.active_version_id != cv.version_id
		  )
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	eventCount := 0
	for rows.Next() {
		var oldChunkID, oldVersionID, oldText string
		if err := rows.Scan(&oldChunkID, &oldVersionID, &oldText); err != nil {
			return 0, err
		}

		// Query anchors that reference this chunk
		anchorRows, err := tx.QueryContext(ctx, `
			SELECT anchor_id, term, term_normalized
			FROM retrieval_semantic_anchors
			WHERE source_chunk_id = ?
			  AND superseded_by IS NULL
			  AND invalidated_at IS NULL
		`, oldChunkID)
		if err != nil {
			return 0, err
		}
		defer anchorRows.Close()

		for anchorRows.Next() {
			var anchorID, term, termNorm string
			if err := anchorRows.Scan(&anchorID, &term, &termNorm); err != nil {
				return 0, err
			}

			// Get the active version for this chunk
			var newText string
			err := tx.QueryRowContext(ctx, `
				SELECT cv.text
				FROM retrieval_chunk_versions cv
				JOIN retrieval_chunks c ON c.chunk_id = cv.chunk_id
				WHERE c.chunk_id = ?
				  AND cv.version_id = c.active_version_id
			`, oldChunkID).Scan(&newText)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					// Chunk was deleted, emit drift event with similarity 0.0
					similarity := 0.0
					eventID := generateAnchorID() + "-evt"
					_, err := tx.ExecContext(ctx, `
						INSERT INTO retrieval_anchor_events
						  (event_id, anchor_id, event_type, detail, similarity_score, created_at)
						VALUES (?, ?, ?, ?, ?, ?)
					`, eventID, anchorID, "drift_detected", "term removed from source", similarity, m.now().UTC())
					if err != nil {
						return 0, err
					}
					eventCount++
					continue
				}
				return 0, err
			}

			// Extract sentences and compare
			oldSentence, _, _, oldFound := ExtractSentence(oldText, term)
			newSentence, _, _, newFound := ExtractSentence(newText, term)

			if !oldFound || !newFound {
				// Term no longer present in active version
				eventID := generateAnchorID() + "-evt"
				similarity := 0.0
				if !newFound {
					_, err := tx.ExecContext(ctx, `
						INSERT INTO retrieval_anchor_events
						  (event_id, anchor_id, event_type, detail, old_definition, similarity_score, created_at)
						VALUES (?, ?, ?, ?, ?, ?, ?)
					`, eventID, anchorID, "drift_detected", "term removed from source", oldSentence, similarity, m.now().UTC())
					if err != nil {
						return 0, err
					}
					eventCount++
				}
				continue
			}

			// Compute Jaccard similarity
			similarity := JaccardSimilarity(oldSentence, newSentence)
			if similarity < driftThreshold {
				eventID := generateAnchorID() + "-evt"
				_, err := tx.ExecContext(ctx, `
					INSERT INTO retrieval_anchor_events
					  (event_id, anchor_id, event_type, detail, old_definition, new_definition, similarity_score, created_at)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?)
				`, eventID, anchorID, "drift_detected", fmt.Sprintf("similarity %.2f < threshold %.2f", similarity, driftThreshold),
					oldSentence, newSentence, similarity, m.now().UTC())
				if err != nil {
					return 0, err
				}
				eventCount++
			}
		}
		if err := anchorRows.Err(); err != nil {
			return 0, err
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	return eventCount, nil
}
