package server

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"

	nexusdb "github.com/lexcodex/relurpify/app/nexus/db"
	"github.com/lexcodex/relurpify/framework/core"
	memdb "github.com/lexcodex/relurpify/framework/memory/db"
	rexevents "github.com/lexcodex/relurpify/named/rex/events"
	rexgateway "github.com/lexcodex/relurpify/named/rex/gateway"
	"github.com/stretchr/testify/require"
)

func TestRexEventBridgeDispatchesCanonicalEvents(t *testing.T) {
	t.Parallel()

	eventLog, err := nexusdb.NewSQLiteEventLog(filepath.Join(t.TempDir(), "events.db"))
	require.NoError(t, err)
	defer eventLog.Close()

	workflowStore, err := memdb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer workflowStore.Close()

	var mu sync.Mutex
	var handled []string
	bridge := &RexEventBridge{
		Log:       eventLog,
		Partition: "local",
		Cursor:    newSQLiteRexEventCursorStore(workflowStore.DB(), "local", "test-dispatch"),
		Gateway: fakeRexEventGateway{
			decision: rexgateway.Decision{
				Decision:   rexgateway.SignalDecisionStart,
				WorkflowID: "rexwf:alpha",
				RunID:      "rexwf:alpha:run",
			},
		},
		Handle: func(_ context.Context, _ rexgateway.Decision, event rexevents.CanonicalEvent) error {
			mu.Lock()
			defer mu.Unlock()
			handled = append(handled, event.ID)
			return nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, bridge.Start(ctx))

	appendRexFrameworkEvent(t, eventLog, core.FrameworkEvent{
		Type:      rexevents.TypeTaskRequested,
		Actor:     core.EventActor{Kind: "service", ID: "test"},
		Partition: "local",
		Payload: mustJSON(t, map[string]any{
			"task_id":     "task-1",
			"instruction": "run rex task",
		}),
	})

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(handled) == 1 && handled[0] != ""
	}, time.Second, 20*time.Millisecond)
}

func TestRexEventBridgePersistsCursor(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	eventLog, err := nexusdb.NewSQLiteEventLog(filepath.Join(dir, "events.db"))
	require.NoError(t, err)
	defer eventLog.Close()

	workflowStore, err := memdb.NewSQLiteWorkflowStateStore(filepath.Join(dir, "workflow.db"))
	require.NoError(t, err)
	defer workflowStore.Close()

	cursor := newSQLiteRexEventCursorStore(workflowStore.DB(), "local", "test-cursor")

	firstHandled := make(chan string, 1)
	firstBridge := &RexEventBridge{
		Log:       eventLog,
		Partition: "local",
		Cursor:    cursor,
		Gateway: fakeRexEventGateway{
			decision: rexgateway.Decision{Decision: rexgateway.SignalDecisionStart, WorkflowID: "wf-1", RunID: "wf-1:run"},
		},
		Handle: func(_ context.Context, _ rexgateway.Decision, event rexevents.CanonicalEvent) error {
			firstHandled <- event.ID
			return nil
		},
	}

	ctx1, cancel1 := context.WithCancel(context.Background())
	require.NoError(t, firstBridge.Start(ctx1))
	appendRexFrameworkEvent(t, eventLog, core.FrameworkEvent{
		Type:      rexevents.TypeTaskRequested,
		Actor:     core.EventActor{Kind: "service", ID: "test"},
		Partition: "local",
		Payload:   mustJSON(t, map[string]any{"task_id": "task-1", "instruction": "first"}),
	})
	select {
	case <-firstHandled:
	case <-time.After(time.Second):
		t.Fatal("first event was not handled")
	}
	cancel1()

	secondHandled := make(chan string, 2)
	secondBridge := &RexEventBridge{
		Log:       eventLog,
		Partition: "local",
		Cursor:    cursor,
		Gateway: fakeRexEventGateway{
			decision: rexgateway.Decision{Decision: rexgateway.SignalDecisionStart, WorkflowID: "wf-2", RunID: "wf-2:run"},
		},
		Handle: func(_ context.Context, _ rexgateway.Decision, event rexevents.CanonicalEvent) error {
			secondHandled <- event.ID
			return nil
		},
	}

	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	require.NoError(t, secondBridge.Start(ctx2))
	appendRexFrameworkEvent(t, eventLog, core.FrameworkEvent{
		Type:      rexevents.TypeTaskRequested,
		Actor:     core.EventActor{Kind: "service", ID: "test"},
		Partition: "local",
		Payload:   mustJSON(t, map[string]any{"task_id": "task-2", "instruction": "second"}),
	})

	select {
	case id := <-secondHandled:
		require.Equal(t, "2", id)
	case <-time.After(time.Second):
		t.Fatal("second event was not handled")
	}
	select {
	case id := <-secondHandled:
		t.Fatalf("unexpected replayed event %s", id)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestMapSessionMessageToRex(t *testing.T) {
	t.Parallel()

	canonicalEvent, err := mapSessionMessageToRex(core.FrameworkEvent{
		Seq:       42,
		Type:      core.FrameworkEventSessionMessage,
		Partition: "local",
		Actor:     core.EventActor{Kind: "session", ID: "sess-1"},
		Payload: mustJSON(t, map[string]any{
			"session_key": "sess-1",
			"channel":     "webchat",
			"content":     "build a plan",
		}),
	})
	require.NoError(t, err)
	require.Equal(t, rexevents.TypeTaskRequested, canonicalEvent.Type)
	require.Equal(t, "framework.session", canonicalEvent.Source)
	require.Equal(t, "sess-1", canonicalEvent.ActorID)
	require.Equal(t, "build a plan", canonicalEvent.Payload["instruction"])
	require.Equal(t, "rex-session:sess-1", canonicalEvent.Payload["workflow_id"])
}

type fakeRexEventGateway struct {
	decision rexgateway.Decision
	err      error
}

func (g fakeRexEventGateway) Resolve(context.Context, rexevents.CanonicalEvent) (rexgateway.Decision, error) {
	return g.decision, g.err
}

func appendRexFrameworkEvent(t *testing.T, log *nexusdb.SQLiteEventLog, event core.FrameworkEvent) {
	t.Helper()
	_, err := log.Append(context.Background(), "local", []core.FrameworkEvent{event})
	require.NoError(t, err)
}

func mustJSON(t *testing.T, payload map[string]any) []byte {
	t.Helper()
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	return data
}
