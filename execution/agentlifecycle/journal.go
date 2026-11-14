package agentlifecycle

import (
	"context"
	"time"
)

// RunCursor is the per-run monotonic journal position. It increments by one
// per committed transition and never skips: consumers resnapshot on gaps.
type RunCursor uint64

// RunTransition is one validated journal write: it moves a run to To, commits
// the event at cursor Expected+1, and is idempotent in CauseID.
type RunTransition struct {
	RunID    string
	CauseID  string        // client-generated identity of the causing fact; repeats return the recorded result
	From     RunState      // expected current state
	To       RunState      // target state
	Reason   string        // bounded human-readable terminal/pause reason
	Metadata map[string]string

	ExpectedCursor RunCursor // stale writes are rejected with ErrStaleCursor
	CommittedAt    time.Time
}

// RunEvent is the committed record of one transition, read back for history.
type RunEvent struct {
	RunID     string
	Cursor    RunCursor
	CauseID   string
	From      RunState
	To        RunState
	Reason    string
	Metadata  map[string]string
	CreatedAt time.Time
}

// RunSnapshot is the committed run record as the journal currently holds it.
type RunSnapshot struct {
	RunID      string
	WorkflowID string
	State      RunState
	Cursor     RunCursor
	StartedAt  time.Time
	CompletedAt *time.Time
	Reason     string
}

// JournalPort is the narrow persistence contract for the durable run journal.
// Implementations MUST commit transitions atomically (record + event in one
// transaction), validate LegalTransition before durable commit, and treat a
// repeated CauseID as a read of the recorded result. Runtime-side adapters own
// the storage encoding; this port stays consumer-owned.
type JournalPort interface {
	// Transition validates and commits one run transition. A repeated CauseID
	// returns (snapshot, true, nil) without writing. An illegal edge returns
	// ErrIllegalTransition; a mismatched ExpectedCursor returns ErrStaleCursor.
	Transition(ctx context.Context, t RunTransition) (RunSnapshot, bool, error)

	// Load returns the committed snapshot of one run, or (nil, false, nil).
	Load(ctx context.Context, runID string) (RunSnapshot, bool, error)

	// ReadEvents pages committed events after the given cursor in ascending
	// order. limit<=0 means a transport-defined default; hard caps apply.
	ReadEvents(ctx context.Context, runID string, after RunCursor, limit int) ([]RunEvent, error)

	// OpenRuns lists run IDs in a nonterminal state for one workspace, oldest
	// first. Crash recovery consumes this at boot.
	OpenRuns(ctx context.Context, workflowID string) ([]string, error)
}
