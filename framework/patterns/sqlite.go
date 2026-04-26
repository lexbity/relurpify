package patterns

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func OpenSQLite(path string) (*sql.DB, error) {
	return sql.Open("sqlite3", path)
}

type SQLitePatternStore struct {
	db *sql.DB
}

type SQLiteCommentStore struct {
	db *sql.DB
}

func NewSQLitePatternStore(db *sql.DB) (*SQLitePatternStore, error) {
	if db == nil {
		return nil, errors.New("patterns: db required")
	}
	if err := ensureSchema(context.Background(), db); err != nil {
		return nil, err
	}
	return &SQLitePatternStore{db: db}, nil
}

func NewSQLiteCommentStore(db *sql.DB) (*SQLiteCommentStore, error) {
	if db == nil {
		return nil, errors.New("patterns: db required")
	}
	if err := ensureSchema(context.Background(), db); err != nil {
		return nil, err
	}
	return &SQLiteCommentStore{db: db}, nil
}

func ensureSchema(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS pattern_records (
			id TEXT PRIMARY KEY,
			kind TEXT NOT NULL,
			title TEXT NOT NULL,
			description TEXT NOT NULL,
			status TEXT NOT NULL,
			instances_json TEXT NOT NULL,
			comment_ids_json TEXT NOT NULL,
			anchor_refs_json TEXT NOT NULL,
			corpus_scope TEXT NOT NULL,
			corpus_source TEXT NOT NULL,
			confirmed_by TEXT NOT NULL DEFAULT '',
			confirmed_at TEXT,
			superseded_by TEXT NOT NULL DEFAULT '',
			confidence REAL NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_pattern_records_status_scope ON pattern_records(status, corpus_scope);`,
		`CREATE INDEX IF NOT EXISTS idx_pattern_records_kind_scope ON pattern_records(kind, corpus_scope);`,
		`CREATE TABLE IF NOT EXISTS comment_records (
			comment_id TEXT PRIMARY KEY,
			pattern_id TEXT NOT NULL DEFAULT '',
			anchor_id TEXT NOT NULL DEFAULT '',
			tension_id TEXT NOT NULL DEFAULT '',
			file_path TEXT NOT NULL DEFAULT '',
			symbol_id TEXT NOT NULL DEFAULT '',
			intent_type TEXT NOT NULL,
			body TEXT NOT NULL,
			author_kind TEXT NOT NULL,
			trust_class TEXT NOT NULL,
			anchor_ref TEXT NOT NULL DEFAULT '',
			corpus_scope TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_comment_records_pattern ON comment_records(pattern_id);`,
		`CREATE INDEX IF NOT EXISTS idx_comment_records_anchor ON comment_records(anchor_id);`,
		`CREATE INDEX IF NOT EXISTS idx_comment_records_tension ON comment_records(tension_id);`,
		`CREATE INDEX IF NOT EXISTS idx_comment_records_symbol ON comment_records(symbol_id);`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE comment_records ADD COLUMN tension_id TEXT NOT NULL DEFAULT ''`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
		return err
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_comment_records_tension ON comment_records(tension_id);`); err != nil {
		return err
	}
	return nil
}

func (s *SQLitePatternStore) Save(ctx context.Context, record PatternRecord) error {
	if s == nil || s.db == nil {
		return errors.New("patterns: store not initialized")
	}
	now := time.Now().UTC()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = now
	}
	instances, err := marshalJSON(record.Instances)
	if err != nil {
		return err
	}
	commentIDs, err := marshalJSON(record.CommentIDs)
	if err != nil {
		return err
	}
	anchorRefs, err := marshalJSON(record.AnchorRefs)
	if err != nil {
		return err
	}
	var confirmedAt any
	if record.ConfirmedAt != nil {
		confirmedAt = record.ConfirmedAt.UTC().Format(time.RFC3339Nano)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO pattern_records (
		id, kind, title, description, status, instances_json, comment_ids_json,
		anchor_refs_json, corpus_scope, corpus_source, confirmed_by, confirmed_at,
		superseded_by, confidence, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		kind = excluded.kind,
		title = excluded.title,
		description = excluded.description,
		status = excluded.status,
		instances_json = excluded.instances_json,
		comment_ids_json = excluded.comment_ids_json,
		anchor_refs_json = excluded.anchor_refs_json,
		corpus_scope = excluded.corpus_scope,
		corpus_source = excluded.corpus_source,
		confirmed_by = excluded.confirmed_by,
		confirmed_at = excluded.confirmed_at,
		superseded_by = excluded.superseded_by,
		confidence = excluded.confidence,
		created_at = excluded.created_at,
		updated_at = excluded.updated_at`,
		record.ID, record.Kind, record.Title, record.Description, record.Status,
		instances, commentIDs, anchorRefs, record.CorpusScope, record.CorpusSource,
		record.ConfirmedBy, confirmedAt, record.SupersededBy, record.Confidence,
		record.CreatedAt.UTC().Format(time.RFC3339Nano), record.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *SQLitePatternStore) Load(ctx context.Context, id string) (*PatternRecord, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, kind, title, description, status, instances_json, comment_ids_json,
		anchor_refs_json, corpus_scope, corpus_source, confirmed_by, confirmed_at, superseded_by,
		confidence, created_at, updated_at FROM pattern_records WHERE id = ?`, id)
	return scanPatternRecord(row)
}

func (s *SQLitePatternStore) ListByStatus(ctx context.Context, status PatternStatus, corpusScope string) ([]PatternRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, kind, title, description, status, instances_json, comment_ids_json,
		anchor_refs_json, corpus_scope, corpus_source, confirmed_by, confirmed_at, superseded_by,
		confidence, created_at, updated_at
		FROM pattern_records WHERE status = ? AND corpus_scope = ? ORDER BY created_at ASC`, status, corpusScope)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPatternRecords(rows)
}

func (s *SQLitePatternStore) ListByKind(ctx context.Context, kind PatternKind, corpusScope string) ([]PatternRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, kind, title, description, status, instances_json, comment_ids_json,
		anchor_refs_json, corpus_scope, corpus_source, confirmed_by, confirmed_at, superseded_by,
		confidence, created_at, updated_at
		FROM pattern_records WHERE kind = ? AND corpus_scope = ? ORDER BY created_at ASC`, kind, corpusScope)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPatternRecords(rows)
}

func (s *SQLitePatternStore) UpdateStatus(ctx context.Context, id string, status PatternStatus, confirmedBy string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var confirmedAt any
	if status == PatternStatusConfirmed {
		confirmedAt = now
	}
	_, err := s.db.ExecContext(ctx, `UPDATE pattern_records
		SET status = ?, confirmed_by = ?, confirmed_at = COALESCE(?, confirmed_at), updated_at = ?
		WHERE id = ?`, status, confirmedBy, confirmedAt, now, id)
	return err
}

func (s *SQLitePatternStore) Supersede(ctx context.Context, oldID string, replacement PatternRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `UPDATE pattern_records
		SET status = ?, superseded_by = ?, updated_at = ?
		WHERE id = ?`, PatternStatusSuperseded, replacement.ID, now, oldID); err != nil {
		return err
	}
	if replacement.Status == "" {
		replacement.Status = PatternStatusConfirmed
	}
	if replacement.UpdatedAt.IsZero() {
		replacement.UpdatedAt = time.Now().UTC()
	}
	if replacement.CreatedAt.IsZero() {
		replacement.CreatedAt = replacement.UpdatedAt
	}
	instances, err := marshalJSON(replacement.Instances)
	if err != nil {
		return err
	}
	commentIDs, err := marshalJSON(replacement.CommentIDs)
	if err != nil {
		return err
	}
	anchorRefs, err := marshalJSON(replacement.AnchorRefs)
	if err != nil {
		return err
	}
	var confirmedAt any
	if replacement.ConfirmedAt != nil {
		confirmedAt = replacement.ConfirmedAt.UTC().Format(time.RFC3339Nano)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO pattern_records (
		id, kind, title, description, status, instances_json, comment_ids_json,
		anchor_refs_json, corpus_scope, corpus_source, confirmed_by, confirmed_at,
		superseded_by, confidence, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		replacement.ID, replacement.Kind, replacement.Title, replacement.Description, replacement.Status,
		instances, commentIDs, anchorRefs, replacement.CorpusScope, replacement.CorpusSource,
		replacement.ConfirmedBy, confirmedAt, replacement.SupersededBy, replacement.Confidence,
		replacement.CreatedAt.UTC().Format(time.RFC3339Nano), replacement.UpdatedAt.UTC().Format(time.RFC3339Nano),
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteCommentStore) Save(ctx context.Context, record CommentRecord) error {
	if s == nil || s.db == nil {
		return errors.New("patterns: store not initialized")
	}
	now := time.Now().UTC()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = now
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO comment_records (
		comment_id, pattern_id, anchor_id, tension_id, file_path, symbol_id, intent_type, body,
		author_kind, trust_class, anchor_ref, corpus_scope, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(comment_id) DO UPDATE SET
		pattern_id = excluded.pattern_id,
		anchor_id = excluded.anchor_id,
		tension_id = excluded.tension_id,
		file_path = excluded.file_path,
		symbol_id = excluded.symbol_id,
		intent_type = excluded.intent_type,
		body = excluded.body,
		author_kind = excluded.author_kind,
		trust_class = excluded.trust_class,
		anchor_ref = excluded.anchor_ref,
		corpus_scope = excluded.corpus_scope,
		created_at = excluded.created_at,
		updated_at = excluded.updated_at`,
		record.CommentID, record.PatternID, record.AnchorID, record.TensionID, record.FilePath, record.SymbolID,
		record.IntentType, record.Body, record.AuthorKind, record.TrustClass, record.AnchorRef,
		record.CorpusScope, record.CreatedAt.UTC().Format(time.RFC3339Nano), record.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *SQLiteCommentStore) Load(ctx context.Context, id string) (*CommentRecord, error) {
	row := s.db.QueryRowContext(ctx, `SELECT comment_id, pattern_id, anchor_id, tension_id, file_path, symbol_id,
		intent_type, body, author_kind, trust_class, anchor_ref, corpus_scope, created_at, updated_at
		FROM comment_records WHERE comment_id = ?`, id)
	return scanCommentRecord(row)
}

func (s *SQLiteCommentStore) ListForPattern(ctx context.Context, patternID string) ([]CommentRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT comment_id, pattern_id, anchor_id, tension_id, file_path, symbol_id,
		intent_type, body, author_kind, trust_class, anchor_ref, corpus_scope, created_at, updated_at
		FROM comment_records WHERE pattern_id = ? ORDER BY created_at ASC`, patternID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCommentRecords(rows)
}

func (s *SQLiteCommentStore) ListForAnchor(ctx context.Context, anchorID string) ([]CommentRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT comment_id, pattern_id, anchor_id, tension_id, file_path, symbol_id,
		intent_type, body, author_kind, trust_class, anchor_ref, corpus_scope, created_at, updated_at
		FROM comment_records WHERE anchor_id = ? ORDER BY created_at ASC`, anchorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCommentRecords(rows)
}

func (s *SQLiteCommentStore) ListForTension(ctx context.Context, tensionID string) ([]CommentRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT comment_id, pattern_id, anchor_id, tension_id, file_path, symbol_id,
		intent_type, body, author_kind, trust_class, anchor_ref, corpus_scope, created_at, updated_at
		FROM comment_records WHERE tension_id = ? ORDER BY created_at ASC`, tensionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCommentRecords(rows)
}

func (s *SQLiteCommentStore) ListForSymbol(ctx context.Context, symbolID string) ([]CommentRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT comment_id, pattern_id, anchor_id, tension_id, file_path, symbol_id,
		intent_type, body, author_kind, trust_class, anchor_ref, corpus_scope, created_at, updated_at
		FROM comment_records WHERE symbol_id = ? ORDER BY created_at ASC`, symbolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCommentRecords(rows)
}

type patternScanner interface {
	Scan(dest ...any) error
}

func scanPatternRecord(row patternScanner) (*PatternRecord, error) {
	var (
		record                                     PatternRecord
		instancesJSON, commentIDsJSON, refs        string
		confirmedAtRaw, createdAtRaw, updatedAtRaw sql.NullString
	)
	if err := row.Scan(&record.ID, &record.Kind, &record.Title, &record.Description, &record.Status,
		&instancesJSON, &commentIDsJSON, &refs, &record.CorpusScope, &record.CorpusSource,
		&record.ConfirmedBy, &confirmedAtRaw, &record.SupersededBy, &record.Confidence,
		&createdAtRaw, &updatedAtRaw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if err := json.Unmarshal([]byte(instancesJSON), &record.Instances); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(commentIDsJSON), &record.CommentIDs); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(refs), &record.AnchorRefs); err != nil {
		return nil, err
	}
	if createdAtRaw.Valid {
		parsed, err := time.Parse(time.RFC3339Nano, createdAtRaw.String)
		if err != nil {
			return nil, err
		}
		record.CreatedAt = parsed
	}
	if updatedAtRaw.Valid {
		parsed, err := time.Parse(time.RFC3339Nano, updatedAtRaw.String)
		if err != nil {
			return nil, err
		}
		record.UpdatedAt = parsed
	}
	if confirmedAtRaw.Valid && confirmedAtRaw.String != "" {
		parsed, err := time.Parse(time.RFC3339Nano, confirmedAtRaw.String)
		if err != nil {
			return nil, err
		}
		record.ConfirmedAt = &parsed
	}
	return &record, nil
}

func scanPatternRecords(rows *sql.Rows) ([]PatternRecord, error) {
	out := make([]PatternRecord, 0)
	for rows.Next() {
		record, err := scanPatternRecord(rows)
		if err != nil {
			return nil, err
		}
		if record != nil {
			out = append(out, *record)
		}
	}
	return out, rows.Err()
}

type commentScanner interface {
	Scan(dest ...any) error
}

func scanCommentRecord(row commentScanner) (*CommentRecord, error) {
	var record CommentRecord
	var createdAtRaw, updatedAtRaw string
	if err := row.Scan(&record.CommentID, &record.PatternID, &record.AnchorID, &record.TensionID, &record.FilePath, &record.SymbolID,
		&record.IntentType, &record.Body, &record.AuthorKind, &record.TrustClass, &record.AnchorRef,
		&record.CorpusScope, &createdAtRaw, &updatedAtRaw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	createdAt, err := time.Parse(time.RFC3339Nano, createdAtRaw)
	if err != nil {
		return nil, err
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, updatedAtRaw)
	if err != nil {
		return nil, err
	}
	record.CreatedAt = createdAt
	record.UpdatedAt = updatedAt
	return &record, nil
}

func scanCommentRecords(rows *sql.Rows) ([]CommentRecord, error) {
	out := make([]CommentRecord, 0)
	for rows.Next() {
		record, err := scanCommentRecord(rows)
		if err != nil {
			return nil, err
		}
		if record != nil {
			out = append(out, *record)
		}
	}
	return out, rows.Err()
}

func marshalJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
