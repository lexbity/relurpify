package runtime

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	"codeburg.org/lexbit/relurpify/execution/agentlifecycle"
)

func newTestJournal(t *testing.T) (*RunJournal, string) {
	t.Helper()
	dir := t.TempDir()
	ctx := context.Background()
	db, err := graphdb.Open(ctx, graphdb.DefaultOptions(filepath.Join(dir, "state")))
	if err != nil {
		t.Fatalf("open engine: %v", err)
	}
	t.Cleanup(func() { _ = db.Close(context.Background()) })
	j, err := NewRunJournal(db, "ws-test")
	if err != nil {
		t.Fatalf("journal: %v", err)
	}
	return j, dir
}

func requestIDForTest(t *testing.T) RequestID {
	t.Helper()
	return generateRequestID()
}

func TestV2JournalTransitionMachineAndCursor(t *testing.T) {
	j, _ := newTestJournal(t)
	ctx := context.Background()

	snap, err := j.AdmitRun(ctx, requestIDForTest(t), "digest-a", "do the thing")
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	if snap.State != agentlifecycle.RunQueued || snap.Cursor != 0 {
		t.Fatalf("unexpected initial snapshot: %+v", snap)
	}

	// Legal start.
	snap2, reapplied, err := j.Transition(ctx, agentlifecycle.RunTransition{
		RunID: snap.RunID, CauseID: "cause-1", From: agentlifecycle.RunQueued, To: agentlifecycle.RunRunning,
	})
	if err != nil || reapplied {
		t.Fatalf("start: %v reapplied=%v", err, reapplied)
	}
	if snap2.Cursor != 1 || snap2.State != agentlifecycle.RunRunning {
		t.Fatalf("unexpected started snapshot: %+v", snap2)
	}

	// A repeated cause is a read of the recorded result, never a second write.
	snap3, reapplied, err := j.Transition(ctx, agentlifecycle.RunTransition{
		RunID: snap.RunID, CauseID: "cause-1", From: agentlifecycle.RunQueued, To: agentlifecycle.RunRunning,
	})
	if err != nil || !reapplied {
		t.Fatalf("reapply: %v reapplied=%v", err, reapplied)
	}
	if snap3.Cursor != 1 {
		t.Fatalf("reapplied cause must not advance the cursor: %+v", snap3)
	}

	// An illegal edge is rejected before any write.
	if _, _, err := j.Transition(ctx, agentlifecycle.RunTransition{
		RunID: snap.RunID, CauseID: "cause-2", From: agentlifecycle.RunRunning, To: agentlifecycle.RunQueued,
	}); !errors.Is(err, agentlifecycle.ErrIllegalTransition) {
		t.Fatalf("expected illegal transition, got %v", err)
	}

	// A stale from-state is a stale cursor, not a lost update.
	if _, _, err := j.Transition(ctx, agentlifecycle.RunTransition{
		RunID: snap.RunID, CauseID: "cause-3", From: agentlifecycle.RunQueued, To: agentlifecycle.RunCompleted,
	}); !errors.Is(err, agentlifecycle.ErrStaleCursor) {
		t.Fatalf("expected stale cursor, got %v", err)
	}

	// Terminal commit.
	if _, reapplied, err := j.Transition(ctx, agentlifecycle.RunTransition{
		RunID: snap.RunID, CauseID: "cause-4", From: agentlifecycle.RunRunning, To: agentlifecycle.RunCompleted, Reason: "done",
	}); err != nil || reapplied {
		t.Fatalf("complete: %v", err)
	}
	// Terminal states are closed forever.
	if _, _, err := j.Transition(ctx, agentlifecycle.RunTransition{
		RunID: snap.RunID, CauseID: "cause-5", From: agentlifecycle.RunCompleted, To: agentlifecycle.RunFailed,
	}); err == nil {
		t.Fatal("terminal state accepted a new transition")
	}

	events, err := j.ReadEvents(ctx, snap.RunID, 0, 100)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	if len(events) != 2 || events[0].CauseID != "cause-1" || events[1].CauseID != "cause-4" {
		t.Fatalf("unexpected event page: %+v", events)
	}
}

func TestV2JournalAdmissionRetryAndConflict(t *testing.T) {
	j, _ := newTestJournal(t)
	ctx := context.Background()
	id := requestIDForTest(t)

	first, err := j.AdmitRun(ctx, id, "digest-a", "one")
	if err != nil {
		t.Fatalf("first admit: %v", err)
	}
	retry, err := j.AdmitRun(ctx, id, "digest-a", "one")
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if retry.RunID != first.RunID {
		t.Fatalf("exact retry minted a new run: %s vs %s", retry.RunID, first.RunID)
	}
	if _, err := j.AdmitRun(ctx, id, "digest-b", "different"); !errors.Is(err, ErrReceiptConflict) {
		t.Fatalf("expected receipt conflict, got %v", err)
	}
	// The conflicting payload did not overwrite the committed run.
	loaded, ok, err := j.Load(ctx, first.RunID)
	if err != nil || !ok || loaded.RunID != first.RunID {
		t.Fatalf("conflict damaged the record: %v ok=%v", err, ok)
	}
}

func TestV2JournalSurvivesReopenAndReconciles(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "state")
	ctx := context.Background()

	db, err := graphdb.Open(ctx, graphdb.DefaultOptions(dbPath))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	j, err := NewRunJournal(db, "ws-test")
	if err != nil {
		t.Fatalf("journal: %v", err)
	}
	snap, err := j.AdmitRun(ctx, requestIDForTest(t), "digest-a", "survive me")
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	if _, _, err := j.Transition(ctx, agentlifecycle.RunTransition{
		RunID: snap.RunID, CauseID: "c1", From: agentlifecycle.RunQueued, To: agentlifecycle.RunRunning,
	}); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := db.Close(ctx); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Reopen: the crash interrupted the running run; reconciliation marks it
	// interrupted and the history is intact.
	db2, err := graphdb.Open(ctx, graphdb.DefaultOptions(dbPath))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db2.Close(ctx)
	j2, err := NewRunJournal(db2, "ws-test")
	if err != nil {
		t.Fatalf("journal reopen: %v", err)
	}
	open, err := j2.OpenRuns(ctx, "ws-test")
	if err != nil || len(open) != 1 || open[0] != snap.RunID {
		t.Fatalf("open runs after reopen: %v %v", open, err)
	}
	reconciled, err := j2.ReconcileAbandoned(ctx, "ws-test")
	if err != nil || len(reconciled) != 1 {
		t.Fatalf("reconcile: %v %v", reconciled, err)
	}
	loaded, ok, err := j2.Load(ctx, snap.RunID)
	if err != nil || !ok || loaded.State != agentlifecycle.RunInterrupted {
		t.Fatalf("reconciled state: %+v ok=%v err=%v", loaded, ok, err)
	}
	events, err := j2.ReadEvents(ctx, snap.RunID, 0, 100)
	if err != nil || len(events) != 2 {
		t.Fatalf("events after reopen: %d %v", len(events), err)
	}
	if events[1].To != agentlifecycle.RunInterrupted {
		t.Fatalf("expected interrupted event, got %+v", events[1])
	}
}

func TestV2JournalRetentionSweep(t *testing.T) {
	j, _ := newTestJournal(t)
	ctx := context.Background()

	oldSnap, err := j.AdmitRun(ctx, requestIDForTest(t), "d-old", "old work")
	if err != nil {
		t.Fatalf("admit old: %v", err)
	}
	if _, _, err := j.Transition(ctx, agentlifecycle.RunTransition{
		RunID: oldSnap.RunID, CauseID: "c1", From: agentlifecycle.RunQueued, To: agentlifecycle.RunRunning,
	}); err != nil {
		t.Fatalf("start old: %v", err)
	}
	// The committed completion time is 60 days ago (the transition carries its
	// commit timestamp), so the retention window applies.
	if _, _, err := j.Transition(ctx, agentlifecycle.RunTransition{
		RunID: oldSnap.RunID, CauseID: "c2", From: agentlifecycle.RunRunning, To: agentlifecycle.RunCompleted,
		CommittedAt: time.Now().AddDate(0, 0, -60),
	}); err != nil {
		t.Fatalf("complete old: %v", err)
	}

	freshSnap, err := j.AdmitRun(ctx, requestIDForTest(t), "d-new", "new work")
	if err != nil {
		t.Fatalf("admit new: %v", err)
	}

	deleted, err := j.SweepExpired(ctx, time.Now().AddDate(0, 0, -30), 100)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if len(deleted) != 1 || deleted[0] != oldSnap.RunID {
		t.Fatalf("sweep deleted %v, want [%s]", deleted, oldSnap.RunID)
	}
	if _, ok, _ := j.Load(ctx, oldSnap.RunID); ok {
		t.Fatal("expired run survived the sweep")
	}
	if _, ok, _ := j.Load(ctx, freshSnap.RunID); !ok {
		t.Fatal("fresh run was evicted by the sweep")
	}
}
