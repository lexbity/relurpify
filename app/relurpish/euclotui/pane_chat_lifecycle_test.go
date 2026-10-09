package euclotui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.uber.org/goleak"

	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	"codeburg.org/lexbit/relurpify/execution"
)

// newLifecyclePane builds a ChatPane around a controllable SubmitTurn.
// release unblocks the run's work goroutine.
func newLifecyclePane(t *testing.T) (*ChatPane, chan struct{}) {
	t.Helper()
	release := make(chan struct{})
	adapter := &stubRuntimeAdapter{}
	adapter.submitFunc = func(ctx context.Context, _ string, _ execution.TaskType, _ map[string]any, callback func(string)) (*execution.Result, error) {
		if callback != nil {
			callback("tok")
		}
		select {
		case <-release:
			return &execution.Result{NodeID: "node", Success: true}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	pane := NewChatPane(context.Background(), adapter, &tui.AgentContext{}, &tui.Session{}, &tui.NotificationQueue{}, nil, nil, nil)
	return pane, release
}

// readTerminal reads run.ch until the run's RunFinishedMsg arrives.
func readTerminal(t *testing.T, run *chatRun) tui.RunFinishedMsg {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case msg := <-run.ch:
			if finished, ok := msg.(tui.RunFinishedMsg); ok && finished.RunID == run.id {
				return finished
			}
		case <-deadline:
			t.Fatal("terminal RunFinishedMsg not delivered within 2s")
		}
	}
}

// TestChatRunDeliversExactlyOneTerminal: a run driven to completion emits
// exactly one RunFinishedMsg; a second finishRun call is a no-op.
func TestChatRunDeliversExactlyOneTerminal(t *testing.T) {
	defer goleak.VerifyNone(t)
	pane, release := newLifecyclePane(t)
	cmd, runID := pane.StartRunWithMetadata("do work", nil)
	if cmd == nil || runID == "" {
		t.Fatal("StartRunWithMetadata did not start a run")
	}
	run, ok := pane.getRun(runID)
	if !ok {
		t.Fatal("run not registered")
	}
	close(release)
	finished := readTerminal(t, run)
	if finished.Outcome != tui.RunSucceeded {
		t.Fatalf("expected succeeded outcome, got %q", finished.Outcome)
	}
	if finished.Err != nil {
		t.Fatalf("unexpected error: %v", finished.Err)
	}
	// Second finish must not deliver a second terminal.
	pane.finishRun(run, tui.RunFailed, errors.New("late duplicate"), 0)
	select {
	case msg, ok := <-run.ch:
		if ok {
			t.Fatalf("duplicate terminal delivered: %#v", msg)
		}
	case <-time.After(50 * time.Millisecond):
	}
}

// TestChatRunFinishCancelsContext: finishing a run (any outcome) cancels its
// context, releasing the timeout timer and any in-flight work.
func TestChatRunFinishCancelsContext(t *testing.T) {
	defer goleak.VerifyNone(t)
	pane, release := newLifecyclePane(t)
	_, runID := pane.StartRunWithMetadata("work", nil)
	run, _ := pane.getRun(runID)
	close(release)
	readTerminal(t, run)
	if err := run.ctx.Err(); err != context.Canceled {
		t.Fatalf("expected run context cancelled after finish, got %v", err)
	}
	if pane.HasActiveRuns() {
		t.Fatal("finished run must leave the active registry")
	}
}

// TestChatRunErrorOutcomeSurfacesMessage: an errored run renders the error
// through the single terminal handler and retires the run.
func TestChatRunErrorOutcomeSurfacesMessage(t *testing.T) {
	defer goleak.VerifyNone(t)
	boom := errors.New("boom")
	adapter := &stubRuntimeAdapter{}
	adapter.submitFunc = func(ctx context.Context, _ string, _ execution.TaskType, _ map[string]any, _ func(string)) (*execution.Result, error) {
		return nil, boom
	}
	pane := NewChatPane(context.Background(), adapter, &tui.AgentContext{}, &tui.Session{}, &tui.NotificationQueue{}, nil, nil, nil)
	_, runID := pane.StartRunWithMetadata("fail fast", nil)
	run, _ := pane.getRun(runID)
	finished := readTerminal(t, run)
	if finished.Outcome != tui.RunFailed {
		t.Fatalf("expected failed outcome, got %q", finished.Outcome)
	}
	pane2, _ := pane.Update(finished)
	if pane2 == nil {
		t.Fatal("Update returned nil pane")
	}
	found := false
	for _, msg := range pane.Messages() {
		if msg.Role == tui.RoleSystem && strings.Contains(msg.Content.Text, "boom") {
			found = true
		}
	}
	if !found {
		t.Fatal("agent error not surfaced as a system message")
	}
	if pane.HasActiveRuns() {
		t.Fatal("errored run must leave the active registry")
	}
}

// TestChatRunCancelledOutcome: cancelling a run (StopLatestRun) funnels into
// the cancelled terminal outcome.
func TestChatRunCancelledOutcome(t *testing.T) {
	defer goleak.VerifyNone(t)
	pane, _ := newLifecyclePane(t)
	_, runID := pane.StartRunWithMetadata("long work", nil)
	run, _ := pane.getRun(runID)
	pane.StopLatestRun()
	finished := readTerminal(t, run)
	if finished.Outcome != tui.RunCancelled {
		t.Fatalf("expected cancelled outcome, got %q", finished.Outcome)
	}
	if !errors.Is(finished.Err, context.Canceled) {
		t.Fatalf("expected context.Canceled error, got %v", finished.Err)
	}
}

// TestChatRunDroppedUpdatesCounted: intermediate projections dropped under
// backpressure are counted and surfaced in the terminal summary.
func TestChatRunDroppedUpdatesCounted(t *testing.T) {
	defer goleak.VerifyNone(t)
	flooded := make(chan struct{})
	release := make(chan struct{})
	adapter := &stubRuntimeAdapter{}
	adapter.submitFunc = func(ctx context.Context, _ string, _ execution.TaskType, _ map[string]any, callback func(string)) (*execution.Result, error) {
		// Flood far beyond the channel buffer with no consumer reading.
		for i := 0; i < 600; i++ {
			if callback != nil {
				callback("token")
			}
		}
		close(flooded)
		<-release
		return &execution.Result{NodeID: "node", Success: true}, nil
	}
	pane := NewChatPane(context.Background(), adapter, &tui.AgentContext{}, &tui.Session{}, &tui.NotificationQueue{}, nil, nil, nil)
	_, runID := pane.StartRunWithMetadata("flood", nil)
	run, _ := pane.getRun(runID)
	select {
	case <-flooded:
	case <-time.After(2 * time.Second):
		t.Fatal("flood did not run")
	}
	close(release)
	finished := readTerminal(t, run)
	if finished.DroppedUpdates == 0 {
		t.Fatal("expected dropped updates to be counted under backpressure")
	}
	pane.Update(finished)
	found := false
	for _, msg := range pane.Messages() {
		if msg.Role == tui.RoleSystem && strings.Contains(msg.Content.Text, "dropped") {
			found = true
		}
	}
	if !found {
		t.Fatal("dropped-update count not surfaced in the run summary")
	}
}

// TestChatPaneCleanupDrainsLiveRuns: program-exit cleanup with three live
// runs cancels them, drains their terminals, and leaves no goroutines.
func TestChatPaneCleanupDrainsLiveRuns(t *testing.T) {
	defer goleak.VerifyNone(t)
	pane, _ := newLifecyclePane(t)
	pane.SetAllowParallel(true)
	for i := 0; i < 3; i++ {
		cmd, runID := pane.StartRunWithMetadata("work", nil)
		if cmd == nil || runID == "" {
			t.Fatalf("run %d not started", i)
		}
	}
	if !pane.HasActiveRuns() {
		t.Fatal("expected 3 active runs")
	}
	pane.Cleanup()
	if pane.HasActiveRuns() {
		t.Fatal("cleanup must cancel and retire all live runs")
	}
}

// TestChatRunListenerStopsAfterTerminal: the stream listener chain ends with
// the terminal message — after handleRunFinished the pane returns no
// listener re-arm for the finished run's channel.
func TestChatRunListenerStopsAfterTerminal(t *testing.T) {
	defer goleak.VerifyNone(t)
	pane, release := newLifecyclePane(t)
	_, runID := pane.StartRunWithMetadata("work", nil)
	run, _ := pane.getRun(runID)
	close(release)
	finished := readTerminal(t, run)
	_, cmd := pane.Update(finished)
	// Drain any cmd chain: it must terminate (nil) rather than block on the
	// finished run's channel forever.
	for i := 0; i < 8 && cmd != nil; i++ {
		msg := cmd()
		if msg == nil {
			break
		}
	}
}

// TestChatRunThousandRunSession: a simulated thousand-run session leaves no
// goroutine growth and delivers exactly one terminal per run — the pane's
// run kernel is repeatable indefinitely, not just once.
func TestChatRunThousandRunSession(t *testing.T) {
	defer goleak.VerifyNone(t)
	adapter := &stubRuntimeAdapter{}
	adapter.submitFunc = func(ctx context.Context, _ string, _ execution.TaskType, _ map[string]any, callback func(string)) (*execution.Result, error) {
		if callback != nil {
			callback("tok")
		}
		return &execution.Result{NodeID: "node", Success: true}, nil
	}
	pane := NewChatPane(context.Background(), adapter, &tui.AgentContext{}, &tui.Session{}, &tui.NotificationQueue{}, nil, nil, nil)
	for i := 0; i < 1000; i++ {
		_, runID := pane.StartRunWithMetadata("work", nil)
		if runID == "" {
			t.Fatalf("run %d not started", i)
		}
		// An immediate-success run may already have moved to the finished
		// archive by the time we look — both registries count as "born".
		run, ok := pane.getRun(runID)
		if !ok {
			run, ok = pane.peekFinishedRun(runID)
			if !ok {
				t.Fatalf("run %d not registered", i)
			}
			// Already finished: the terminal is in the channel or archive;
			// drive the handler directly off the archived state.
			finished := readTerminal(t, run)
			if finished.Outcome != tui.RunSucceeded {
				t.Fatalf("run %d: unexpected outcome %q", i, finished.Outcome)
			}
			pane.Update(finished)
			continue
		}
		finished := readTerminal(t, run)
		if finished.Outcome != tui.RunSucceeded {
			t.Fatalf("run %d: unexpected outcome %q", i, finished.Outcome)
		}
		if _, cmd := pane.Update(finished); cmd != nil {
			// Drain the returned listener/spinner chain so it cannot linger.
			for j := 0; j < 4 && cmd != nil; j++ {
				if msg := cmd(); msg == nil {
					break
				}
			}
		}
	}
	if pane.HasActiveRuns() {
		t.Fatal("all runs must be retired")
	}
}
