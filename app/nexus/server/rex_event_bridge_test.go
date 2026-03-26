package server

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	nexusdb "github.com/lexcodex/relurpify/app/nexus/db"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	memdb "github.com/lexcodex/relurpify/framework/memory/db"
	rexpkg "github.com/lexcodex/relurpify/named/rex"
	rexcontrolplane "github.com/lexcodex/relurpify/named/rex/controlplane"
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

func TestRexEventBridgeAdmissionRejectsOnceAndAudits(t *testing.T) {
	t.Parallel()

	eventLog, err := nexusdb.NewSQLiteEventLog(filepath.Join(t.TempDir(), "events.db"))
	require.NoError(t, err)
	defer eventLog.Close()

	workflowStore, err := memdb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer workflowStore.Close()

	controller := &countingAdmissionController{
		decision: rexcontrolplane.AdmissionDecision{Allowed: false, Reason: "over_capacity"},
	}
	audit := &rexcontrolplane.AuditLog{}
	bridge := &RexEventBridge{
		Log:            eventLog,
		Partition:      "local",
		Cursor:         newSQLiteRexEventCursorStore(workflowStore.DB(), "local", "test-admission"),
		Gateway:        fakeRexEventGateway{},
		Admission:      controller,
		AdmissionAudit: audit,
		Handle: func(_ context.Context, _ rexgateway.Decision, _ rexevents.CanonicalEvent) error {
			t.Fatal("handle should not be called for rejected admission")
			return nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, bridge.Start(ctx))

	appendRexFrameworkEvent(t, eventLog, core.FrameworkEvent{
		Type:      rexevents.TypeTaskRequested,
		Actor:     core.EventActor{Kind: "service", ID: "test", TenantID: "tenant-a"},
		Partition: "local",
		Payload: mustJSON(t, map[string]any{
			"task_id":        "task-1",
			"instruction":    "run rex task",
			"workload_class": string(rexcontrolplane.WorkloadImportant),
		}),
	})

	require.Eventually(t, func() bool {
		return controller.decideCalls.Load() == 1
	}, time.Second, 20*time.Millisecond)
	require.Equal(t, int32(0), controller.releaseCalls.Load())
	records := audit.Records()
	require.Len(t, records, 1)
	require.False(t, records[0].Allowed)
	require.Equal(t, "over_capacity", records[0].Reason)

	events, err := eventLog.Read(context.Background(), "local", 0, 10, true)
	require.NoError(t, err)
	found := false
	for _, event := range events {
		if event.Type == "rex.admission.rejected.v1" {
			found = true
			break
		}
	}
	require.True(t, found)
}

func TestHandleEventDecisionReleasesAdmissionAfterExecution(t *testing.T) {
	t.Parallel()

	workflowStore, err := memdb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer workflowStore.Close()

	runtimeStore, err := memdb.NewSQLiteRuntimeMemoryStore(filepath.Join(t.TempDir(), "runtime.db"))
	require.NoError(t, err)
	defer runtimeStore.Close()

	checkpoints := memdb.NewSQLiteCheckpointStore(workflowStore.DB())
	composite := memory.NewCompositeRuntimeStore(workflowStore, runtimeStore, checkpoints)
	agent := rexpkg.New(agentenv.AgentEnvironment{
		Model:    stubModel{},
		Registry: capability.NewRegistry(),
		Memory:   composite,
		Config:   &core.Config{Name: "rex-test", Model: "stub", MaxIterations: 1},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	agent.Runtime.Start(ctx)

	controller := &countingAdmissionController{
		decision: rexcontrolplane.AdmissionDecision{Allowed: true, Reason: "capacity_available"},
	}
	provider := &RexRuntimeProvider{
		Agent:          agent,
		Admission:      controller,
		AdmissionAudit: &rexcontrolplane.AuditLog{},
	}

	err = provider.handleEventDecision(ctx, rexgateway.Decision{
		Decision:   rexgateway.SignalDecisionStart,
		WorkflowID: "wf-1",
		RunID:      "wf-1:run",
	}, rexevents.CanonicalEvent{
		ID:         "evt-1",
		Type:       rexevents.TypeTaskRequested,
		Partition:  "local",
		TrustClass: rexevents.TrustInternal,
		Payload: map[string]any{
			"task_id":                 "task-1",
			"instruction":             "review the code",
			"workspace":               t.TempDir(),
			"edit_permitted":          false,
			"rex.admission_tenant_id": "tenant-a",
			"rex.workload_class":      string(rexcontrolplane.WorkloadImportant),
		},
	})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return controller.releaseCalls.Load() == 1
	}, 2*time.Second, 20*time.Millisecond)
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

type countingAdmissionController struct {
	decision     rexcontrolplane.AdmissionDecision
	decideCalls  atomic.Int32
	releaseCalls atomic.Int32
}

func (c *countingAdmissionController) Admit(req rexcontrolplane.AdmissionRequest) bool {
	return c.Decide(req).Allowed
}

func (c *countingAdmissionController) Decide(req rexcontrolplane.AdmissionRequest) rexcontrolplane.AdmissionDecision {
	_ = req
	c.decideCalls.Add(1)
	return c.decision
}

func (c *countingAdmissionController) Release(req rexcontrolplane.AdmissionRequest) {
	_ = req
	c.releaseCalls.Add(1)
}

type stubModel struct{}

func (stubModel) Generate(context.Context, string, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: `{"thought":"done","action":"complete","complete":true,"summary":"ok"}`}, nil
}
func (stubModel) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}
func (stubModel) Chat(context.Context, []core.Message, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: "{}"}, nil
}
func (stubModel) ChatWithTools(context.Context, []core.Message, []core.LLMToolSpec, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: `{"thought":"done","action":"complete","complete":true,"summary":"ok"}`}, nil
}
