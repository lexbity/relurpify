package retrieval

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// AnchorDeclaration is the caller-facing input for declaring semantic anchors during ingestion.
type AnchorDeclaration struct {
	Term       string            // the load-bearing word or phrase
	Definition string            // what it means in this document, right now
	Class      string            // policy | identity | commitment | technical
	Context    map[string]string // surrounding interpretive frame (optional)
}

// AnchorRecord is the persisted anchor state.
type AnchorRecord struct {
	AnchorID        string
	Term            string
	TermNormalized  string
	Definition      string
	ContextSummary  string
	Scope           string
	AnchorClass     string
	SourceChunkID   string
	SourceVersionID string
	SourceDocID     string
	CorpusScope     string
	PolicySnapshotID *string
	SupersededBy    *string
	CreatedAt       time.Time
	InvalidatedAt   *time.Time
}

// AnchorRef is the lightweight reference carried in packed blocks and mixed evidence.
type AnchorRef struct {
	AnchorID   string `json:"anchor_id"`
	Term       string `json:"term"`
	Definition string `json:"definition"`
	Class      string `json:"class"`
	Active     bool   `json:"active"`
	CreatedAt  string `json:"created_at"`
}

// AnchorEventRecord is the persisted anchor lifecycle event.
type AnchorEventRecord struct {
	EventID         string
	AnchorID        string
	EventType       string
	Detail          string
	OldDefinition   string
	NewDefinition   string
	SimilarityScore float64
	CreatedAt       time.Time
}

// normalizeTerm normalizes a term for comparison (lowercase, trim, collapse whitespace).
func normalizeTerm(term string) string {
	term = strings.ToLower(strings.TrimSpace(term))
	term = strings.Join(strings.Fields(term), " ")
	return term
}

// summarizeContext creates a brief summary of the anchor context.
func summarizeContext(ctx map[string]string) string {
	if len(ctx) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ctx))
	for k, v := range ctx {
		parts = append(parts, fmt.Sprintf("%s:%s", k, v))
	}
	return strings.Join(parts, "|")
}

// generateAnchorID generates a unique anchor ID (simplified version using hash).
func generateAnchorID() string {
	// In production, use a proper UUID or content hash.
	// For now, use a simple approach.
	return fmt.Sprintf("anchor-%d", time.Now().UnixNano())
}

// ActiveAnchors returns all non-invalidated anchors for a corpus scope.
func ActiveAnchors(ctx context.Context, db *sql.DB, corpusScope string) ([]AnchorRecord, error) {
	if db == nil {
		return nil, errors.New("db required")
	}

	rows, err := db.QueryContext(ctx, `
		SELECT anchor_id, term, term_normalized, definition, context_summary, scope,
		       anchor_class, source_chunk_id, source_version_id, source_doc_id,
		       corpus_scope, policy_snapshot_id, superseded_by, created_at, invalidated_at
		FROM retrieval_semantic_anchors
		WHERE corpus_scope = ? AND invalidated_at IS NULL
		ORDER BY created_at DESC
	`, corpusScope)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []AnchorRecord
	for rows.Next() {
		var r AnchorRecord
		var createdAt string
		var invalidatedAt *string
		if err := rows.Scan(&r.AnchorID, &r.Term, &r.TermNormalized, &r.Definition,
			&r.ContextSummary, &r.Scope, &r.AnchorClass, &r.SourceChunkID,
			&r.SourceVersionID, &r.SourceDocID, &r.CorpusScope, &r.PolicySnapshotID,
			&r.SupersededBy, &createdAt, &invalidatedAt); err != nil {
			return nil, err
		}
		createdTime, err := time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, err
		}
		r.CreatedAt = createdTime
		if invalidatedAt != nil {
			invalidatedTime, err := time.Parse(time.RFC3339, *invalidatedAt)
			if err != nil {
				return nil, err
			}
			r.InvalidatedAt = &invalidatedTime
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// AnchorsForChunk returns active anchors whose source_chunk_id matches.
func AnchorsForChunk(ctx context.Context, db *sql.DB, chunkID string) ([]AnchorRef, error) {
	if db == nil {
		return nil, errors.New("db required")
	}

	rows, err := db.QueryContext(ctx, `
		SELECT anchor_id, term, definition, anchor_class, created_at
		FROM retrieval_semantic_anchors
		WHERE source_chunk_id = ? AND invalidated_at IS NULL
		ORDER BY created_at DESC
	`, chunkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var refs []AnchorRef
	for rows.Next() {
		var r AnchorRef
		var createdAt string
		if err := rows.Scan(&r.AnchorID, &r.Term, &r.Definition, &r.Class, &createdAt); err != nil {
			return nil, err
		}
		r.Active = true
		r.CreatedAt = createdAt
		refs = append(refs, r)
	}
	return refs, rows.Err()
}

// AnchorsForTerms returns active anchors matching any of the given normalized terms within a corpus scope.
func AnchorsForTerms(ctx context.Context, db *sql.DB, terms []string, corpusScope string) ([]AnchorRef, error) {
	if db == nil {
		return nil, errors.New("db required")
	}
	if len(terms) == 0 {
		return []AnchorRef{}, nil
	}

	// Normalize all input terms
	normalizedTerms := make([]interface{}, len(terms))
	for i, term := range terms {
		normalizedTerms[i] = normalizeTerm(term)
	}

	placeholders := strings.Repeat("?,", len(normalizedTerms)-1) + "?"
	query := fmt.Sprintf(`
		SELECT anchor_id, term, definition, anchor_class, created_at
		FROM retrieval_semantic_anchors
		WHERE term_normalized IN (%s) AND corpus_scope = ? AND invalidated_at IS NULL
		ORDER BY created_at DESC
	`, placeholders)

	args := append(normalizedTerms, corpusScope)
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var refs []AnchorRef
	for rows.Next() {
		var r AnchorRef
		var createdAt string
		if err := rows.Scan(&r.AnchorID, &r.Term, &r.Definition, &r.Class, &createdAt); err != nil {
			return nil, err
		}
		r.Active = true
		r.CreatedAt = createdAt
		refs = append(refs, r)
	}
	return refs, rows.Err()
}

// AnchorHistory returns the full supersession chain for an anchor term.
func AnchorHistory(ctx context.Context, db *sql.DB, termNormalized, corpusScope string) ([]AnchorRecord, error) {
	if db == nil {
		return nil, errors.New("db required")
	}

	rows, err := db.QueryContext(ctx, `
		SELECT anchor_id, term, term_normalized, definition, context_summary, scope,
		       anchor_class, source_chunk_id, source_version_id, source_doc_id,
		       corpus_scope, policy_snapshot_id, superseded_by, created_at, invalidated_at
		FROM retrieval_semantic_anchors
		WHERE term_normalized = ? AND corpus_scope = ?
		ORDER BY created_at DESC
	`, termNormalized, corpusScope)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []AnchorRecord
	for rows.Next() {
		var r AnchorRecord
		var createdAt string
		var invalidatedAt *string
		if err := rows.Scan(&r.AnchorID, &r.Term, &r.TermNormalized, &r.Definition,
			&r.ContextSummary, &r.Scope, &r.AnchorClass, &r.SourceChunkID,
			&r.SourceVersionID, &r.SourceDocID, &r.CorpusScope, &r.PolicySnapshotID,
			&r.SupersededBy, &createdAt, &invalidatedAt); err != nil {
			return nil, err
		}
		createdTime, err := time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, err
		}
		r.CreatedAt = createdTime
		if invalidatedAt != nil {
			invalidatedTime, err := time.Parse(time.RFC3339, *invalidatedAt)
			if err != nil {
				return nil, err
			}
			r.InvalidatedAt = &invalidatedTime
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// InvalidateAnchor marks an anchor as no longer valid. Does not create a replacement.
func InvalidateAnchor(ctx context.Context, db *sql.DB, anchorID string, reason string) error {
	if db == nil {
		return errors.New("db required")
	}

	now := time.Now().UTC()
	res, err := db.ExecContext(ctx, `
		UPDATE retrieval_semantic_anchors
		SET invalidated_at = ?
		WHERE anchor_id = ?
	`, now.Format(time.RFC3339), anchorID)
	if err != nil {
		return err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("anchor %s not found", anchorID)
	}

	// Record the invalidation event
	eventID := generateAnchorID() + "-evt"
	_, err = db.ExecContext(ctx, `
		INSERT INTO retrieval_anchor_events
		(event_id, anchor_id, event_type, detail, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, eventID, anchorID, "invalidated", reason, now.Format(time.RFC3339))

	return err
}

// SupersedeAnchor replaces an anchor with a new definition. Links via superseded_by.
func SupersedeAnchor(ctx context.Context, db *sql.DB, anchorID string, newDefinition string, newContext map[string]string) (*AnchorRecord, error) {
	if db == nil {
		return nil, errors.New("db required")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Get the old anchor
	var oldAnchor AnchorRecord
	var createdAt string
	var invalidatedAt *string
	err = tx.QueryRowContext(ctx, `
		SELECT anchor_id, term, term_normalized, definition, context_summary, scope,
		       anchor_class, source_chunk_id, source_version_id, source_doc_id,
		       corpus_scope, policy_snapshot_id, superseded_by, created_at, invalidated_at
		FROM retrieval_semantic_anchors
		WHERE anchor_id = ?
	`, anchorID).Scan(&oldAnchor.AnchorID, &oldAnchor.Term, &oldAnchor.TermNormalized,
		&oldAnchor.Definition, &oldAnchor.ContextSummary, &oldAnchor.Scope,
		&oldAnchor.AnchorClass, &oldAnchor.SourceChunkID, &oldAnchor.SourceVersionID,
		&oldAnchor.SourceDocID, &oldAnchor.CorpusScope, &oldAnchor.PolicySnapshotID,
		&oldAnchor.SupersededBy, &createdAt, &invalidatedAt)
	if err != nil {
		return nil, err
	}

	createdTime, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, err
	}
	oldAnchor.CreatedAt = createdTime
	if invalidatedAt != nil {
		invalidatedTime, err := time.Parse(time.RFC3339, *invalidatedAt)
		if err != nil {
			return nil, err
		}
		oldAnchor.InvalidatedAt = &invalidatedTime
	}

	// Create new anchor
	newAnchorID := generateAnchorID()
	contextSummary := summarizeContext(newContext)
	now := time.Now().UTC()

	newRecord := AnchorRecord{
		AnchorID:        newAnchorID,
		Term:            oldAnchor.Term,
		TermNormalized:  oldAnchor.TermNormalized,
		Definition:      newDefinition,
		ContextSummary:  contextSummary,
		Scope:           oldAnchor.Scope,
		AnchorClass:     oldAnchor.AnchorClass,
		SourceChunkID:   oldAnchor.SourceChunkID,
		SourceVersionID: oldAnchor.SourceVersionID,
		SourceDocID:     oldAnchor.SourceDocID,
		CorpusScope:     oldAnchor.CorpusScope,
		PolicySnapshotID: oldAnchor.PolicySnapshotID,
		SupersededBy:    nil,
		CreatedAt:       now,
		InvalidatedAt:   nil,
	}

	// Insert new anchor
	_, err = tx.ExecContext(ctx, `
		INSERT INTO retrieval_semantic_anchors
		(anchor_id, term, term_normalized, definition, context_summary, scope,
		 anchor_class, source_chunk_id, source_version_id, source_doc_id,
		 corpus_scope, policy_snapshot_id, superseded_by, created_at, invalidated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, newRecord.AnchorID, newRecord.Term, newRecord.TermNormalized, newRecord.Definition,
		newRecord.ContextSummary, newRecord.Scope, newRecord.AnchorClass,
		newRecord.SourceChunkID, newRecord.SourceVersionID, newRecord.SourceDocID,
		newRecord.CorpusScope, newRecord.PolicySnapshotID, newRecord.SupersededBy,
		newRecord.CreatedAt.Format(time.RFC3339), nil)
	if err != nil {
		return nil, err
	}

	// Mark old anchor as superseded
	_, err = tx.ExecContext(ctx, `
		UPDATE retrieval_semantic_anchors
		SET superseded_by = ?
		WHERE anchor_id = ?
	`, newAnchorID, anchorID)
	if err != nil {
		return nil, err
	}

	// Record the supersession event
	eventID := generateAnchorID() + "-evt"
	_, err = tx.ExecContext(ctx, `
		INSERT INTO retrieval_anchor_events
		(event_id, anchor_id, event_type, detail, old_definition, new_definition, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, eventID, anchorID, "superseded", fmt.Sprintf("superseded by %s", newAnchorID),
		oldAnchor.Definition, newDefinition, now.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &newRecord, nil
}

// UnresolvedDrifts returns anchor drift events that have not been resolved.
func UnresolvedDrifts(ctx context.Context, db *sql.DB, corpusScope string) ([]AnchorEventRecord, error) {
	if db == nil {
		return nil, errors.New("db required")
	}

	rows, err := db.QueryContext(ctx, `
		SELECT e.event_id, e.anchor_id, e.event_type, e.detail,
		       e.old_definition, e.new_definition, e.similarity_score, e.created_at
		FROM retrieval_anchor_events e
		JOIN retrieval_semantic_anchors a ON e.anchor_id = a.anchor_id
		WHERE a.corpus_scope = ? AND e.event_type = 'drift_detected'
		ORDER BY e.created_at DESC
	`, corpusScope)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []AnchorEventRecord
	for rows.Next() {
		var e AnchorEventRecord
		if err := rows.Scan(&e.EventID, &e.AnchorID, &e.EventType, &e.Detail,
			&e.OldDefinition, &e.NewDefinition, &e.SimilarityScore, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// ResolveDrift marks a drift event as resolved (either confirmed or updated).
func ResolveDrift(ctx context.Context, db *sql.DB, eventID string, resolution string) error {
	if db == nil {
		return errors.New("db required")
	}

	res, err := db.ExecContext(ctx, `
		INSERT INTO retrieval_anchor_events
		(event_id, anchor_id, event_type, detail, created_at)
		SELECT ?, anchor_id, ?, ?, ?
		FROM retrieval_anchor_events
		WHERE event_id = ?
	`, generateAnchorID()+"-evt", "drift_resolved", resolution, time.Now().UTC().Format(time.RFC3339), eventID)
	if err != nil {
		return err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("drift event %s not found", eventID)
	}
	return nil
}

// DriftedAnchors returns active anchors that have unresolved drift events.
func DriftedAnchors(ctx context.Context, db *sql.DB, corpusScope string) ([]AnchorRecord, error) {
	if db == nil {
		return nil, errors.New("db required")
	}

	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT a.anchor_id, a.term, a.term_normalized, a.definition, a.context_summary,
		                a.scope, a.anchor_class, a.source_chunk_id, a.source_version_id,
		                a.source_doc_id, a.corpus_scope, a.policy_snapshot_id, a.superseded_by,
		                a.created_at, a.invalidated_at
		FROM retrieval_semantic_anchors a
		JOIN retrieval_anchor_events e ON e.anchor_id = a.anchor_id
		WHERE a.corpus_scope = ? AND a.invalidated_at IS NULL
		  AND e.event_type = 'drift_detected'
		ORDER BY a.created_at DESC
	`, corpusScope)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []AnchorRecord
	for rows.Next() {
		var r AnchorRecord
		var createdAt string
		var invalidatedAt *string
		if err := rows.Scan(&r.AnchorID, &r.Term, &r.TermNormalized, &r.Definition,
			&r.ContextSummary, &r.Scope, &r.AnchorClass, &r.SourceChunkID,
			&r.SourceVersionID, &r.SourceDocID, &r.CorpusScope, &r.PolicySnapshotID,
			&r.SupersededBy, &createdAt, &invalidatedAt); err != nil {
			return nil, err
		}
		createdTime, err := time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, err
		}
		r.CreatedAt = createdTime
		if invalidatedAt != nil {
			invalidatedTime, err := time.Parse(time.RFC3339, *invalidatedAt)
			if err != nil {
				return nil, err
			}
			r.InvalidatedAt = &invalidatedTime
		}
		records = append(records, r)
	}
	return records, rows.Err()
}
