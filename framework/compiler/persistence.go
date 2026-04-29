package compiler

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/persistence"
)

// Repository defines the compiler-specific persistence adapter interface.
// This is separate from the lifecycle repository to keep compiler state ownership explicit.
// The compiler owns compilation records, cache entries, and compiler-produced artifacts.
type Repository interface {
	// Compilation record operations
	StoreCompilationRecord(ctx context.Context, record CompilationRecord) error
	GetCompilationRecord(ctx context.Context, requestID string) (*CompilationRecord, error)
	ListCompilationRecords(ctx context.Context, eventLogSeq uint64) ([]CompilationRecord, error)

	// Cache entry operations
	StoreCacheEntry(ctx context.Context, entry CacheEntry) error
	GetCacheEntry(ctx context.Context, key CacheKey) (*CacheEntry, error)
	InvalidateCacheEntries(ctx context.Context, chunkIDs []string) error

	// Compiler artifact operations
	StoreCompilerArtifact(ctx context.Context, artifact CompilerArtifact) error
	GetCompilerArtifact(ctx context.Context, artifactID string) (*CompilerArtifact, error)
	ListCompilerArtifacts(ctx context.Context, requestID string) ([]CompilerArtifact, error)

	// Close
	Close() error
}

// CompilerArtifact represents a compiler-produced artifact (e.g., candidate records, replay metadata).
type CompilerArtifact struct {
	ArtifactID          string
	RequestID           string
	ArtifactKind        string
	ContentType         string
	Payload             []byte
	Metadata            map[string]any
	CreatedAt           int64
}

// Adapter is the generic persistence adapter that the compiler repository depends on.
// This is implemented by framework/persistence and backed by framework/graphdb.
type Adapter interface {
	// Load retrieves a record by its entity ID and unmarshals it into target.
	Load(ctx context.Context, entityKind, entityID string, target any) error

	// Store marshals and stores a record with schema metadata.
	Store(ctx context.Context, entityKind, entityID string, record any, metadata persistence.SchemaMetadata) error

	// Append adds a record to an ordered collection.
	Append(ctx context.Context, entityKind, parentID string, record any, metadata persistence.SchemaMetadata) error

	// List retrieves records of a given kind, optionally filtered by parent ID.
	List(ctx context.Context, entityKind string, parentID string, target any) error

	// FindByIndex retrieves records by an indexed field value.
	FindByIndex(ctx context.Context, entityKind, indexName string, indexValue string, target any) error

	// Close closes the adapter and releases resources.
	Close() error
}
