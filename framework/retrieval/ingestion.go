package retrieval

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	goparser "go/parser"
	"go/token"
	"math"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	frameworkast "github.com/lexcodex/relurpify/framework/ast"
)

const (
	defaultTextChunkChars = 1200
	defaultTextOverlap    = 120
	defaultParserVersion  = "retrieval-0.1.0"
)

var topLevelDeclPattern = regexp.MustCompile(`^(func\s+\([^)]+\)\s*[A-Za-z_][A-Za-z0-9_]*|func\s+[A-Za-z_][A-Za-z0-9_]*|type\s+[A-Za-z_][A-Za-z0-9_]*|const\s*\(|var\s*\(|const\s+[A-Za-z_][A-Za-z0-9_]*|var\s+[A-Za-z_][A-Za-z0-9_]*)`)

// IngestionPipeline parses, chunks, embeds, and persists source material.
type IngestionPipeline struct {
	db             *sql.DB
	embedder       Embedder
	detector       *frameworkast.LanguageDetector
	textChunkChars int
	textOverlap    int
	now            func() time.Time
}

// IngestRequest describes a single ingestion operation.
type IngestRequest struct {
	CanonicalURI   string
	Content        []byte
	SourceType     string
	CorpusScope    string
	PolicyTags     []string
	SourceUpdatedAt *time.Time
	SkipEmbeddings bool
}

// IngestResult summarizes the persisted records.
type IngestResult struct {
	Document   DocumentRecord
	Version    DocumentVersionRecord
	Chunks     []ChunkRecord
	Embeddings []EmbeddingRecord
}

// NewIngestionPipeline constructs a pipeline over an existing retrieval-enabled SQLite handle.
func NewIngestionPipeline(db *sql.DB, embedder Embedder) *IngestionPipeline {
	return &IngestionPipeline{
		db:             db,
		embedder:       embedder,
		detector:       frameworkast.NewLanguageDetector(),
		textChunkChars: defaultTextChunkChars,
		textOverlap:    defaultTextOverlap,
		now:            func() time.Time { return time.Now().UTC() },
	}
}

// IngestFile reads a local file and ingests its contents.
func (p *IngestionPipeline) IngestFile(ctx context.Context, path string, corpusScope string, policyTags []string) (*IngestResult, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	sourceUpdatedAt := info.ModTime().UTC()
	return p.Ingest(ctx, IngestRequest{
		CanonicalURI: path,
		Content:      content,
		CorpusScope:  corpusScope,
		PolicyTags:   policyTags,
		SourceUpdatedAt: &sourceUpdatedAt,
	})
}

// Ingest parses, chunks, and persists one document revision plus optional embeddings.
func (p *IngestionPipeline) Ingest(ctx context.Context, req IngestRequest) (*IngestResult, error) {
	if p == nil || p.db == nil {
		return nil, errors.New("ingestion pipeline db required")
	}
	if err := EnsureSchema(ctx, p.db); err != nil {
		return nil, err
	}
	uri := CanonicalizeURI(req.CanonicalURI)
	if uri == "" {
		return nil, errors.New("canonical uri required")
	}
	now := p.now()
	content := string(req.Content)
	sourceType := strings.TrimSpace(req.SourceType)
	if sourceType == "" {
		sourceType = p.detector.Detect(uri)
	}
	parserVersion := parserVersionFor(sourceType)
	sourceUpdatedAt := now
	if req.SourceUpdatedAt != nil {
		sourceUpdatedAt = req.SourceUpdatedAt.UTC()
	}
	doc := NewDocumentRecord(uri, req.Content, sourceType, parserVersion, req.CorpusScope, req.PolicyTags, sourceUpdatedAt, now)
	existing, err := loadCurrentDocumentState(ctx, p.db, doc.DocID)
	if err != nil {
		return nil, err
	}
	if existing != nil && existing.Document.ContentHash == doc.ContentHash {
		doc.CreatedAt = existing.Document.CreatedAt
		doc.SourceUpdatedAt = existing.Document.SourceUpdatedAt
		doc.LastIngestedAt = now
		doc.UpdatedAt = now
		if err := updateDocumentMetadata(ctx, p.db, doc); err != nil {
			return nil, err
		}
		embeddings := existing.Embeddings
		if !req.SkipEmbeddings && p.embedder != nil {
			if embeddings, err = p.BackfillEmbeddings(ctx, BackfillRequest{DocID: doc.DocID, VersionID: existing.Version.VersionID}); err != nil {
				return nil, err
			}
		}
		return &IngestResult{
			Document:   doc,
			Version:    existing.Version,
			Chunks:     existing.Chunks,
			Embeddings: embeddings,
		}, nil
	}
	version := NewDocumentVersionRecord(doc, now)
	chunks, err := p.chunkDocument(doc, version, content, now)
	if err != nil {
		return nil, err
	}
	for i := range chunks {
		chunks[i].ChunkerVersion = doc.ChunkerVersion
	}
	var embeddings []EmbeddingRecord
	if !req.SkipEmbeddings {
		embeddings, err = p.buildEmbeddings(ctx, chunks, now)
		if err != nil {
			return nil, err
		}
	}
	if err := persistIngestedDocument(ctx, p.db, doc, version, chunks, embeddings); err != nil {
		return nil, err
	}
	return &IngestResult{
		Document:   doc,
		Version:    version,
		Chunks:     chunks,
		Embeddings: embeddings,
	}, nil
}

// TombstoneDocument logically removes a document and its active chunks from retrieval.
func (p *IngestionPipeline) TombstoneDocument(ctx context.Context, canonicalURI string) error {
	if p == nil || p.db == nil {
		return errors.New("ingestion pipeline db required")
	}
	if err := EnsureSchema(ctx, p.db); err != nil {
		return err
	}
	docID := DeriveDocID(canonicalURI)
	now := p.now().Format(time.RFC3339Nano)
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE retrieval_chunks
		SET tombstoned = 1, active_version_id = '', updated_at = ?
		WHERE doc_id = ?`, now, docID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE retrieval_chunk_versions
		SET tombstoned = 1
		WHERE doc_id = ?`, docID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE retrieval_document_versions
		SET superseded = 1
		WHERE doc_id = ?`, docID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM retrieval_chunks_fts WHERE doc_id = ?`, docID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE retrieval_documents SET updated_at = ? WHERE doc_id = ?`, now, docID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM retrieval_document_policy_tags WHERE doc_id = ?`, docID); err != nil {
		return err
	}
	return tx.Commit()
}

// BackfillRequest scopes embedding generation for already persisted chunks.
type BackfillRequest struct {
	DocID     string
	VersionID string
}

// BackfillEmbeddings generates any missing embeddings for persisted chunk versions.
func (p *IngestionPipeline) BackfillEmbeddings(ctx context.Context, req BackfillRequest) ([]EmbeddingRecord, error) {
	if p == nil || p.db == nil {
		return nil, errors.New("ingestion pipeline db required")
	}
	if p.embedder == nil {
		return nil, nil
	}
	if err := EnsureSchema(ctx, p.db); err != nil {
		return nil, err
	}
	rows, err := p.db.QueryContext(ctx, `SELECT cv.chunk_id, cv.doc_id, cv.version_id, cv.text, cv.structural_key, cv.start_offset, cv.end_offset, cv.parent_chunk, cv.tombstoned, cv.created_at
		FROM retrieval_chunk_versions cv
		LEFT JOIN retrieval_embeddings e
			ON e.chunk_id = cv.chunk_id AND e.version_id = cv.version_id AND e.model_id = ?
		WHERE (? = '' OR cv.doc_id = ?)
		  AND (? = '' OR cv.version_id = ?)
		  AND cv.tombstoned = 0
		  AND e.chunk_id IS NULL
		ORDER BY cv.doc_id, cv.version_id, cv.start_offset ASC`,
		p.embedder.ModelID(), req.DocID, req.DocID, req.VersionID, req.VersionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	chunks := make([]ChunkRecord, 0)
	for rows.Next() {
		var chunk ChunkRecord
		var createdAt string
		var tombstoned int
		if err := rows.Scan(&chunk.ChunkID, &chunk.DocID, &chunk.VersionID, &chunk.Text, &chunk.StructuralKey, &chunk.StartOffset, &chunk.EndOffset, &chunk.ParentChunk, &tombstoned, &createdAt); err != nil {
			return nil, err
		}
		chunk.Tombstoned = tombstoned != 0
		chunk.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, chunk)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(chunks) == 0 {
		return loadEmbeddings(ctx, p.db, p.embedder.ModelID(), req)
	}
	embeddings, err := p.buildEmbeddings(ctx, chunks, p.now())
	if err != nil {
		return nil, err
	}
	if err := persistEmbeddings(ctx, p.db, embeddings); err != nil {
		return nil, err
	}
	return loadEmbeddings(ctx, p.db, p.embedder.ModelID(), req)
}

func (p *IngestionPipeline) chunkDocument(doc DocumentRecord, version DocumentVersionRecord, content string, now time.Time) ([]ChunkRecord, error) {
	switch doc.SourceType {
	case "go":
		chunks, err := chunkGoDocument(doc, version, content, now)
		if err == nil && len(chunks) > 0 {
			return chunks, nil
		}
	case "markdown":
		chunks := chunkMarkdownDocument(doc, version, content, now)
		if len(chunks) > 0 {
			return chunks, nil
		}
	case "plaintext", "text", "txt":
		return chunkPlainTextDocument(doc, version, content, p.textChunkChars, p.textOverlap, now), nil
	}
	return chunkFallbackDocument(doc, version, content, p.textChunkChars, p.textOverlap, now), nil
}

func (p *IngestionPipeline) buildEmbeddings(ctx context.Context, chunks []ChunkRecord, now time.Time) ([]EmbeddingRecord, error) {
	if p.embedder == nil || len(chunks) == 0 {
		return nil, nil
	}
	texts := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		texts = append(texts, chunk.Text)
	}
	vectors, err := p.embedder.Embed(ctx, texts)
	if err != nil {
		return nil, err
	}
	if len(vectors) != len(chunks) {
		return nil, fmt.Errorf("embedder returned %d vectors for %d chunks", len(vectors), len(chunks))
	}
	out := make([]EmbeddingRecord, 0, len(chunks))
	for i, chunk := range chunks {
		out = append(out, EmbeddingRecord{
			ChunkID:     chunk.ChunkID,
			VersionID:   chunk.VersionID,
			ModelID:     p.embedder.ModelID(),
			Vector:      append([]float32(nil), vectors[i]...),
			GeneratedAt: now,
		})
	}
	return out, nil
}

func persistIngestedDocument(ctx context.Context, db *sql.DB, doc DocumentRecord, version DocumentVersionRecord, chunks []ChunkRecord, embeddings []EmbeddingRecord) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	policyTagsJSON, err := json.Marshal(doc.PolicyTags)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO retrieval_documents
		(doc_id, canonical_uri, content_hash, corpus_scope, source_type, policy_tags_json, parser_version, chunker_version, source_updated_at, last_ingested_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(doc_id) DO UPDATE SET
			canonical_uri = excluded.canonical_uri,
			content_hash = excluded.content_hash,
			corpus_scope = excluded.corpus_scope,
			source_type = excluded.source_type,
			policy_tags_json = excluded.policy_tags_json,
			parser_version = excluded.parser_version,
			chunker_version = excluded.chunker_version,
			source_updated_at = excluded.source_updated_at,
			last_ingested_at = excluded.last_ingested_at,
			updated_at = excluded.updated_at`,
		doc.DocID, doc.CanonicalURI, doc.ContentHash, doc.CorpusScope, doc.SourceType, string(policyTagsJSON), doc.ParserVersion, doc.ChunkerVersion,
		doc.SourceUpdatedAt.Format(time.RFC3339Nano), doc.LastIngestedAt.Format(time.RFC3339Nano),
		doc.CreatedAt.Format(time.RFC3339Nano), doc.UpdatedAt.Format(time.RFC3339Nano),
	); err != nil {
		return err
	}
	if err := replaceDocumentPolicyTags(ctx, tx, doc.DocID, doc.PolicyTags); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE retrieval_document_versions SET superseded = 1 WHERE doc_id = ? AND version_id <> ?`,
		doc.DocID, version.VersionID,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO retrieval_document_versions
		(version_id, doc_id, content_hash, ingested_at, superseded)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(version_id) DO UPDATE SET
			content_hash = excluded.content_hash,
			ingested_at = excluded.ingested_at,
			superseded = excluded.superseded`,
		version.VersionID, version.DocID, version.ContentHash,
		version.IngestedAt.Format(time.RFC3339Nano), boolToInt(version.Superseded),
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE retrieval_chunks
			SET tombstoned = 1,
				active_version_id = '',
				updated_at = ?
		  WHERE doc_id = ?`,
		version.IngestedAt.Format(time.RFC3339Nano), doc.DocID,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM retrieval_chunks_fts WHERE doc_id = ?`, doc.DocID); err != nil {
		return err
	}
	chunkStmt, err := tx.PrepareContext(ctx, `INSERT INTO retrieval_chunks
		(chunk_id, doc_id, structural_key, parent_chunk, first_version_id, last_version_id, active_version_id, tombstoned, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(chunk_id) DO UPDATE SET
			doc_id = excluded.doc_id,
			structural_key = excluded.structural_key,
			parent_chunk = excluded.parent_chunk,
			last_version_id = excluded.last_version_id,
			active_version_id = excluded.active_version_id,
			tombstoned = excluded.tombstoned,
			updated_at = excluded.updated_at`)
	if err != nil {
		return err
	}
	defer chunkStmt.Close()
	chunkVersionStmt, err := tx.PrepareContext(ctx, `INSERT INTO retrieval_chunk_versions
		(chunk_id, doc_id, version_id, text, structural_key, chunker_version, start_offset, end_offset, parent_chunk, tombstoned, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(chunk_id, version_id) DO UPDATE SET
			text = excluded.text,
			structural_key = excluded.structural_key,
			chunker_version = excluded.chunker_version,
			start_offset = excluded.start_offset,
			end_offset = excluded.end_offset,
			parent_chunk = excluded.parent_chunk,
			tombstoned = excluded.tombstoned,
			created_at = excluded.created_at`)
	if err != nil {
		return err
	}
	defer chunkVersionStmt.Close()
	ftsStmt, err := tx.PrepareContext(ctx, `INSERT INTO retrieval_chunks_fts (chunk_id, doc_id, version_id, text) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer ftsStmt.Close()
	for _, chunk := range chunks {
		if _, err := chunkStmt.ExecContext(ctx,
			chunk.ChunkID, chunk.DocID, chunk.StructuralKey, chunk.ParentChunk, chunk.VersionID, chunk.VersionID, chunk.VersionID,
			boolToInt(chunk.Tombstoned), chunk.CreatedAt.Format(time.RFC3339Nano), version.IngestedAt.Format(time.RFC3339Nano),
		); err != nil {
			return err
		}
		chunkVersion := chunk.ChunkVersion()
		if _, err := chunkVersionStmt.ExecContext(ctx,
			chunkVersion.ChunkID, chunkVersion.DocID, chunkVersion.VersionID, chunkVersion.Text, chunkVersion.StructuralKey, chunkVersion.ChunkerVersion,
			chunkVersion.StartOffset, chunkVersion.EndOffset, chunkVersion.ParentChunk, boolToInt(chunkVersion.Tombstoned),
			chunkVersion.CreatedAt.Format(time.RFC3339Nano),
		); err != nil {
			return err
		}
		if _, err := ftsStmt.ExecContext(ctx, chunk.ChunkID, chunk.DocID, chunk.VersionID, chunk.Text); err != nil {
			return err
		}
	}
	if len(embeddings) > 0 {
		embedStmt, err := tx.PrepareContext(ctx, `INSERT INTO retrieval_embeddings
			(chunk_id, version_id, model_id, vector, generated_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(chunk_id, version_id, model_id) DO UPDATE SET
				vector = excluded.vector,
				generated_at = excluded.generated_at`)
		if err != nil {
			return err
		}
		defer embedStmt.Close()
		for _, embedding := range embeddings {
			if _, err := embedStmt.ExecContext(ctx,
				embedding.ChunkID, embedding.VersionID, embedding.ModelID, encodeVector(embedding.Vector),
				embedding.GeneratedAt.Format(time.RFC3339Nano),
			); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

type currentDocumentState struct {
	Document   DocumentRecord
	Version    DocumentVersionRecord
	Chunks     []ChunkRecord
	Embeddings []EmbeddingRecord
}

func loadCurrentDocumentState(ctx context.Context, db *sql.DB, docID string) (*currentDocumentState, error) {
	row := db.QueryRowContext(ctx, `SELECT doc_id, canonical_uri, content_hash, corpus_scope, source_type, policy_tags_json, parser_version, chunker_version, source_updated_at, last_ingested_at, created_at, updated_at
		FROM retrieval_documents
		WHERE doc_id = ?`, docID)
	var doc DocumentRecord
	var policyTagsJSON string
	var sourceUpdatedAt string
	var lastIngestedAt string
	var createdAt string
	var updatedAt string
	if err := row.Scan(&doc.DocID, &doc.CanonicalURI, &doc.ContentHash, &doc.CorpusScope, &doc.SourceType, &policyTagsJSON, &doc.ParserVersion, &doc.ChunkerVersion, &sourceUpdatedAt, &lastIngestedAt, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if err := json.Unmarshal([]byte(policyTagsJSON), &doc.PolicyTags); err != nil {
		return nil, err
	}
	var err error
	doc.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, err
	}
	doc.SourceUpdatedAt, err = time.Parse(time.RFC3339Nano, sourceUpdatedAt)
	if err != nil {
		return nil, err
	}
	doc.LastIngestedAt, err = time.Parse(time.RFC3339Nano, lastIngestedAt)
	if err != nil {
		return nil, err
	}
	doc.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return nil, err
	}

	row = db.QueryRowContext(ctx, `SELECT version_id, doc_id, content_hash, ingested_at, superseded
		FROM retrieval_document_versions
		WHERE doc_id = ? AND superseded = 0
		ORDER BY ingested_at DESC
		LIMIT 1`, docID)
	var version DocumentVersionRecord
	var ingestedAt string
	var superseded int
	if err := row.Scan(&version.VersionID, &version.DocID, &version.ContentHash, &ingestedAt, &superseded); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &currentDocumentState{Document: doc}, nil
		}
		return nil, err
	}
	version.IngestedAt, err = time.Parse(time.RFC3339Nano, ingestedAt)
	if err != nil {
		return nil, err
	}
	version.Superseded = superseded != 0

	chunks, err := loadChunkVersions(ctx, db, BackfillRequest{DocID: docID, VersionID: version.VersionID})
	if err != nil {
		return nil, err
	}
	return &currentDocumentState{
		Document: doc,
		Version:  version,
		Chunks:   chunks,
	}, nil
}

func updateDocumentMetadata(ctx context.Context, db *sql.DB, doc DocumentRecord) error {
	policyTagsJSON, err := json.Marshal(doc.PolicyTags)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE retrieval_documents
		SET canonical_uri = ?,
			content_hash = ?,
			corpus_scope = ?,
			source_type = ?,
			policy_tags_json = ?,
			parser_version = ?,
			chunker_version = ?,
			source_updated_at = ?,
			last_ingested_at = ?,
			updated_at = ?
			WHERE doc_id = ?`,
		doc.CanonicalURI, doc.ContentHash, doc.CorpusScope, doc.SourceType, string(policyTagsJSON), doc.ParserVersion, doc.ChunkerVersion,
		doc.SourceUpdatedAt.Format(time.RFC3339Nano), doc.LastIngestedAt.Format(time.RFC3339Nano),
		doc.UpdatedAt.Format(time.RFC3339Nano), doc.DocID); err != nil {
		return err
	}
	if err := replaceDocumentPolicyTags(ctx, tx, doc.DocID, doc.PolicyTags); err != nil {
		return err
	}
	return tx.Commit()
}

func replaceDocumentPolicyTags(ctx context.Context, tx *sql.Tx, docID string, policyTags []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM retrieval_document_policy_tags WHERE doc_id = ?`, docID); err != nil {
		return err
	}
	tags := cloneStrings(policyTags)
	if len(tags) == 0 {
		return nil
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO retrieval_document_policy_tags (doc_id, tag) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, tag := range tags {
		if _, err := stmt.ExecContext(ctx, docID, tag); err != nil {
			return err
		}
	}
	return nil
}

func loadChunkVersions(ctx context.Context, db *sql.DB, req BackfillRequest) ([]ChunkRecord, error) {
	rows, err := db.QueryContext(ctx, `SELECT chunk_id, doc_id, version_id, text, structural_key, chunker_version, start_offset, end_offset, parent_chunk, tombstoned, created_at
		FROM retrieval_chunk_versions
		WHERE (? = '' OR doc_id = ?)
		  AND (? = '' OR version_id = ?)
		ORDER BY start_offset ASC`,
		req.DocID, req.DocID, req.VersionID, req.VersionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ChunkRecord, 0)
	for rows.Next() {
		var chunk ChunkRecord
		var createdAt string
		var tombstoned int
		if err := rows.Scan(&chunk.ChunkID, &chunk.DocID, &chunk.VersionID, &chunk.Text, &chunk.StructuralKey, &chunk.ChunkerVersion, &chunk.StartOffset, &chunk.EndOffset, &chunk.ParentChunk, &tombstoned, &createdAt); err != nil {
			return nil, err
		}
		chunk.Tombstoned = tombstoned != 0
		chunk.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, err
		}
		out = append(out, chunk)
	}
	return out, rows.Err()
}

func persistEmbeddings(ctx context.Context, db *sql.DB, embeddings []EmbeddingRecord) error {
	if len(embeddings) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO retrieval_embeddings
		(chunk_id, version_id, model_id, vector, generated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(chunk_id, version_id, model_id) DO UPDATE SET
			vector = excluded.vector,
			generated_at = excluded.generated_at`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, embedding := range embeddings {
		if _, err := stmt.ExecContext(ctx,
			embedding.ChunkID, embedding.VersionID, embedding.ModelID, encodeVector(embedding.Vector),
			embedding.GeneratedAt.Format(time.RFC3339Nano),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func loadEmbeddings(ctx context.Context, db *sql.DB, modelID string, req BackfillRequest) ([]EmbeddingRecord, error) {
	rows, err := db.QueryContext(ctx, `SELECT chunk_id, version_id, model_id, vector, generated_at
		FROM retrieval_embeddings
		WHERE model_id = ?
		  AND (? = '' OR chunk_id IN (SELECT chunk_id FROM retrieval_chunk_versions WHERE doc_id = ?))
		  AND (? = '' OR version_id = ?)
		ORDER BY generated_at ASC`, modelID, req.DocID, req.DocID, req.VersionID, req.VersionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]EmbeddingRecord, 0)
	for rows.Next() {
		var embedding EmbeddingRecord
		var generatedAt string
		var vectorBlob []byte
		if err := rows.Scan(&embedding.ChunkID, &embedding.VersionID, &embedding.ModelID, &vectorBlob, &generatedAt); err != nil {
			return nil, err
		}
		embedding.GeneratedAt, err = time.Parse(time.RFC3339Nano, generatedAt)
		if err != nil {
			return nil, err
		}
		embedding.Vector = decodeVector(vectorBlob)
		out = append(out, embedding)
	}
	return out, rows.Err()
}

func chunkGoDocument(doc DocumentRecord, version DocumentVersionRecord, content string, now time.Time) ([]ChunkRecord, error) {
	fset := token.NewFileSet()
	file, err := goparser.ParseFile(fset, doc.CanonicalURI, content, goparser.ParseComments)
	if err != nil {
		return nil, err
	}
	type chunkCandidate struct {
		name       string
		start, end int
	}
	candidates := make([]chunkCandidate, 0, len(file.Decls))
	for _, decl := range file.Decls {
		start := fset.Position(decl.Pos()).Offset
		end := fset.Position(decl.End()).Offset
		if start < 0 || end <= start || end > len(content) {
			continue
		}
		name := goDeclName(decl)
		candidates = append(candidates, chunkCandidate{name: name, start: start, end: end})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].start < candidates[j].start })
	out := make([]ChunkRecord, 0, len(candidates))
	for _, candidate := range candidates {
		text := strings.TrimSpace(content[candidate.start:candidate.end])
		if text == "" {
			continue
		}
		out = append(out, NewChunkRecord(
			doc.DocID,
			version.VersionID,
			text,
			"go/"+candidate.name,
			candidate.start,
			candidate.end,
			now,
		))
	}
	return out, nil
}

func chunkMarkdownDocument(doc DocumentRecord, version DocumentVersionRecord, content string, now time.Time) []ChunkRecord {
	lines := strings.Split(content, "\n")
	type heading struct {
		title string
		start int
		level int
	}
	headings := make([]heading, 0)
	offset := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			level := 0
			for level < len(trimmed) && trimmed[level] == '#' {
				level++
			}
			if level > 0 && level < len(trimmed) && trimmed[level] == ' ' {
				headings = append(headings, heading{
					title: strings.TrimSpace(trimmed[level:]),
					start: offset,
					level: level,
				})
			}
		}
		offset += len(line) + 1
	}
	if len(headings) == 0 {
		return chunkPlainTextDocument(doc, version, content, defaultTextChunkChars, defaultTextOverlap, now)
	}
	out := make([]ChunkRecord, 0, len(headings))
	for i, h := range headings {
		end := len(content)
		if i+1 < len(headings) {
			end = headings[i+1].start
		}
		text := strings.TrimSpace(content[h.start:end])
		if text == "" {
			continue
		}
		out = append(out, NewChunkRecord(
			doc.DocID,
			version.VersionID,
			text,
			"md/"+slugify(h.title),
			h.start,
			end,
			now,
		))
	}
	return out
}

func chunkPlainTextDocument(doc DocumentRecord, version DocumentVersionRecord, content string, chunkChars, overlap int, now time.Time) []ChunkRecord {
	paragraphs := splitParagraphs(content)
	if len(paragraphs) == 0 {
		return nil
	}
	out := make([]ChunkRecord, 0, len(paragraphs))
	for i, part := range paragraphs {
		key := fmt.Sprintf("text/paragraph-%03d", i+1)
		out = append(out, NewChunkRecord(doc.DocID, version.VersionID, part.text, key, part.start, part.end, now))
	}
	return mergeOversizedChunks(doc, version, out, chunkChars, overlap, now)
}

func chunkFallbackDocument(doc DocumentRecord, version DocumentVersionRecord, content string, chunkChars, overlap int, now time.Time) []ChunkRecord {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil
	}
	if looksStructuredByLines(content) {
		return chunkLineWindows(doc, version, content, chunkChars, overlap, now)
	}
	return chunkLineWindows(doc, version, content, chunkChars, overlap, now)
}

type textSpan struct {
	text       string
	start, end int
}

func splitParagraphs(content string) []textSpan {
	raw := strings.Split(content, "\n\n")
	out := make([]textSpan, 0, len(raw))
	searchFrom := 0
	for _, part := range raw {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			searchFrom += len(part) + 2
			continue
		}
		idx := strings.Index(content[searchFrom:], part)
		if idx < 0 {
			continue
		}
		start := searchFrom + idx
		end := start + len(part)
		out = append(out, textSpan{text: trimmed, start: start, end: end})
		searchFrom = end
	}
	return out
}

func mergeOversizedChunks(doc DocumentRecord, version DocumentVersionRecord, chunks []ChunkRecord, chunkChars, overlap int, now time.Time) []ChunkRecord {
	out := make([]ChunkRecord, 0, len(chunks))
	for _, chunk := range chunks {
		if len(chunk.Text) <= chunkChars || chunkChars <= 0 {
			out = append(out, chunk)
			continue
		}
		split := chunkTextWindowed(doc, version, chunk.Text, chunk.StartOffset, "text/window", chunkChars, overlap, now)
		out = append(out, split...)
	}
	return out
}

func chunkLineWindows(doc DocumentRecord, version DocumentVersionRecord, content string, chunkChars, overlap int, now time.Time) []ChunkRecord {
	return chunkTextWindowed(doc, version, content, 0, "fallback/window", chunkChars, overlap, now)
}

func chunkTextWindowed(doc DocumentRecord, version DocumentVersionRecord, content string, baseOffset int, prefix string, chunkChars, overlap int, now time.Time) []ChunkRecord {
	if chunkChars <= 0 {
		chunkChars = defaultTextChunkChars
	}
	if overlap < 0 {
		overlap = 0
	}
	out := make([]ChunkRecord, 0)
	start := 0
	index := 1
	for start < len(content) {
		end := start + chunkChars
		if end > len(content) {
			end = len(content)
		}
		if end < len(content) {
			if cut := strings.LastIndex(content[start:end], "\n"); cut > chunkChars/3 {
				end = start + cut
			}
		}
		text := strings.TrimSpace(content[start:end])
		if text != "" {
			key := fmt.Sprintf("%s-%03d", prefix, index)
			out = append(out, NewChunkRecord(doc.DocID, version.VersionID, text, key, baseOffset+start, baseOffset+end, now))
			index++
		}
		if end == len(content) {
			break
		}
		nextStart := end - overlap
		if nextStart <= start {
			nextStart = end
		}
		start = nextStart
	}
	return out
}

func goDeclName(decl ast.Decl) string {
	switch typed := decl.(type) {
	case *ast.FuncDecl:
		if typed.Recv != nil && len(typed.Recv.List) > 0 {
			return fmt.Sprintf("%s.%s", receiverTypeName(typed.Recv.List[0].Type), typed.Name.Name)
		}
		return typed.Name.Name
	case *ast.GenDecl:
		if len(typed.Specs) == 0 {
			return typed.Tok.String()
		}
		switch spec := typed.Specs[0].(type) {
		case *ast.TypeSpec:
			return spec.Name.Name
		case *ast.ValueSpec:
			if len(spec.Names) > 0 {
				return spec.Names[0].Name
			}
		}
		return typed.Tok.String()
	default:
		return "decl"
	}
}

func receiverTypeName(expr ast.Expr) string {
	switch typed := expr.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.StarExpr:
		return receiverTypeName(typed.X)
	case *ast.SelectorExpr:
		return typed.Sel.Name
	default:
		return "recv"
	}
}

func parserVersionFor(sourceType string) string {
	switch sourceType {
	case "go", "markdown":
		return "0.1.0"
	default:
		return defaultParserVersion
	}
}

func looksStructuredByLines(content string) bool {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return false
	}
	matches := 0
	for _, line := range lines {
		if topLevelDeclPattern.MatchString(strings.TrimSpace(line)) {
			matches++
		}
	}
	return matches >= 2
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func encodeVector(vector []float32) []byte {
	buf := make([]byte, 4*len(vector))
	for i, v := range vector {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

func decodeVector(blob []byte) []float32 {
	if len(blob) == 0 {
		return nil
	}
	out := make([]float32, 0, len(blob)/4)
	for i := 0; i+4 <= len(blob); i += 4 {
		out = append(out, math.Float32frombits(binary.LittleEndian.Uint32(blob[i:i+4])))
	}
	return out
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
