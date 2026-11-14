package tui

import (
	"context"

	runtimesvc "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
)

// ExecutionPort is the cohesive consumer contract for run submission and
// observation (spec §11.4). The runtime owns every executor goroutine and the
// durable journal; views hold tickets, never executors. Implemented implicitly
// by the runtime's RunService through the existing adapter — no runtime→tui
// import, no per-view executor.
type ExecutionPort interface {
	// SubmitInstruction admits the instruction durably and starts the
	// runtime-owned executor. An exact retry of the same request resolves to
	// the original run. The ticket's Events channel closes after the
	// exactly-once finished event; navigation away never affects the run.
	SubmitInstruction(ctx context.Context, cmd runtimesvc.InstructionCommand) (runtimesvc.RunTicket, error)
	// CancelRun requests a protective stop for a live run. It reports whether
	// the stop reached a live executor; the durable terminal outcome is
	// committed by the executor, never by the caller.
	CancelRun(runID string) (stopDelivered bool, err error)
}

// HistoryPort is the read-only consumer contract over the durable journal.
// Playback performs no execution and acquires no model lease.
type HistoryPort interface {
	// ListRuns pages committed run snapshots, newest first.
	ListRuns(ctx context.Context) ([]runtimesvc.RunSummary, error)
	// ReadRunEvents pages one run's committed events after a cursor.
	ReadRunEvents(ctx context.Context, runID string, after uint64, limit int) ([]runtimesvc.RunEventRecord, error)
	// RunSnapshot returns one run's committed state, ok=false when unknown.
	RunSnapshot(ctx context.Context, runID string) (runtimesvc.RunSummary, bool, error)
}

// compile-time assertions: the runtime adapter provides both ports.
var (
	_ ExecutionPort = (*runtimeAdapter)(nil)
	_ HistoryPort   = (*runtimeAdapter)(nil)
)
