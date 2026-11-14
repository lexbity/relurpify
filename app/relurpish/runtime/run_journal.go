package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	"codeburg.org/lexbit/relurpify/execution/agentlifecycle"
)

// RunJournal is the durable, runtime-owned run journal over the shared graph
// engine. Every transition is one validated batch: the validator reads the
// engine's consistent view and approves the batch derived from a pre-read; a
// mismatch between pre-read and consistent view rejects the write as stale.
// A repeated cause ID is a read of the recorded result, never a second write.
// The journal owns no goroutines and no second in-memory authority.
type RunJournal struct {
	db        *graphdb.Engine
	workspace WorkspaceID
	now       func() time.Time
}

// NewRunJournal binds the durable run journal to one workspace namespace.
func NewRunJournal(db *graphdb.Engine, workspace WorkspaceID) (*RunJournal, error) {
	if db == nil || workspace == "" {
		return nil, errors.New("run journal: engine and workspace required")
	}
	return &RunJournal{db: db, workspace: workspace, now: time.Now}, nil
}

type journalRunRecord struct {
	RunID       string                   `json:"run_id"`
	WorkflowID  string                   `json:"workflow_id"`
	State       agentlifecycle.RunState  `json:"state"`
	Cursor      agentlifecycle.RunCursor `json:"cursor"`
	StartedAt   time.Time                `json:"started_at"`
	CompletedAt *time.Time               `json:"completed_at,omitempty"`
	Reason      string                   `json:"reason,omitempty"`
}

type journalEventRecord struct {
	RunID     string                   `json:"run_id"`
	Cursor    agentlifecycle.RunCursor `json:"cursor"`
	CauseID   string                   `json:"cause_id"`
	From      agentlifecycle.RunState  `json:"from"`
	To        agentlifecycle.RunState  `json:"to"`
	Reason    string                   `json:"reason,omitempty"`
	CreatedAt time.Time                `json:"created_at"`
}

func (j *RunJournal) runNodeID(runID string) string {
	return fmt.Sprintf("lcrun:%s:%s", j.workspace, runID)
}

func (j *RunJournal) eventNodeID(runID, causeID string) string {
	return fmt.Sprintf("lcevent:%s:%s:%s", j.workspace, runID, causeID)
}

func (j *RunJournal) encodeRecord(rec journalRunRecord) (json.RawMessage, error) {
	data, err := json.Marshal(rec)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}

// AdmitRun commits the command receipt and the queued run record as one
// transaction. An exact retry of the same request ID returns the original run
// without a second admission; a different digest conflicts; an unknown ID
// outside the five-minute issuance window is expired.
func (j *RunJournal) AdmitRun(ctx context.Context, id RequestID, digest, instruction string) (agentlifecycle.RunSnapshot, error) {
	now := j.now().UTC()
	if id == "" {
		id = generateRequestID()
	}
	issued, ok := extractIssuanceTime(id)
	if !ok {
		return agentlifecycle.RunSnapshot{}, fmt.Errorf("invalid request ID: %s", id)
	}
	runID := string(graphdb.GenerateID("run"))
	receipt := receiptRecord{
		RequestID: id, Digest: digest, Target: "run", AdmittedAt: now,
		ProtectedUntil: now.Add(10 * time.Minute), Status: ReceiptStatusAccepted, OperationID: runID,
	}
	record := journalRunRecord{
		RunID: runID, WorkflowID: string(j.workspace),
		State: agentlifecycle.RunQueued, Cursor: 0, StartedAt: now,
	}
	_ = instruction // correlation only; the instruction itself never enters the journal

	recData, err := j.encodeRecord(record)
	if err != nil {
		return agentlifecycle.RunSnapshot{}, err
	}
	encReceipt, err := encodeReceipt(receipt)
	if err != nil {
		return agentlifecycle.RunSnapshot{}, err
	}
	workspace := j.workspace
	batch := graphdb.GraphBatch{Nodes: []graphdb.NodeRecord{
		{ID: NewJournalAdapter(j.db, workspace).receiptKey(id), Kind: "receipt", Props: encReceipt},
		{ID: j.runNodeID(runID), Kind: "lcrun", Labels: []string{string(workspace)}, Props: recData},
	}}
	var retryRun agentlifecycle.RunSnapshot
	var decisionErr error
	err = j.db.ApplyBatchValidated(ctx, batch, func(get graphdb.NodeGetter) error {
		adapter := NewJournalAdapter(j.db, workspace)
		if node, found := get(adapter.receiptKey(id)); found {
			stored, _, decodeErr := decodeReceipt(node.Props)
			if decodeErr != nil || stored.RequestID != id || stored.OperationID == "" {
				decisionErr = ErrJournalUnavailable
				return decisionErr
			}
			if stored.Digest != digest {
				decisionErr = ErrReceiptConflict
				return decisionErr
			}
			if runNode, ok := get(j.runNodeID(stored.OperationID)); ok {
				if snap, derr := decodeRunRecord(runNode.Props); derr == nil {
					retryRun = snap
				}
			}
			return graphdb.ErrBatchConflict // exact retry: read-only
		}
		if now.Sub(issued) > 5*time.Minute || issued.After(now.Add(30*time.Second)) {
			decisionErr = ErrReceiptExpired
			return decisionErr
		}
		return nil
	})
	if retryRun.RunID != "" {
		return retryRun, nil
	}
	if decisionErr != nil {
		return agentlifecycle.RunSnapshot{}, decisionErr
	}
	if err != nil {
		return agentlifecycle.RunSnapshot{}, err
	}
	return snapshotFromRecord(record), nil
}

// Transition implements agentlifecycle.JournalPort. The pre-read derives the
// candidate; the validator re-checks the consistent view against it. A
// mismatch — another writer moved first — is ErrStaleCursor, never a lost
// update. The candidate commit payload is the exact batch validation approved.
func (j *RunJournal) Transition(ctx context.Context, t agentlifecycle.RunTransition) (agentlifecycle.RunSnapshot, bool, error) {
	if t.RunID == "" || t.CauseID == "" {
		return agentlifecycle.RunSnapshot{}, false, errors.New("run journal: run ID and cause ID required")
	}
	if len(t.Reason) > 512 {
		return agentlifecycle.RunSnapshot{}, false, errors.New("run journal: reason exceeds 512 bytes")
	}
	now := j.now().UTC()
	if !t.CommittedAt.IsZero() {
		now = t.CommittedAt
	}

	// A committed cause never writes again: repeat answers are reads. This
	// check precedes state validation so an idempotent replay of an already
	// applied transition returns the recorded result, not a stale error.
	if node, found := j.db.GetNode(j.eventNodeID(t.RunID, t.CauseID)); found {
		var ev journalEventRecord
		if json.Unmarshal(node.Props, &ev) == nil {
			if recNode, ok := j.db.GetNode(j.runNodeID(t.RunID)); ok {
				if snap, derr := decodeRunRecord(recNode.Props); derr == nil {
					return snap, true, nil
				}
			}
		}
		return agentlifecycle.RunSnapshot{}, false, fmt.Errorf("run journal: committed cause %s undecodable", t.CauseID)
	}
	node, ok := j.db.GetNode(j.runNodeID(t.RunID))
	if !ok {
		return agentlifecycle.RunSnapshot{}, false, fmt.Errorf("run journal: run %s not found", t.RunID)
	}
	current, err := decodeRunRecord(node.Props)
	if err != nil {
		return agentlifecycle.RunSnapshot{}, false, err
	}
	// Fast pre-validation keeps malformed calls off the engine lock.
	if current.WorkflowID != string(j.workspace) {
		return agentlifecycle.RunSnapshot{}, false, fmt.Errorf("run journal: run %s belongs to another workspace", t.RunID)
	}
	if current.State.Terminal() {
		return agentlifecycle.RunSnapshot{}, false, fmt.Errorf("run journal: run %s already %s", t.RunID, current.State)
	}
	if current.State != t.From {
		return agentlifecycle.RunSnapshot{}, false, fmt.Errorf("%w: run %s is %s, expected %s", agentlifecycle.ErrStaleCursor, t.RunID, current.State, t.From)
	}
	if !agentlifecycle.LegalTransition(t.From, t.To) {
		return agentlifecycle.RunSnapshot{}, false, fmt.Errorf("%w: %s→%s", agentlifecycle.ErrIllegalTransition, t.From, t.To)
	}
	if t.ExpectedCursor != 0 && t.ExpectedCursor != current.Cursor {
		return agentlifecycle.RunSnapshot{}, false, fmt.Errorf("%w: run %s at %d, expected %d", agentlifecycle.ErrStaleCursor, t.RunID, current.Cursor, t.ExpectedCursor)
	}

	next := journalRunRecord{
		RunID: current.RunID, WorkflowID: current.WorkflowID,
		State: t.To, Cursor: current.Cursor + 1, StartedAt: current.StartedAt,
		CompletedAt: current.CompletedAt, Reason: t.Reason,
	}
	if t.To.Terminal() {
		completed := now
		next.CompletedAt = &completed
	}
	recData, err := j.encodeRecord(next)
	if err != nil {
		return agentlifecycle.RunSnapshot{}, false, err
	}
	event := journalEventRecord{RunID: t.RunID, Cursor: next.Cursor, CauseID: t.CauseID, From: t.From, To: t.To, Reason: t.Reason, CreatedAt: now}
	evData, err := json.Marshal(event)
	if err != nil {
		return agentlifecycle.RunSnapshot{}, false, err
	}
	batch := graphdb.GraphBatch{Nodes: []graphdb.NodeRecord{
		{ID: j.runNodeID(t.RunID), Kind: "lcrun", Labels: []string{current.WorkflowID}, Props: recData},
		{ID: j.eventNodeID(t.RunID, t.CauseID), Kind: "lcevent", Labels: []string{current.WorkflowID}, Props: evData},
	}}
	var decisionErr error
	var reapplied agentlifecycle.RunSnapshot
	err = j.db.ApplyBatchValidated(ctx, batch, func(get graphdb.NodeGetter) error {
		// A committed cause never writes again.
		if _, found := get(j.eventNodeID(t.RunID, t.CauseID)); found {
			if recNode, ok := get(j.runNodeID(t.RunID)); ok {
				if snap, derr := decodeRunRecord(recNode.Props); derr == nil {
					reapplied = snap
				}
			}
			return graphdb.ErrBatchConflict
		}
		curNode, ok := get(j.runNodeID(t.RunID))
		if !ok {
			decisionErr = fmt.Errorf("run journal: run %s not found", t.RunID)
			return decisionErr
		}
		consistent, derr := decodeRunRecord(curNode.Props)
		if derr != nil {
			decisionErr = derr
			return decisionErr
		}
		// The pre-read must still hold; otherwise another writer moved first.
		if consistent.Cursor != current.Cursor || consistent.State != current.State {
			decisionErr = fmt.Errorf("%w: run %s moved to %s@%d during validation", agentlifecycle.ErrStaleCursor, t.RunID, consistent.State, consistent.Cursor)
			return decisionErr
		}
		return nil
	})
	if reapplied.RunID != "" {
		return reapplied, true, nil
	}
	if decisionErr != nil {
		return agentlifecycle.RunSnapshot{}, false, decisionErr
	}
	if err != nil {
		return agentlifecycle.RunSnapshot{}, false, err
	}
	return snapshotFromRecord(next), false, nil
}

// Load implements agentlifecycle.JournalPort.
func (j *RunJournal) Load(_ context.Context, runID string) (agentlifecycle.RunSnapshot, bool, error) {
	node, ok := j.db.GetNode(j.runNodeID(runID))
	if !ok {
		return agentlifecycle.RunSnapshot{}, false, nil
	}
	snap, err := decodeRunRecord(node.Props)
	if err != nil {
		return agentlifecycle.RunSnapshot{}, false, err
	}
	if snap.Reason == "retention-evicted" {
		return agentlifecycle.RunSnapshot{}, false, nil
	}
	return snap, true, nil
}

// ReadEvents implements agentlifecycle.JournalPort: ascending pages after the
// given cursor, bounded to the §8.4 query page cap.
func (j *RunJournal) ReadEvents(_ context.Context, runID string, after agentlifecycle.RunCursor, limit int) ([]agentlifecycle.RunEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	nodes := j.db.ListNodesByLabelPrefix("lcevent", string(j.workspace))
	events := make([]agentlifecycle.RunEvent, 0, len(nodes))
	for _, node := range nodes {
		var rec journalEventRecord
		if json.Unmarshal(node.Props, &rec) != nil || rec.RunID != runID {
			continue
		}
		if rec.Cursor <= after {
			continue
		}
		events = append(events, agentlifecycle.RunEvent{
			RunID: rec.RunID, Cursor: rec.Cursor, CauseID: rec.CauseID,
			From: rec.From, To: rec.To, Reason: rec.Reason, CreatedAt: rec.CreatedAt,
		})
	}
	sort.Slice(events, func(a, b int) bool { return events[a].Cursor < events[b].Cursor })
	if len(events) > limit {
		events = events[:limit]
	}
	return events, nil
}

// OpenRuns implements agentlifecycle.JournalPort: nonterminal run IDs for the
// workflow (workspace), oldest first.
func (j *RunJournal) OpenRuns(_ context.Context, workflowID string) ([]string, error) {
	nodes := j.db.ListNodesByLabel("lcrun", workflowID)
	runs := make([]agentlifecycle.RunSnapshot, 0, len(nodes))
	for _, node := range nodes {
		rec, err := decodeRunRecord(node.Props)
		if err != nil {
			continue
		}
		if !rec.State.Terminal() {
			runs = append(runs, rec)
		}
	}
	sort.Slice(runs, func(a, b int) bool { return runs[a].StartedAt.Before(runs[b].StartedAt) })
	ids := make([]string, 0, len(runs))
	for _, rec := range runs {
		ids = append(ids, rec.RunID)
	}
	return ids, nil
}

// ListRuns returns every non-evicted run snapshot for the workspace, newest
// first. History consumers page through it.
func (j *RunJournal) ListRuns(_ context.Context) ([]agentlifecycle.RunSnapshot, error) {
	nodes := j.db.ListNodes("lcrun")
	runs := make([]agentlifecycle.RunSnapshot, 0, len(nodes))
	for _, node := range nodes {
		rec, err := decodeRunRecord(node.Props)
		if err != nil || rec.WorkflowID != string(j.workspace) {
			continue
		}
		if rec.Reason == "retention-evicted" {
			continue
		}
		runs = append(runs, rec)
	}
	sort.Slice(runs, func(a, b int) bool { return runs[a].StartedAt.After(runs[b].StartedAt) })
	return runs, nil
}

// ReconcileAbandoned marks every nonterminal run interrupted. It runs before
// normal boot reads: a crash after admission creates interrupted runs, never
// automatic replay. It returns the reconciled run IDs.
func (j *RunJournal) ReconcileAbandoned(ctx context.Context, workflowID string) ([]string, error) {
	ids, err := j.OpenRuns(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	reconciled := make([]string, 0, len(ids))
	for _, runID := range ids {
		snap, ok, err := j.Load(ctx, runID)
		if err != nil || !ok {
			continue
		}
		cause := string(graphdb.GenerateID("cause"))
		_, _, err = j.Transition(ctx, agentlifecycle.RunTransition{
			RunID: runID, CauseID: cause,
			From: snap.State, To: agentlifecycle.RunInterrupted,
			Reason: "reconciled after restart: run was " + string(snap.State),
		})
		if err == nil {
			reconciled = append(reconciled, runID)
		}
	}
	return reconciled, nil
}

// SweepExpired tombstones terminal runs completed before the cutoff, oldest
// first, in a bounded batch. Open runs are never touched. It returns the
// evicted run IDs.
func (j *RunJournal) SweepExpired(ctx context.Context, cutoff time.Time, maxRuns int) ([]string, error) {
	if maxRuns <= 0 {
		maxRuns = 100
	}
	nodes := j.db.ListNodes("lcrun")
	type expired struct {
		runID  string
		doneAt time.Time
	}
	candidates := make([]expired, 0, len(nodes))
	for _, node := range nodes {
		rec, err := decodeRunRecord(node.Props)
		if err != nil || rec.WorkflowID != string(j.workspace) || rec.Reason == "retention-evicted" {
			continue
		}
		if !rec.State.Terminal() {
			continue
		}
		done := rec.StartedAt
		if rec.CompletedAt != nil {
			done = *rec.CompletedAt
		}
		if done.Before(cutoff) {
			candidates = append(candidates, expired{runID: rec.RunID, doneAt: done})
		}
	}
	sort.Slice(candidates, func(a, b int) bool { return candidates[a].doneAt.Before(candidates[b].doneAt) })
	if len(candidates) > maxRuns {
		candidates = candidates[:maxRuns]
	}
	deleted := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if err := j.DeleteRun(ctx, c.runID); err == nil {
			deleted = append(deleted, c.runID)
		}
	}
	return deleted, nil
}

// DeleteRun tombstones one terminal run's record so history consumers see an
// evicted marker instead of silently missing data.
func (j *RunJournal) DeleteRun(ctx context.Context, runID string) error {
	node, ok := j.db.GetNode(j.runNodeID(runID))
	if !ok {
		return nil
	}
	current, err := decodeRunRecord(node.Props)
	if err != nil {
		return err
	}
	if !current.State.Terminal() {
		return fmt.Errorf("run journal: refusal to evict open run %s", runID)
	}
	if current.Reason == "retention-evicted" {
		return nil
	}
	tombstone := journalRunRecord{
		RunID: current.RunID, WorkflowID: current.WorkflowID, State: current.State,
		Cursor: current.Cursor, StartedAt: current.StartedAt, CompletedAt: current.CompletedAt,
		Reason: "retention-evicted",
	}
	data, err := j.encodeRecord(tombstone)
	if err != nil {
		return err
	}
	return j.db.ApplyBatchValidated(ctx, graphdb.GraphBatch{Nodes: []graphdb.NodeRecord{{
		ID: j.runNodeID(runID), Kind: "lcrun", Labels: []string{current.WorkflowID, "evicted"}, Props: data,
	}}}, func(get graphdb.NodeGetter) error {
		cur, ok := get(j.runNodeID(runID))
		if !ok {
			return graphdb.ErrBatchConflict
		}
		previous, err := decodeRunRecord(cur.Props)
		if err != nil || !previous.State.Terminal() {
			return graphdb.ErrBatchConflict
		}
		return nil
	})
}

func decodeRunRecord(props json.RawMessage) (agentlifecycle.RunSnapshot, error) {
	var rec journalRunRecord
	if err := json.Unmarshal(props, &rec); err != nil {
		return agentlifecycle.RunSnapshot{}, err
	}
	return snapshotFromRecord(rec), nil
}

func snapshotFromRecord(rec journalRunRecord) agentlifecycle.RunSnapshot {
	return agentlifecycle.RunSnapshot{
		RunID: rec.RunID, WorkflowID: rec.WorkflowID, State: rec.State,
		Cursor: rec.Cursor, StartedAt: rec.StartedAt, CompletedAt: rec.CompletedAt, Reason: rec.Reason,
	}
}
