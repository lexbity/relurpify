package retrieval

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// DocumentRecord is the durable identity of an ingested source.
type DocumentRecord struct {
	DocID          string    `json:"doc_id"`
	CanonicalURI   string    `json:"canonical_uri"`
	ContentHash    string    `json:"content_hash"`
	CorpusScope    string    `json:"corpus_scope"`
	SourceType     string    `json:"source_type"`
	PolicyTags     []string  `json:"policy_tags,omitempty"`
	ParserVersion  string    `json:"parser_version"`
	ChunkerVersion string    `json:"chunker_version"`
	SourceUpdatedAt time.Time `json:"source_updated_at"`
	LastIngestedAt  time.Time `json:"last_ingested_at"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// DocumentVersionRecord records each ingested revision for a document lineage.
type DocumentVersionRecord struct {
	DocID       string    `json:"doc_id"`
	VersionID   string    `json:"version_id"`
	ContentHash string    `json:"content_hash"`
	IngestedAt  time.Time `json:"ingested_at"`
	Superseded  bool      `json:"superseded"`
}

// ChunkRecord is a durable logical subdivision of a document version.
type ChunkRecord struct {
	ChunkID        string    `json:"chunk_id"`
	DocID          string    `json:"doc_id"`
	VersionID      string    `json:"version_id"`
	Text           string    `json:"text"`
	StructuralKey  string    `json:"structural_key,omitempty"`
	ChunkerVersion string    `json:"chunker_version"`
	StartOffset    int       `json:"start_offset"`
	EndOffset      int       `json:"end_offset"`
	ParentChunk    string    `json:"parent_chunk,omitempty"`
	Tombstoned     bool      `json:"tombstoned"`
	CreatedAt      time.Time `json:"created_at"`
}

// ChunkVersionRecord stores an append-only chunk revision scoped to a document version.
type ChunkVersionRecord struct {
	ChunkID        string    `json:"chunk_id"`
	DocID          string    `json:"doc_id"`
	VersionID      string    `json:"version_id"`
	Text           string    `json:"text"`
	StructuralKey  string    `json:"structural_key,omitempty"`
	ChunkerVersion string    `json:"chunker_version"`
	StartOffset    int       `json:"start_offset"`
	EndOffset      int       `json:"end_offset"`
	ParentChunk    string    `json:"parent_chunk,omitempty"`
	Tombstoned     bool      `json:"tombstoned"`
	CreatedAt      time.Time `json:"created_at"`
}

// EmbeddingRecord associates an embedding vector with a chunk and model version.
type EmbeddingRecord struct {
	ChunkID     string    `json:"chunk_id"`
	VersionID   string    `json:"version_id"`
	ModelID     string    `json:"model_id"`
	Vector      []float32 `json:"vector"`
	GeneratedAt time.Time `json:"generated_at"`
}

// NewDocumentRecord materializes the durable identity for an ingested source.
func NewDocumentRecord(canonicalURI string, content []byte, sourceType, parserVersion, corpusScope string, policyTags []string, sourceUpdatedAt, now time.Time) DocumentRecord {
	ts := normalizeTimestamp(now)
	sourceTS := normalizeTimestamp(sourceUpdatedAt)
	uri := CanonicalizeURI(canonicalURI)
	chunkerVersion := chunkerVersionFor(sourceType)
	return DocumentRecord{
		DocID:          DeriveDocID(uri),
		CanonicalURI:   uri,
		ContentHash:    HashContent(content),
		CorpusScope:    normalizeScope(corpusScope),
		SourceType:     strings.TrimSpace(sourceType),
		PolicyTags:     cloneStrings(policyTags),
		ParserVersion:  strings.TrimSpace(parserVersion),
		ChunkerVersion: chunkerVersion,
		SourceUpdatedAt: sourceTS,
		LastIngestedAt:  ts,
		CreatedAt:      ts,
		UpdatedAt:      ts,
	}
}

// NewDocumentVersionRecord creates the append-only version row for an ingest.
func NewDocumentVersionRecord(doc DocumentRecord, ingestedAt time.Time) DocumentVersionRecord {
	ts := normalizeTimestamp(ingestedAt)
	return DocumentVersionRecord{
		DocID:       doc.DocID,
		VersionID:   DeriveVersionID(doc.DocID, doc.ContentHash),
		ContentHash: doc.ContentHash,
		IngestedAt:  ts,
	}
}

// NewChunkRecord creates a stable chunk identity for a document lineage.
func NewChunkRecord(docID, versionID, text, structuralKey string, startOffset, endOffset int, createdAt time.Time) ChunkRecord {
	return ChunkRecord{
		ChunkID:        DeriveChunkID(docID, structuralKey, startOffset, endOffset),
		DocID:          strings.TrimSpace(docID),
		VersionID:      strings.TrimSpace(versionID),
		Text:           text,
		StructuralKey:  normalizeStructuralKey(structuralKey),
		ChunkerVersion: "",
		StartOffset:    startOffset,
		EndOffset:      endOffset,
		CreatedAt:      normalizeTimestamp(createdAt),
	}
}

// ChunkVersion materializes the append-only row for a chunk revision.
func (c ChunkRecord) ChunkVersion() ChunkVersionRecord {
	return ChunkVersionRecord{
		ChunkID:        c.ChunkID,
		DocID:          c.DocID,
		VersionID:      c.VersionID,
		Text:           c.Text,
		StructuralKey:  c.StructuralKey,
		ChunkerVersion: c.ChunkerVersion,
		StartOffset:    c.StartOffset,
		EndOffset:      c.EndOffset,
		ParentChunk:    c.ParentChunk,
		Tombstoned:     c.Tombstoned,
		CreatedAt:      c.CreatedAt,
	}
}

// CanonicalizeURI normalizes local paths while leaving non-file URIs intact.
func CanonicalizeURI(uri string) string {
	trimmed := strings.TrimSpace(uri)
	if trimmed == "" {
		return ""
	}
	if looksLikeURI(trimmed) {
		return trimmed
	}
	cleaned := filepath.Clean(trimmed)
	return filepath.ToSlash(cleaned)
}

// DeriveDocID returns a stable logical document identifier.
func DeriveDocID(canonicalURI string) string {
	return "doc:" + shortStableHash(CanonicalizeURI(canonicalURI))
}

// DeriveVersionID returns a stable version identifier scoped to a document lineage.
func DeriveVersionID(docID, contentHash string) string {
	return "ver:" + shortStableHash(strings.TrimSpace(docID)+"\n"+strings.TrimSpace(contentHash))
}

// DeriveChunkID returns a stable chunk identifier for a document lineage.
func DeriveChunkID(docID, structuralKey string, startOffset, endOffset int) string {
	key := normalizeStructuralKey(structuralKey)
	if key == "" {
		key = fmt.Sprintf("%d:%d", startOffset, endOffset)
	}
	return "chunk:" + shortStableHash(strings.TrimSpace(docID)+"\n"+key)
}

// HashContent returns the canonical SHA-256 hex digest for source content.
func HashContent(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func normalizeStructuralKey(key string) string {
	key = strings.TrimSpace(key)
	key = strings.TrimPrefix(key, "/")
	return strings.ReplaceAll(key, "\\", "/")
}

func normalizeTimestamp(ts time.Time) time.Time {
	if ts.IsZero() {
		return time.Now().UTC()
	}
	return ts.UTC()
}

func shortStableHash(input string) string {
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:8])
}

func looksLikeURI(value string) bool {
	i := strings.Index(value, "://")
	return i > 0
}

func normalizeScope(scope string) string {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return "workspace"
	}
	return scope
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func chunkerVersionFor(sourceType string) string {
	switch strings.TrimSpace(sourceType) {
	case "go":
		return "chunk-go-0.1.0"
	case "markdown":
		return "chunk-markdown-0.1.0"
	case "plaintext", "text", "txt":
		return "chunk-text-0.1.0"
	default:
		return "chunk-fallback-0.1.0"
	}
}
