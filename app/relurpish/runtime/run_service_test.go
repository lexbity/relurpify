package runtime

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	"codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentlifecycle"
)

type runServiceHarness struct {
	svc    *RunService
	journal *RunJournal
	db     *graphdb.Engine
	execMu sync.Mutex
	exec   func(ctx context.Context, instruction string, taskType execution.TaskType, metadata map[string]any, callback func(string)) (*execution.Result, error)
}

func newRunServiceHarness(t *testing.T) *runServiceHarness {
	t.Helper()
	dir := t.TempDir()
	ctx := context.Background()
	db, err := graphdb.Open(ctx, graphdb.DefaultOptions(filepath.Join(dir, "state")))
	if err != nil {
		t.Fatalf("open engine: %v", err)
	}
	t.Cleanup(func() { _ = db.Close(ctx) })
	journal, err := NewRunJournal(db, "ws-test")
	if err != nil {
		t.Fatalf("journal: %v", err)
	}
	h := &runServiceHarness{journal: journal, db: db}
	coord := newRunCoordinator(nil)
	rt := &Runtime{}
	h.svc, err = NewRunService(rt, coord, journal)
	if err != nil {
		t.Fatalf("run service: %v", err)
	}
	h.svc.SetTimeout(5 * time.Second)
	h.svc.exec = func(ctx context.Context, instruction string, taskType execution.TaskType, metadata map[string]any, callback func(string)) (*execution.Result, error) {
		h.execMu.Lock()
		exec := h.exec
		h.execMu.Unlock()
		return exec(ctx, instruction, taskType, metadata, callback)
	}
	return h
}

func drainTicket(t *testing.T, ticket *RunTicket) []RunEvent {
	t.Helper()
	events := make([]RunEvent, 0, 8)
	for ev := range ticket.Events {
		events = append(events, ev)
		if ev.Kind == RunEventFinished {
			return events
		}
	}
	return events
}

func TestV2RunServiceOwnsRunAcrossAbandonedTicket(t *testing.T) {
	h := newRunServiceHarness(t)
	release := make(chan struct{})
	started := make(chan struct{})
	h.execMu.Lock()
	h.exec = func(ctx context.Context, instruction string, _ execution.TaskType, _ map[string]any, callback func(string)) (*execution.Result, error) {
		close(started)
		select {
		case <-release:
			return &execution.Result{NodeID: "node", Success: true}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	h.execMu.Unlock()

	_, ticket, err := h.svc.Submit(context.Background(), InstructionCommand{Instruction: "long running work", TaskType: execution.TaskTypeCodeGeneration})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	<-started
	// The view abandons the ticket entirely: drops every event, never cancels.
	_ = ticket
	// The run must still finish and journal its terminal transition.
	close(release)

	deadline := time.Now().Add(5 * time.Second)
	for {
		snap, ok, err := h.journal.Load(context.Background(), ticket.RunID)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if ok && snap.State.Terminal() {
			if snap.State != agentlifecycle.RunCompleted {
				t.Fatalf("expected completed, got %s (%s)", snap.State, snap.Reason)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("run never reached a terminal state: %+v", snap)
		}
		time.Sleep(10 * time.Millisecond)
	}
	events, err := h.journal.ReadEvents(context.Background(), ticket.RunID, 0, 100)
	if err != nil || len(events) < 2 {
		t.Fatalf("committed events after abandoned ticket: %d %v", len(events), err)
	}
}

func TestV2RunServiceExactRetryResolvesSameRun(t *testing.T) {
	h := newRunServiceHarness(t)
	h.execMu.Lock()
	h.exec = func(ctx context.Context, _ string, _ execution.TaskType, _ map[string]any, _ func(string)) (*execution.Result, error) {
		return &execution.Result{NodeID: "node", Success: true}, nil
	}
	h.execMu.Unlock()

	id := generateRequestID()
	_, first, err := h.svc.Submit(context.Background(), InstructionCommand{Instruction: "same", RequestID: id})
	if err != nil {
		t.Fatalf("first submit: %v", err)
	}
	drainTicket(t, first)

	// Same request ID, same payload: the same committed run replays its state.
	_, second, err := h.svc.Submit(context.Background(), InstructionCommand{Instruction: "same", RequestID: id})
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if second.RunID != first.RunID {
		t.Fatalf("retry minted run %s, want %s", second.RunID, first.RunID)
	}
	events := drainTicket(t, second)
	if len(events) != 1 || events[0].Kind != RunEventFinished {
		t.Fatalf("retry replayed %d events, want exactly one finished", len(events))
	}
	// A different payload on the same request ID is a conflict, never a run.
	if _, _, err := h.svc.Submit(context.Background(), InstructionCommand{Instruction: "different", RequestID: id}); !errors.Is(err, ErrReceiptConflict) {
		t.Fatalf("expected receipt conflict, got %v", err)
	}
}

func TestV2RunServiceCancelRecordsCanceledOutcome(t *testing.T) {
	h := newRunServiceHarness(t)
	started := make(chan struct{})
	block := make(chan struct{})
	h.execMu.Lock()
	h.exec = func(ctx context.Context, _ string, _ execution.TaskType, _ map[string]any, _ func(string)) (*execution.Result, error) {
		close(started)
		<-ctx.Done()
		close(block)
		return nil, ctx.Err()
	}
	h.execMu.Unlock()

	_, ticket, err := h.svc.Submit(context.Background(), InstructionCommand{Instruction: "cancellable work"})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	<-started
	delivered, err := h.svc.CancelRun(ticket.RunID)
	if err != nil || !delivered {
		t.Fatalf("cancel: delivered=%v err=%v", delivered, err)
	}
	events := drainTicket(t, ticket)
	last := events[len(events)-1]
	if last.Kind != RunEventFinished || last.Outcome != RunOutcomeCanceled {
		t.Fatalf("expected canceled finish, got %+v", last)
	}
	snap, ok, err := h.journal.Load(context.Background(), ticket.RunID)
	if err != nil || !ok || snap.State != agentlifecycle.RunCanceled {
		t.Fatalf("journal outcome: %+v ok=%v err=%v", snap, ok, err)
	}
	// A stop for a finished run is not delivered: nothing is live.
	if again, _ := h.svc.CancelRun(ticket.RunID); again {
		t.Fatal("cancel claimed delivery for a finished run")
	}
}

func TestV2RunServiceAwaitingInteractionPausesNotFails(t *testing.T) {
	h := newRunServiceHarness(t)
	h.execMu.Lock()
	h.exec = func(ctx context.Context, _ string, _ execution.TaskType, _ map[string]any, _ func(string)) (*execution.Result, error) {
		return &execution.Result{NodeID: "node", Success: false, Data: execution.ToolResultPayload{FieldsMap: map[string]any{"outcome_category": "awaiting_interaction"}}}, nil
	}
	h.execMu.Unlock()

	_, ticket, err := h.svc.Submit(context.Background(), InstructionCommand{Instruction: "clarify first"})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	events := drainTicket(t, ticket)
	last := events[len(events)-1]
	if last.Kind != RunEventFinished || !last.Awaiting {
		t.Fatalf("expected awaiting finish, got %+v", last)
	}
	snap, ok, _ := h.journal.Load(context.Background(), ticket.RunID)
	if !ok || snap.State != agentlifecycle.RunAwaitingInteraction {
		t.Fatalf("journal pause state: %+v ok=%v", snap, ok)
	}
	if !strings.Contains(string(snap.State), "awaiting") {
		t.Fatalf("unexpected state %s", snap.State)
	}
}
