package memory

import (
	"context"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

// AnchorRef mirrors the retrieval.AnchorRef type to avoid import cycles.
// This represents a reference to a semantic anchor found to be stale (drifted or superseded).
type AnchorRef struct {
	AnchorID   string `json:"anchor_id"`
	Term       string `json:"term"`
	Definition string `json:"definition"`
	Class      string `json:"class"`
	Active     bool   `json:"active"`
	CreatedAt  string `json:"created_at"`
}

// DeclarativeMemoryKind classifies durable fact-like records.
type DeclarativeMemoryKind string

const (
	DeclarativeMemoryKindFact             DeclarativeMemoryKind = "fact"
	DeclarativeMemoryKindDecision         DeclarativeMemoryKind = "decision"
	DeclarativeMemoryKindConstraint       DeclarativeMemoryKind = "constraint"
	DeclarativeMemoryKindPreference       DeclarativeMemoryKind = "preference"
	DeclarativeMemoryKindProjectKnowledge DeclarativeMemoryKind = "project-knowledge"
)

// ProceduralMemoryKind classifies durable reusable routines.
type ProceduralMemoryKind string

const (
	ProceduralMemoryKindRoutine               ProceduralMemoryKind = "routine"
	ProceduralMemoryKindCapabilityComposition ProceduralMemoryKind = "capability-composition"
	ProceduralMemoryKindRecoveryRoutine       ProceduralMemoryKind = "recovery-routine"
)

// DeclarativeMemoryRecord stores durable facts, decisions, and constraints.
type DeclarativeMemoryRecord struct {
	RecordID     string
	Scope        MemoryScope
	Kind         DeclarativeMemoryKind
	Title        string
	Content      string
	Summary      string
	WorkflowID   string
	TaskID       string
	ProjectID    string
	ArtifactRef  string
	Tags         []string
	Metadata     map[string]any
	Verified     bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
	StaleAnchors []AnchorRef `json:"-"` // populated on read if bound anchors have drifted/superseded status
}

// ProceduralMemoryRecord stores reusable executable routines and metadata.
type ProceduralMemoryRecord struct {
	RoutineID              string
	Scope                  MemoryScope
	Kind                   ProceduralMemoryKind
	Name                   string
	Description            string
	Summary                string
	WorkflowID             string
	TaskID                 string
	ProjectID              string
	BodyRef                string
	InlineBody             string
	CapabilityDependencies []core.CapabilitySelector
	VerificationMetadata   map[string]any
	PolicySnapshotID       string
	Verified               bool
	Version                int
	ReuseCount             int
	CreatedAt              time.Time
	UpdatedAt              time.Time
	StaleAnchors           []AnchorRef `json:"-"` // populated on read if bound anchors have drifted/superseded status
}

// DeclarativeMemoryQuery supports bounded retrieval of fact-like memory.
type DeclarativeMemoryQuery struct {
	Query      string
	Scope      MemoryScope
	Kinds      []DeclarativeMemoryKind
	TaskID     string
	WorkflowID string
	ProjectID  string
	Limit      int
}

// ProceduralMemoryQuery supports bounded retrieval of routine-like memory.
type ProceduralMemoryQuery struct {
	Query          string
	Scope          MemoryScope
	Kinds          []ProceduralMemoryKind
	TaskID         string
	WorkflowID     string
	ProjectID      string
	CapabilityName string
	Limit          int
}

// DeclarativeMemoryStore persists and retrieves declarative memory records.
type DeclarativeMemoryStore interface {
	PutDeclarative(ctx context.Context, record DeclarativeMemoryRecord) error
	GetDeclarative(ctx context.Context, recordID string) (*DeclarativeMemoryRecord, bool, error)
	SearchDeclarative(ctx context.Context, query DeclarativeMemoryQuery) ([]DeclarativeMemoryRecord, error)
}

// ProceduralMemoryStore persists and retrieves procedural memory records.
type ProceduralMemoryStore interface {
	PutProcedural(ctx context.Context, record ProceduralMemoryRecord) error
	GetProcedural(ctx context.Context, routineID string) (*ProceduralMemoryRecord, bool, error)
	SearchProcedural(ctx context.Context, query ProceduralMemoryQuery) ([]ProceduralMemoryRecord, error)
}

// RuntimeMemoryStore combines declarative and procedural retrieval lanes.
type RuntimeMemoryStore interface {
	DeclarativeMemoryStore
	ProceduralMemoryStore
}
