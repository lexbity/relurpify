package runtime

import (
	"context"
	"time"

	"codeburg.org/lexbit/relurpify/execution/agentlifecycle"
)

// RunSummary is the history-facing projection of one committed run. It carries
// no executor, no repository and no mutable runtime state.
type RunSummary struct {
	RunID       string
	WorkflowID  string
	State       string
	Cursor      uint64
	StartedAt   time.Time
	CompletedAt *time.Time
	Reason      string
	Live        bool
}

// RunEventRecord is the history-facing projection of one committed journal
// event.
type RunEventRecord struct {
	RunID     string
	Cursor    uint64
	CauseID   string
	From      string
	To        string
	Reason    string
	CreatedAt time.Time
}

// HistoryService is the read-only facade over the durable journal: selection,
// cursor replay and run inspection perform no execution and acquire no model
// lease. Open runs are labeled, never resumed implicitly.
type HistoryService struct {
	journal *RunJournal
	runs    *RunService
}

// NewHistoryService binds history to the journal and dispatcher.
func NewHistoryService(journal *RunJournal, runs *RunService) *HistoryService {
	return &HistoryService{journal: journal, runs: runs}
}

// ListRuns returns committed run snapshots, newest first, with a live marker.
func (h *HistoryService) ListRuns(ctx context.Context) ([]RunSummary, error) {
	snapshots, err := h.journal.ListRuns(ctx)
	if err != nil {
		return nil, err
	}
	live := make(map[string]struct{})
	if h.runs != nil {
		for _, id := range h.runs.LiveRuns() {
			live[id] = struct{}{}
		}
	}
	out := make([]RunSummary, 0, len(snapshots))
	for _, snap := range snapshots {
		_, isLive := live[snap.RunID]
		out = append(out, RunSummary{
			RunID: snap.RunID, WorkflowID: snap.WorkflowID, State: string(snap.State),
			Cursor: uint64(snap.Cursor), StartedAt: snap.StartedAt,
			CompletedAt: snap.CompletedAt, Reason: snap.Reason, Live: isLive,
		})
	}
	return out, nil
}

// ReadRunEvents pages one run's committed events after a cursor.
func (h *HistoryService) ReadRunEvents(ctx context.Context, runID string, after uint64, limit int) ([]RunEventRecord, error) {
	events, err := h.journal.ReadEvents(ctx, runID, agentlifecycle.RunCursor(after), limit)
	if err != nil {
		return nil, err
	}
	out := make([]RunEventRecord, 0, len(events))
	for _, ev := range events {
		out = append(out, RunEventRecord{
			RunID: ev.RunID, Cursor: uint64(ev.Cursor), CauseID: ev.CauseID,
			From: string(ev.From), To: string(ev.To), Reason: ev.Reason, CreatedAt: ev.CreatedAt,
		})
	}
	return out, nil
}

// Snapshot returns one run's committed state, or ok=false.
func (h *HistoryService) Snapshot(ctx context.Context, runID string) (RunSummary, bool, error) {
	snap, ok, err := h.journal.Load(ctx, runID)
	if err != nil || !ok {
		return RunSummary{}, false, err
	}
	_, isLive := h.runs.LiveRunsMap()[snap.RunID]
	return RunSummary{
		RunID: snap.RunID, WorkflowID: snap.WorkflowID, State: string(snap.State),
		Cursor: uint64(snap.Cursor), StartedAt: snap.StartedAt,
		CompletedAt: snap.CompletedAt, Reason: snap.Reason, Live: isLive,
	}, true, nil
}

func agentlifecycleRunCursor(v uint64) agentlifecycle.RunCursor { return agentlifecycle.RunCursor(v) }
