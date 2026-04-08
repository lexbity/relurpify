package db

import (
	"context"
	"time"
)

// LineageBindingRecord stores the first-class rex FMP lineage binding state.
type LineageBindingRecord struct {
	WorkflowID string
	RunID      string
	LineageID  string
	AttemptID  string
	RuntimeID  string
	SessionID  string
	State      string
	UpdatedAt  time.Time
}

// LineageBindingStore exposes indexed lookup for rex FMP lineage bindings.
type LineageBindingStore interface {
	UpsertLineageBinding(ctx context.Context, record LineageBindingRecord) error
	GetLineageBinding(ctx context.Context, workflowID, runID string) (*LineageBindingRecord, bool, error)
	FindLineageBindingsByLineageID(ctx context.Context, lineageID string) ([]LineageBindingRecord, error)
	FindLineageBindingsByAttemptID(ctx context.Context, attemptID string) ([]LineageBindingRecord, error)
}
