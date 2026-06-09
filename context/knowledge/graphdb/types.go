package graphdb

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// ErrQueryLimitExceeded is returned when a query reaches its configured
// result limit before exhausting the traversal space. Callers MAY use
// partial results alongside this error.
var ErrQueryLimitExceeded = errors.New("graphdb: query limit exceeded")

// ────────────────────────────────────────────────────────────────────
// Maintenance
// ────────────────────────────────────────────────────────────────────

// MaintenanceRequest describes a requested storage maintenance operation.
type MaintenanceRequest struct {
	ValueLogGC     bool
	IntegrityCheck bool
}

// MaintenanceResult reports the outcome of a maintenance request.
type MaintenanceResult struct {
	Backend        string
	Reclaimed      bool
	CheckedRecords int
}

// ────────────────────────────────────────────────────────────────────
// Observability
// ────────────────────────────────────────────────────────────────────

// EventObserver receives structured events emitted by the engine.
// Implementations MUST be safe for concurrent use.
type EventObserver interface {
	Observe(event Event)
}

// Event carries structured telemetry from a single graphdb operation.
type Event struct {
	Kind       string
	Backend    string
	NodeCount  int
	EdgeCount  int
	BatchSize  int
	Duration   time.Duration
	ErrorClass string
}

// Event kinds emitted by the engine and backends.
const (
	EventOpenStart          = "graphdb.open.start"
	EventOpenComplete       = "graphdb.open.complete"
	EventBackendCommit      = "graphdb.backend.commit"
	EventBackendCommitFail  = "graphdb.backend.commit_failed"
	EventMemoryApplyFail    = "graphdb.memory_apply_failed"
	EventTraversalComplete  = "graphdb.traversal.complete"
	EventTraversalCancelled = "graphdb.traversal_cancelled"
	EventBadgerGC           = "graphdb.badger.gc"
	EventMigrationStart     = "graphdb.migration.start"
	EventMigrationProgress  = "graphdb.migration.progress"
	EventMigrationComplete  = "graphdb.migration.complete"
)

// ────────────────────────────────────────────────────────────────────
// Backup
// ────────────────────────────────────────────────────────────────────

// ErrBackupUnsupported is returned by backends that do not support backup.
var ErrBackupUnsupported = errors.New("graphdb: backup not supported by this backend")

// ────────────────────────────────────────────────────────────────────
// Graph types
// ────────────────────────────────────────────────────────────────────

// NodeKind and EdgeKind are opaque typed strings. The engine assigns no meaning
// to their values.
type NodeKind string
type EdgeKind string

// Direction controls traversal direction in queries.
type Direction string

const (
	DirectionOut  Direction = "out"
	DirectionIn   Direction = "in"
	DirectionBoth Direction = "both"
)

type NodeRecord struct {
	ID             string          `json:"id"`
	Kind           NodeKind        `json:"kind"`
	SourceID       string          `json:"source_id,omitempty"`
	StableID       string          `json:"stable_id,omitempty"`
	RevisionRootID string          `json:"revision_root_id,omitempty"`
	RevisionOf     string          `json:"revision_of,omitempty"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
	TaskID         string          `json:"task_id,omitempty"`
	SessionID      string          `json:"session_id,omitempty"`
	TurnID         string          `json:"turn_id,omitempty"`
	StateVersion   uint64          `json:"state_version,omitempty"`
	Labels         []string        `json:"labels,omitempty"`
	Props          json.RawMessage `json:"props,omitempty"`
	CreatedAt      int64           `json:"created_at"`
	UpdatedAt      int64           `json:"updated_at"`
	DeletedAt      int64           `json:"deleted_at,omitempty"`
}

type EdgeRecord struct {
	SourceID       string          `json:"s"`
	TargetID       string          `json:"t"`
	Kind           EdgeKind        `json:"k"`
	StableID       string          `json:"stable_id,omitempty"`
	RevisionRootID string          `json:"revision_root_id,omitempty"`
	RevisionOf     string          `json:"revision_of,omitempty"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
	TaskID         string          `json:"task_id,omitempty"`
	SessionID      string          `json:"session_id,omitempty"`
	TurnID         string          `json:"turn_id,omitempty"`
	StateVersion   uint64          `json:"state_version,omitempty"`
	Weight         float32         `json:"w,omitempty"`
	Props          json.RawMessage `json:"p,omitempty"`
	CreatedAt      int64           `json:"c"`
	DeletedAt      int64           `json:"d,omitempty"`
}

func (e EdgeRecord) IsActive() bool {
	return e.DeletedAt == 0
}

type PathResult struct {
	Source string       `json:"source"`
	Target string       `json:"target"`
	Path   []string     `json:"path"`
	Edges  []EdgeRecord `json:"edges"`
}

type GraphQuery struct {
	RootIDs      []string   `json:"root_ids"`
	EdgeKinds    []EdgeKind `json:"edge_kinds"`
	NodeKinds    []NodeKind `json:"node_kinds,omitempty"`
	Direction    Direction  `json:"direction"`
	MaxDepth     int        `json:"max_depth"`
	Limit        int        `json:"limit"`
	MaxEdges     int        `json:"max_edges"`
	Cursor       string     `json:"cursor,omitempty"`
	IncludeProps bool       `json:"include_props,omitempty"`
}

const (
	defaultMaxEdgesMultiplier = 4
	defaultMaxEdgesCap        = 64000
)

// defaultMaxEdges returns a reasonable MaxEdges for the given Limit.
func defaultMaxEdges(limit int) int {
	if limit <= 0 {
		return defaultMaxEdgesCap
	}
	edges := limit * defaultMaxEdgesMultiplier
	if edges > defaultMaxEdgesCap {
		return defaultMaxEdgesCap
	}
	return edges
}

type ImpactResult struct {
	OriginIDs []string         `json:"origin_ids"`
	Affected  []string         `json:"affected"`
	ByDepth   map[int][]string `json:"by_depth"`
}

// MutationScope identifies the kind of graph mutation summarized by a result.
type MutationScope string

const (
	MutationScopeNode       MutationScope = "node"
	MutationScopeEdge       MutationScope = "edge"
	MutationScopeProjection MutationScope = "projection"
)

// MutationStatus captures the overall outcome of a mutation pass.
type MutationStatus string

const (
	MutationStatusCreated    MutationStatus = "created"
	MutationStatusUpdated    MutationStatus = "updated"
	MutationStatusAnnotated  MutationStatus = "annotated"
	MutationStatusSuperseded MutationStatus = "superseded"
	MutationStatusMatched    MutationStatus = "matched"
	MutationStatusRejected   MutationStatus = "rejected"
	MutationStatusConflict   MutationStatus = "conflict"
	MutationStatusNoop       MutationStatus = "noop"
)

// MutationResult is the GraphDB-level audit/result record for a mutation pass.
// Euclo treats it as the projection-level outcome wrapper.
type MutationResult struct {
	StableID       string         `json:"stable_id,omitempty"`
	Scope          MutationScope  `json:"scope"`
	Status         MutationStatus `json:"status"`
	Reason         string         `json:"reason,omitempty"`
	RecordIDs      []string       `json:"record_ids,omitempty"`
	CreatedIDs     []string       `json:"created_ids,omitempty"`
	UpdatedIDs     []string       `json:"updated_ids,omitempty"`
	AnnotatedIDs   []string       `json:"annotated_ids,omitempty"`
	SupersededIDs  []string       `json:"superseded_ids,omitempty"`
	MatchedIDs     []string       `json:"matched_ids,omitempty"`
	RejectedIDs    []string       `json:"rejected_ids,omitempty"`
	ConflictIDs    []string       `json:"conflict_ids,omitempty"`
	TaskID         string         `json:"task_id,omitempty"`
	SessionID      string         `json:"session_id,omitempty"`
	TurnID         string         `json:"turn_id,omitempty"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
	StateVersion   uint64         `json:"state_version,omitempty"`
	AppliedAt      time.Time      `json:"applied_at,omitempty"`
	Details        map[string]any `json:"details,omitempty"`
}

// Normalize trims fields and ensures the result has a stable identity.
func (r *MutationResult) Normalize(taskID, sessionID string) {
	if r == nil {
		return
	}
	r.StableID = strings.TrimSpace(r.StableID)
	r.Reason = strings.TrimSpace(r.Reason)
	r.TaskID = strings.TrimSpace(r.TaskID)
	r.SessionID = strings.TrimSpace(r.SessionID)
	r.TurnID = strings.TrimSpace(r.TurnID)
	r.IdempotencyKey = strings.TrimSpace(r.IdempotencyKey)
	if r.StableID == "" {
		r.StableID = StableMutationID(
			taskID,
			sessionID,
			string(r.Scope),
			string(r.Status),
			r.TaskID,
			r.SessionID,
			r.TurnID,
			r.IdempotencyKey,
			r.Reason,
		)
	}
	r.AppliedAt = r.AppliedAt.UTC()
}

// StableMutationID returns a deterministic mutation identifier for the given parts.
func StableMutationID(parts ...string) string {
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	sum := sha256.Sum256([]byte(strings.Join(cleaned, "\x1f")))
	return hex.EncodeToString(sum[:])
}

// StableNodeID computes a stable node identity from logical mutation inputs.
func StableNodeID(parts ...string) string {
	return StableMutationID(parts...)
}

// StableEdgeID computes a stable edge identity from logical mutation inputs.
func StableEdgeID(parts ...string) string {
	return StableMutationID(parts...)
}
