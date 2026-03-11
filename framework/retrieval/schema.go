package retrieval

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// SchemaVersion is the current retrieval schema version.
const SchemaVersion = 4

const schemaMetadataKey = "retrieval_schema_version"

// EnsureSchema creates or upgrades the retrieval tables in an existing SQLite database.
func EnsureSchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return errors.New("retrieval schema db required")
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_metadata (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);`); err != nil {
		return err
	}
	version, err := currentSchemaVersionOrZero(ctx, db)
	if err != nil {
		return err
	}
	if version < 2 {
		if err := migrateToSchemaV2(ctx, db); err != nil {
			return err
		}
	}
	if version < 3 {
		if err := migrateToSchemaV3(ctx, db); err != nil {
			return err
		}
	}
	if version < 4 {
		if err := migrateToSchemaV4(ctx, db); err != nil {
			return err
		}
	}
	_, err = db.ExecContext(ctx,
		`INSERT INTO schema_metadata (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		schemaMetadataKey,
		fmt.Sprintf("%d", SchemaVersion),
	)
	return err
}

// CurrentSchemaVersion reads the installed retrieval schema version.
func CurrentSchemaVersion(ctx context.Context, db *sql.DB) (int, error) {
	if db == nil {
		return 0, errors.New("retrieval schema db required")
	}
	return currentSchemaVersionOrZero(ctx, db)
}

func currentSchemaVersionOrZero(ctx context.Context, db *sql.DB) (int, error) {
	row := db.QueryRowContext(ctx, `SELECT value FROM schema_metadata WHERE key = ?`, schemaMetadataKey)
	var value string
	if err := row.Scan(&value); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	var version int
	if _, err := fmt.Sscanf(value, "%d", &version); err != nil {
		return 0, err
	}
	return version, nil
}

func migrateToSchemaV2(ctx context.Context, db *sql.DB) error {
	if err := createDocumentTablesV2(ctx, db); err != nil {
		return err
	}
	if err := ensureDocumentColumnsV2(ctx, db); err != nil {
		return err
	}
	if err := migrateChunkStorageV2(ctx, db); err != nil {
		return err
	}
	if err := migrateEmbeddingsV2(ctx, db); err != nil {
		return err
	}
	if err := createEventTablesV2(ctx, db); err != nil {
		return err
	}
	return recreateChunkSearchTable(ctx, db)
}

func migrateToSchemaV3(ctx context.Context, db *sql.DB) error {
	if err := ensureColumn(ctx, db, "retrieval_documents", "chunker_version", `ALTER TABLE retrieval_documents ADD COLUMN chunker_version TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if err := ensureColumn(ctx, db, "retrieval_chunk_versions", "chunker_version", `ALTER TABLE retrieval_chunk_versions ADD COLUMN chunker_version TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `UPDATE retrieval_documents
		SET chunker_version = CASE
			WHEN chunker_version <> '' THEN chunker_version
			WHEN source_type = 'go' THEN 'chunk-go-0.1.0'
			WHEN source_type = 'markdown' THEN 'chunk-markdown-0.1.0'
			WHEN source_type IN ('plaintext', 'text', 'txt') THEN 'chunk-text-0.1.0'
			ELSE 'chunk-fallback-0.1.0'
		END`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `UPDATE retrieval_chunk_versions
		SET chunker_version = CASE
			WHEN chunker_version <> '' THEN chunker_version
			ELSE COALESCE((SELECT d.chunker_version FROM retrieval_documents d WHERE d.doc_id = retrieval_chunk_versions.doc_id), 'chunk-fallback-0.1.0')
		END`); err != nil {
		return err
	}
	return nil
}

func migrateToSchemaV4(ctx context.Context, db *sql.DB) error {
	if err := ensureColumn(ctx, db, "retrieval_documents", "source_updated_at", `ALTER TABLE retrieval_documents ADD COLUMN source_updated_at TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if err := ensureColumn(ctx, db, "retrieval_documents", "last_ingested_at", `ALTER TABLE retrieval_documents ADD COLUMN last_ingested_at TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `UPDATE retrieval_documents
		SET source_updated_at = CASE
			WHEN source_updated_at <> '' THEN source_updated_at
			ELSE updated_at
		END,
		last_ingested_at = CASE
			WHEN last_ingested_at <> '' THEN last_ingested_at
			ELSE updated_at
		END`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_retrieval_documents_scope_type_source_updated
		ON retrieval_documents(corpus_scope, source_type, source_updated_at DESC);`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS retrieval_document_policy_tags (
		doc_id TEXT NOT NULL,
		tag TEXT NOT NULL,
		PRIMARY KEY(doc_id, tag),
		FOREIGN KEY(doc_id) REFERENCES retrieval_documents(doc_id) ON DELETE CASCADE
	);`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_retrieval_document_policy_tags_tag_doc
		ON retrieval_document_policy_tags(tag, doc_id);`); err != nil {
		return err
	}
	return backfillDocumentPolicyTagsV4(ctx, db)
}

func createDocumentTablesV2(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS retrieval_documents (
			doc_id TEXT PRIMARY KEY,
			canonical_uri TEXT NOT NULL UNIQUE,
			content_hash TEXT NOT NULL,
			corpus_scope TEXT NOT NULL DEFAULT 'workspace',
			source_type TEXT NOT NULL,
			policy_tags_json TEXT NOT NULL DEFAULT '[]',
			parser_version TEXT NOT NULL DEFAULT '',
			chunker_version TEXT NOT NULL DEFAULT '',
			source_updated_at TEXT NOT NULL,
			last_ingested_at TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS retrieval_document_versions (
			version_id TEXT PRIMARY KEY,
			doc_id TEXT NOT NULL,
			content_hash TEXT NOT NULL,
			ingested_at TEXT NOT NULL,
			superseded INTEGER NOT NULL DEFAULT 0,
			FOREIGN KEY(doc_id) REFERENCES retrieval_documents(doc_id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_retrieval_document_versions_doc_time
			ON retrieval_document_versions(doc_id, ingested_at DESC);`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func ensureDocumentColumnsV2(ctx context.Context, db *sql.DB) error {
	if err := ensureColumn(ctx, db, "retrieval_documents", "corpus_scope", `ALTER TABLE retrieval_documents ADD COLUMN corpus_scope TEXT NOT NULL DEFAULT 'workspace'`); err != nil {
		return err
	}
	if err := ensureColumn(ctx, db, "retrieval_documents", "policy_tags_json", `ALTER TABLE retrieval_documents ADD COLUMN policy_tags_json TEXT NOT NULL DEFAULT '[]'`); err != nil {
		return err
	}
	if err := ensureColumn(ctx, db, "retrieval_documents", "source_updated_at", `ALTER TABLE retrieval_documents ADD COLUMN source_updated_at TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if err := ensureColumn(ctx, db, "retrieval_documents", "last_ingested_at", `ALTER TABLE retrieval_documents ADD COLUMN last_ingested_at TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `UPDATE retrieval_documents
		SET source_updated_at = CASE
			WHEN source_updated_at <> '' THEN source_updated_at
			ELSE updated_at
		END,
		last_ingested_at = CASE
			WHEN last_ingested_at <> '' THEN last_ingested_at
			ELSE updated_at
		END`); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_retrieval_documents_scope_type_updated
		ON retrieval_documents(corpus_scope, source_type, updated_at DESC);`)
	return err
}

func migrateChunkStorageV2(ctx context.Context, db *sql.DB) error {
	hasChunks, err := tableExists(ctx, db, "retrieval_chunks")
	if err != nil {
		return err
	}
	hasChunkVersions, err := tableExists(ctx, db, "retrieval_chunk_versions")
	if err != nil {
		return err
	}
	oldShape := false
	if hasChunks {
		oldShape, err = columnExists(ctx, db, "retrieval_chunks", "version_id")
		if err != nil {
			return err
		}
	}
	if hasChunks && oldShape {
		if _, err := db.ExecContext(ctx, `ALTER TABLE retrieval_chunks RENAME TO retrieval_chunks_legacy`); err != nil {
			return err
		}
		hasChunks = false
	}
	if !hasChunks {
		if err := createChunkTablesV2(ctx, db); err != nil {
			return err
		}
	} else {
		if err := ensureChunkLineageColumnsV2(ctx, db); err != nil {
			return err
		}
	}
	if !hasChunkVersions {
		if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS retrieval_chunk_versions (
			chunk_id TEXT NOT NULL,
			doc_id TEXT NOT NULL,
			version_id TEXT NOT NULL,
			text TEXT NOT NULL,
			structural_key TEXT NOT NULL DEFAULT '',
			chunker_version TEXT NOT NULL DEFAULT '',
			start_offset INTEGER NOT NULL DEFAULT 0,
			end_offset INTEGER NOT NULL DEFAULT 0,
			parent_chunk TEXT NOT NULL DEFAULT '',
			tombstoned INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			PRIMARY KEY(chunk_id, version_id),
			FOREIGN KEY(chunk_id) REFERENCES retrieval_chunks(chunk_id) ON DELETE CASCADE,
			FOREIGN KEY(doc_id) REFERENCES retrieval_documents(doc_id) ON DELETE CASCADE,
			FOREIGN KEY(version_id) REFERENCES retrieval_document_versions(version_id) ON DELETE CASCADE
		);`); err != nil {
			return err
		}
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_retrieval_chunk_versions_doc_version
		ON retrieval_chunk_versions(doc_id, version_id, start_offset ASC);`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_retrieval_chunks_doc_active
		ON retrieval_chunks(doc_id, tombstoned, active_version_id, updated_at DESC);`); err != nil {
		return err
	}
	if legacyExists, err := tableExists(ctx, db, "retrieval_chunks_legacy"); err != nil {
		return err
	} else if legacyExists {
		if err := backfillChunkLegacyRows(ctx, db); err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, `DROP TABLE retrieval_chunks_legacy`); err != nil {
			return err
		}
	}
	return nil
}

func createChunkTablesV2(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS retrieval_chunks (
			chunk_id TEXT PRIMARY KEY,
			doc_id TEXT NOT NULL,
			structural_key TEXT NOT NULL DEFAULT '',
			parent_chunk TEXT NOT NULL DEFAULT '',
			first_version_id TEXT NOT NULL DEFAULT '',
			last_version_id TEXT NOT NULL DEFAULT '',
			active_version_id TEXT NOT NULL DEFAULT '',
			tombstoned INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(doc_id) REFERENCES retrieval_documents(doc_id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS retrieval_chunk_versions (
			chunk_id TEXT NOT NULL,
			doc_id TEXT NOT NULL,
			version_id TEXT NOT NULL,
			text TEXT NOT NULL,
			structural_key TEXT NOT NULL DEFAULT '',
			chunker_version TEXT NOT NULL DEFAULT '',
			start_offset INTEGER NOT NULL DEFAULT 0,
			end_offset INTEGER NOT NULL DEFAULT 0,
			parent_chunk TEXT NOT NULL DEFAULT '',
			tombstoned INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			PRIMARY KEY(chunk_id, version_id),
			FOREIGN KEY(chunk_id) REFERENCES retrieval_chunks(chunk_id) ON DELETE CASCADE,
			FOREIGN KEY(doc_id) REFERENCES retrieval_documents(doc_id) ON DELETE CASCADE,
			FOREIGN KEY(version_id) REFERENCES retrieval_document_versions(version_id) ON DELETE CASCADE
		);`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func ensureChunkLineageColumnsV2(ctx context.Context, db *sql.DB) error {
	columnDDLs := []struct {
		column string
		ddl    string
	}{
		{"first_version_id", `ALTER TABLE retrieval_chunks ADD COLUMN first_version_id TEXT NOT NULL DEFAULT ''`},
		{"last_version_id", `ALTER TABLE retrieval_chunks ADD COLUMN last_version_id TEXT NOT NULL DEFAULT ''`},
		{"active_version_id", `ALTER TABLE retrieval_chunks ADD COLUMN active_version_id TEXT NOT NULL DEFAULT ''`},
		{"updated_at", `ALTER TABLE retrieval_chunks ADD COLUMN updated_at TEXT NOT NULL DEFAULT ''`},
	}
	for _, item := range columnDDLs {
		if err := ensureColumn(ctx, db, "retrieval_chunks", item.column, item.ddl); err != nil {
			return err
		}
	}
	return nil
}

func backfillChunkLegacyRows(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `INSERT INTO retrieval_chunks
		(chunk_id, doc_id, structural_key, parent_chunk, first_version_id, last_version_id, active_version_id, tombstoned, created_at, updated_at)
		SELECT
			chunk_id,
			doc_id,
			structural_key,
			parent_chunk,
			version_id,
			version_id,
			CASE WHEN tombstoned = 0 THEN version_id ELSE '' END,
			tombstoned,
			created_at,
			created_at
		FROM retrieval_chunks_legacy`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO retrieval_chunk_versions
		(chunk_id, doc_id, version_id, text, structural_key, start_offset, end_offset, parent_chunk, tombstoned, created_at)
		SELECT
			chunk_id,
			doc_id,
			version_id,
			text,
			structural_key,
			start_offset,
			end_offset,
			parent_chunk,
			tombstoned,
			created_at
		FROM retrieval_chunks_legacy`); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateEmbeddingsV2(ctx context.Context, db *sql.DB) error {
	hasEmbeddings, err := tableExists(ctx, db, "retrieval_embeddings")
	if err != nil {
		return err
	}
	if !hasEmbeddings {
		return createEmbeddingsTableV2(ctx, db)
	}
	hasVersionID, err := columnExists(ctx, db, "retrieval_embeddings", "version_id")
	if err != nil {
		return err
	}
	if hasVersionID {
		return createEmbeddingIndexesV2(ctx, db)
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE retrieval_embeddings RENAME TO retrieval_embeddings_legacy`); err != nil {
		return err
	}
	if err := createEmbeddingsTableV2(ctx, db); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO retrieval_embeddings
		(chunk_id, version_id, model_id, vector, generated_at)
		SELECT
			e.chunk_id,
			COALESCE(c.active_version_id, ''),
			e.model_id,
			e.vector,
			e.generated_at
		FROM retrieval_embeddings_legacy e
		LEFT JOIN retrieval_chunks c ON c.chunk_id = e.chunk_id`); err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `DROP TABLE retrieval_embeddings_legacy`)
	return err
}

func createEmbeddingsTableV2(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS retrieval_embeddings (
		chunk_id TEXT NOT NULL,
		version_id TEXT NOT NULL,
		model_id TEXT NOT NULL,
		vector BLOB NOT NULL,
		generated_at TEXT NOT NULL,
		PRIMARY KEY(chunk_id, version_id, model_id),
		FOREIGN KEY(chunk_id, version_id) REFERENCES retrieval_chunk_versions(chunk_id, version_id) ON DELETE CASCADE
	);`); err != nil {
		return err
	}
	return createEmbeddingIndexesV2(ctx, db)
}

func createEmbeddingIndexesV2(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_retrieval_embeddings_model_time
		ON retrieval_embeddings(model_id, generated_at DESC);`)
	return err
}

func createEventTablesV2(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS retrieval_events (
			query_id TEXT PRIMARY KEY,
			query_text TEXT NOT NULL,
			filter_summary TEXT NOT NULL DEFAULT '',
			sparse_candidates INTEGER NOT NULL DEFAULT 0,
			dense_candidates INTEGER NOT NULL DEFAULT 0,
			fused_candidates INTEGER NOT NULL DEFAULT 0,
			excluded_reasons_json TEXT NOT NULL DEFAULT '{}',
			cache_tier TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS retrieval_packing_events (
			query_id TEXT PRIMARY KEY,
			injected_chunks_json TEXT NOT NULL DEFAULT '[]',
			dropped_chunks_json TEXT NOT NULL DEFAULT '[]',
			token_budget INTEGER NOT NULL DEFAULT 0,
			tokens_used INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		);`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func backfillDocumentPolicyTagsV4(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `SELECT doc_id, policy_tags_json FROM retrieval_documents`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type item struct {
		docID string
		tags  []string
	}
	docs := make([]item, 0)
	for rows.Next() {
		var docID string
		var raw string
		if err := rows.Scan(&docID, &raw); err != nil {
			return err
		}
		var tags []string
		if strings.TrimSpace(raw) != "" {
			if err := json.Unmarshal([]byte(raw), &tags); err != nil {
				return err
			}
		}
		docs = append(docs, item{docID: docID, tags: cloneStrings(tags)})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM retrieval_document_policy_tags`); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO retrieval_document_policy_tags (doc_id, tag) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, doc := range docs {
		for _, tag := range doc.tags {
			if _, err := stmt.ExecContext(ctx, doc.docID, tag); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func recreateChunkSearchTable(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS retrieval_chunks_fts`); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `CREATE VIRTUAL TABLE retrieval_chunks_fts USING fts5(
		chunk_id UNINDEXED,
		doc_id UNINDEXED,
		version_id UNINDEXED,
		text
	);`)
	if err == nil {
		return nil
	}
	if !strings.Contains(strings.ToLower(err.Error()), "no such module: fts5") {
		return err
	}
	_, fallbackErr := db.ExecContext(ctx, `CREATE TABLE retrieval_chunks_fts (
		chunk_id TEXT NOT NULL,
		doc_id TEXT NOT NULL,
		version_id TEXT NOT NULL,
		text TEXT NOT NULL,
		PRIMARY KEY(chunk_id, version_id)
	);`)
	return fallbackErr
}

func tableExists(ctx context.Context, db *sql.DB, table string) (bool, error) {
	row := db.QueryRowContext(ctx, `SELECT 1 FROM sqlite_master WHERE type='table' AND name = ?`, table)
	var exists int
	if err := row.Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func columnExists(ctx context.Context, db *sql.DB, table, column string) (bool, error) {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, pk int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

func ensureColumn(ctx context.Context, db *sql.DB, table, column, ddl string) error {
	exists, err := columnExists(ctx, db, table, column)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = db.ExecContext(ctx, ddl)
	return err
}
