// Package ingestion implements the six-stage ingestion pipeline.
// This is the pre-runtime pipeline for scanning and indexing workspace content.
package ingestion

import (
	"context"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextpolicy"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
)

// RawIngestion represents Stage 1: Acquisition
type RawIngestion struct {
	Content           []byte
	SourcePrincipal   identity.SubjectRef
	AcquisitionMethod string
	AcquiredAt        time.Time
	MIMEHint          string
	FilePath          string // optional, for file-based ingestion
}

// TypedIngestion represents Stage 2: Parsing+Typing
type TypedIngestion struct {
	Content           []byte          // Raw content
	ContentType       string          // e.g., "go", "python", "markdown", "text"
	StructuredRepr    interface{}     // AST or structured representation
	ChunkBoundaries   []ChunkBoundary // Where to split content into chunks
	Metadata          map[string]any  // Preliminary metadata
	SourcePrincipal   identity.SubjectRef
	AcquisitionMethod string
	AcquiredAt        time.Time
}

// ChunkBoundary marks a span of content for chunking.
type ChunkBoundary struct {
	Start int
	End   int
	Type  string // e.g., "function", "struct", "section", "paragraph"
	Name  string // e.g., function name, heading text
}

// TypedChunk is a chunk of typed content ready for scanning.
type TypedChunk struct {
	Content     []byte
	Boundary    ChunkBoundary
	ContentType string
	Metadata    map[string]any
}

// ScanResult represents Stage 3: Scanning result per chunk
type ScanResult struct {
	SuspicionScore float64
	Flags          []string // e.g., "unicode_tag", "role_switch", "base64_payload"
	ScannerNames   []string // Which scanners produced results
}

// CandidateEdges represents Stage 4: Enrichment
type CandidateEdges struct {
	ChunkID     knowledge.ChunkID
	CallsTo     []knowledge.ChunkID // call graph edges
	ImportsFrom []knowledge.ChunkID // import relations
	Documents   []knowledge.ChunkID // document links
	References  []string            // external references
}

// IngestDisposition represents Stage 5: Admission decision
type IngestDisposition string

const (
	DispositionCommit     IngestDisposition = "commit"
	DispositionQuarantine IngestDisposition = "quarantine"
	DispositionReject     IngestDisposition = "reject"
)

// DispositionReason explains why a particular disposition was chosen.
type DispositionReason struct {
	Stage       string
	Explanation string
}

// IngestResult represents Stage 6: Commit result
type IngestResult struct {
	Disposition       IngestDisposition
	Reason            DispositionReason
	ChunksCommitted   int
	ChunksQuarantined int
	ChunksRejected    int
	ChunkIDs          []knowledge.ChunkID
	EdgesCreated      int
	EventsEmitted     int
	QuarantinePath    string // path to quarantine directory if applicable
	Error             error
}

// Scanner interface for pluggable content scanners.
type Scanner interface {
	Name() string
	Scan(ctx context.Context, chunk TypedChunk) ScanResult
}

// Pipeline executes the six stages for a single raw ingestion.
type Pipeline struct {
	raw           RawIngestion
	policy        *contextpolicy.ContextPolicyBundle
	evaluator     *contextpolicy.Evaluator
	store         *knowledge.ChunkStore
	scanners      []Scanner
	quarantineDir string
}

// ScannerReport aggregates scan results from multiple scanners.
type ScannerReport struct {
	SuspicionScore float64
	Flags          []string
	Details        map[string]ScanResult
}

// ScanReport summarizes workspace scan results.
type ScanReport struct {
	FilesScanned      int
	FilesIgnored      int
	ChunksCreated     int
	ChunksQuarantined int
	ChunksRejected    int
	Errors            []error
	Duration          time.Duration
}

// EventLog is a minimal event logging interface.
type EventLog interface {
	Emit(eventType string, payload map[string]any)
}

// ContentParser parses raw content into typed ingestion.
type ContentParser interface {
	CanParse(mimeType string, filePath string) bool
	Parse(ctx context.Context, raw RawIngestion) (*TypedIngestion, error)
}
