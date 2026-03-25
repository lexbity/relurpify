package retrieval

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

func TestEnsureSchemaCreatesRetrievalTables(t *testing.T) {
	db, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()
	require.NoError(t, EnsureSchema(ctx, db))

	version, err := CurrentSchemaVersion(ctx, db)
	require.NoError(t, err)
	require.Equal(t, SchemaVersion, version)

	for _, table := range []string{
		"retrieval_documents",
		"retrieval_document_versions",
		"retrieval_chunks",
		"retrieval_chunk_versions",
		"retrieval_chunks_fts",
		"retrieval_embeddings",
		"retrieval_events",
		"retrieval_packing_events",
	} {
		var name string
		err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE name = ?`, table).Scan(&name)
		require.NoError(t, err, table)
		require.Equal(t, table, name)
	}
	requireHasColumn(t, db, "retrieval_documents", "corpus_scope")
	requireHasColumn(t, db, "retrieval_documents", "policy_tags_json")
	requireHasColumn(t, db, "retrieval_documents", "chunker_version")
	requireHasColumn(t, db, "retrieval_documents", "source_updated_at")
	requireHasColumn(t, db, "retrieval_documents", "last_ingested_at")
	requireHasColumn(t, db, "retrieval_chunks", "active_version_id")
	requireHasColumn(t, db, "retrieval_chunk_versions", "chunker_version")
	requireHasColumn(t, db, "retrieval_embeddings", "version_id")
	var name string
	err = db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE name = 'retrieval_document_policy_tags'`).Scan(&name)
	require.NoError(t, err)
	require.Equal(t, "retrieval_document_policy_tags", name)
}

func TestEnsureSchemaMigratesLegacyChunkStorage(t *testing.T) {
	db, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()
	_, err = db.ExecContext(ctx, `CREATE TABLE schema_metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO schema_metadata (key, value) VALUES ('retrieval_schema_version', '1')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `CREATE TABLE retrieval_documents (
		doc_id TEXT PRIMARY KEY,
		canonical_uri TEXT NOT NULL UNIQUE,
		content_hash TEXT NOT NULL,
		source_type TEXT NOT NULL,
		parser_version TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `CREATE TABLE retrieval_document_versions (
		version_id TEXT PRIMARY KEY,
		doc_id TEXT NOT NULL,
		content_hash TEXT NOT NULL,
		ingested_at TEXT NOT NULL,
		superseded INTEGER NOT NULL DEFAULT 0
	)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `CREATE TABLE retrieval_chunks (
		chunk_id TEXT PRIMARY KEY,
		doc_id TEXT NOT NULL,
		version_id TEXT NOT NULL,
		text TEXT NOT NULL,
		structural_key TEXT NOT NULL DEFAULT '',
		start_offset INTEGER NOT NULL DEFAULT 0,
		end_offset INTEGER NOT NULL DEFAULT 0,
		parent_chunk TEXT NOT NULL DEFAULT '',
		tombstoned INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL
	)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `CREATE TABLE retrieval_embeddings (
		chunk_id TEXT NOT NULL,
		model_id TEXT NOT NULL,
		vector BLOB NOT NULL,
		generated_at TEXT NOT NULL,
		PRIMARY KEY(chunk_id, model_id)
	)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO retrieval_documents
		(doc_id, canonical_uri, content_hash, source_type, parser_version, created_at, updated_at)
		VALUES ('doc:1', 'docs/spec.md', 'abc', 'markdown', 'v1', '2026-03-11T12:00:00Z', '2026-03-11T12:00:00Z')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO retrieval_document_versions
		(version_id, doc_id, content_hash, ingested_at, superseded)
		VALUES ('ver:1', 'doc:1', 'abc', '2026-03-11T12:00:00Z', 0)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO retrieval_chunks
		(chunk_id, doc_id, version_id, text, structural_key, start_offset, end_offset, parent_chunk, tombstoned, created_at)
		VALUES ('chunk:1', 'doc:1', 'ver:1', 'hello', 'md/intro', 0, 5, '', 0, '2026-03-11T12:00:00Z')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO retrieval_embeddings
		(chunk_id, model_id, vector, generated_at)
		VALUES ('chunk:1', 'fake-v1', x'00000000', '2026-03-11T12:00:00Z')`)
	require.NoError(t, err)

	require.NoError(t, EnsureSchema(ctx, db))

	version, err := CurrentSchemaVersion(ctx, db)
	require.NoError(t, err)
	require.Equal(t, SchemaVersion, version)
	requireHasColumn(t, db, "retrieval_documents", "corpus_scope")
	requireHasColumn(t, db, "retrieval_documents", "chunker_version")
	requireHasColumn(t, db, "retrieval_documents", "source_updated_at")
	requireHasColumn(t, db, "retrieval_documents", "last_ingested_at")
	requireHasColumn(t, db, "retrieval_chunks", "active_version_id")
	requireHasColumn(t, db, "retrieval_chunk_versions", "chunker_version")
	requireHasColumn(t, db, "retrieval_embeddings", "version_id")

	var sourceUpdatedAt string
	err = db.QueryRowContext(ctx, `SELECT source_updated_at FROM retrieval_documents WHERE doc_id = 'doc:1'`).Scan(&sourceUpdatedAt)
	require.NoError(t, err)
	require.Equal(t, "2026-03-11T12:00:00Z", sourceUpdatedAt)

	var activeVersion string
	err = db.QueryRowContext(ctx, `SELECT active_version_id FROM retrieval_chunks WHERE chunk_id = 'chunk:1'`).Scan(&activeVersion)
	require.NoError(t, err)
	require.Equal(t, "ver:1", activeVersion)

	var chunkerVersion string
	err = db.QueryRowContext(ctx, `SELECT chunker_version FROM retrieval_documents WHERE doc_id = 'doc:1'`).Scan(&chunkerVersion)
	require.NoError(t, err)
	require.Equal(t, "chunk-markdown-0.1.0", chunkerVersion)

	var versionCount int
	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM retrieval_chunk_versions WHERE chunk_id = 'chunk:1'`).Scan(&versionCount)
	require.NoError(t, err)
	require.Equal(t, 1, versionCount)

	var activeChunkCount string
	err = db.QueryRowContext(ctx, `SELECT value FROM schema_metadata WHERE key = ?`, activeChunkCountMetadataKey).Scan(&activeChunkCount)
	require.NoError(t, err)
	require.Equal(t, "1", activeChunkCount)
}

func requireHasColumn(t *testing.T, db *sql.DB, table, column string) {
	t.Helper()
	exists, err := columnExists(context.Background(), db, table, column)
	require.NoError(t, err)
	require.True(t, exists, "%s.%s", table, column)
}
