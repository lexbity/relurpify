package agentlifecycle

import (
	"context"
)

// Observation is one consistent subscription to a run's committed state: an
// initial snapshot plus a cursor channel the runtime adapter coalesces.
type Observation struct {
	Snapshot RunSnapshot
	// Cursors delivers (runID, latest committed cursor) after each commit.
	// Notifications may coalesce; a full channel retains a gap flag and the
	// latest cursor — it never drops the authoritative facts themselves.
	Cursors <-chan CursorNotification
	// Close releases the subscription exactly once. Context cancellation also
	// closes it.
	Close func()
}

// CursorNotification is the coalesced head of a run's journal.
type CursorNotification struct {
	RunID  string
	Cursor RunCursor
	// Gap is set when notifications were coalesced past a slow consumer;
	// readers pull the missing range with ReadEvents.
	Gap bool
}

// ObserveRun opens an Observation for one run. afterCursor < the retained
// floor is an adapter-defined error (CursorExpired) prompting a resnapshot.
type ObserveRun func(ctx context.Context, runID string, afterCursor RunCursor) (Observation, error)
