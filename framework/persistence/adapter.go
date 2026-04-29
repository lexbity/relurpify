package persistence

import (
	"context"
	"encoding/json"
)

// Adapter is the generic persistence adapter interface.
// It provides basic CRUD operations that can be implemented by graphdb or other backends.
// This is intentionally small and domain-agnostic - business logic lives in domain packages.
type Adapter interface {
	// Load retrieves a record by its entity ID and unmarshals it into target.
	Load(ctx context.Context, entityKind, entityID string, target any) error

	// Store marshals and stores a record with schema metadata.
	Store(ctx context.Context, entityKind, entityID string, record any, metadata SchemaMetadata) error

	// Append adds a record to an ordered collection (e.g., events, transitions).
	Append(ctx context.Context, entityKind, parentID string, record any, metadata SchemaMetadata) error

	// List retrieves records of a given kind, optionally filtered by parent ID.
	List(ctx context.Context, entityKind string, parentID string, target any) error

	// FindByIndex retrieves records by an indexed field value.
	FindByIndex(ctx context.Context, entityKind, indexName string, indexValue string, target any) error

	// Close closes the adapter and releases resources.
	Close() error
}

// Record represents a persisted record with schema metadata.
type Record struct {
	Metadata SchemaMetadata    `json:"metadata"`
	Data     json.RawMessage   `json:"data"`
}

// MarshalRecord serializes a domain record with schema metadata.
func MarshalRecord(record any, metadata SchemaMetadata) (*Record, error) {
	data, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}

	return &Record{
		Metadata: metadata,
		Data:     data,
	}, nil
}

// UnmarshalRecord deserializes a persisted record into a domain type.
func UnmarshalRecord(record *Record, target any) error {
	return json.Unmarshal(record.Data, target)
}
