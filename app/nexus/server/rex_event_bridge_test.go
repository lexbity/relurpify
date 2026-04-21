package server

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	nexusdb "codeburg.org/lexbit/relurpify/app/nexus/db"
	"codeburg.org/lexbit/relurpify/ayenitd"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	memdb "codeburg.org/lexbit/relurpify/framework/memory/db"
	rexpkg "codeburg.org/lexbit/relurpify/named/rex"
	rexconfig "codeburg.org/lexbit/relurpify/named/rex/config"
	rexcontrolplane "codeburg.org/lexbit/relurpify/named/rex/controlplane"
	rexevents "codeburg.org/lexbit/relurpify/named/rex/events"
	rexgateway "codeburg.org/lexbit/relurpify/named/rex/gateway"
	rexctx "codeburg.org/lexbit/relurpify/named/rex/rexctx"
	rexruntime "codeburg.org/lexbit/relurpify/named/rex/runtime"
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
	require.Eventually(t, func() bool {
		seq, err := cursor.Load(context.Background())
		require.NoError(t, err)
		return seq == 1
	}, time.Second, 20*time.Millisecond)
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

func TestMapSessionMessageToRexAcceptsStructuredContent(t *testing.T) {
	t.Parallel()

	canonicalEvent, err := mapSessionMessageToRex(core.FrameworkEvent{
		Seq:       7,
		Type:      core.FrameworkEventSessionMessage,
		Partition: "local",
		Actor:     core.EventActor{Kind: "session", ID: "sess-7"},
		Payload: mustJSON(t, map[string]any{
			"session_key": "sess-7",
			"channel":     "webchat",
			"content": map[string]any{
				"text": "summarize the issue",
			},
		}),
	})
	require.NoError(t, err)
	require.Equal(t, "summarize the issue", canonicalEvent.Payload["instruction"])
}

func TestRexEventBridgeRetriesEventAfterHandlerFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	eventLog, err := nexusdb.NewSQLiteEventLog(filepath.Join(dir, "events.db"))
	require.NoError(t, err)
	defer eventLog.Close()

	workflowStore, err := memdb.NewSQLiteWorkflowStateStore(filepath.Join(dir, "workflow.db"))
	require.NoError(t, err)
	defer workflowStore.Close()

	cursor := newSQLiteRexEventCursorStore(workflowStore.DB(), "local", "test-retry")
	firstAttempt := make(chan struct{}, 1)
	var attempts atomic.Int32
	firstHandler := &bridgeHandlerDouble{failuresRemaining: 1, blockUntilContextDone: true}
	firstBridge := &RexEventBridge{
		Log:       eventLog,
		Partition: "local",
		Cursor:    cursor,
		Gateway: fakeRexEventGateway{
			decision: rexgateway.Decision{Decision: rexgateway.SignalDecisionStart, WorkflowID: "wf-retry", RunID: "wf-retry:run"},
		},
		Handle: func(ctx context.Context, _ rexgateway.Decision, event rexevents.CanonicalEvent) error {
			attempts.Add(1)
			select {
			case firstAttempt <- struct{}{}:
			default:
			}
			return firstHandler.Handle(ctx, event)
		},
	}

	ctx1, cancel1 := context.WithCancel(context.Background())
	require.NoError(t, firstBridge.Start(ctx1))
	appendRexFrameworkEvent(t, eventLog, core.FrameworkEvent{
		Type:      rexevents.TypeTaskRequested,
		Actor:     core.EventActor{Kind: "service", ID: "test"},
		Partition: "local",
		Payload:   mustJSON(t, map[string]any{"task_id": "task-retry", "instruction": "retry me"}),
	})

	select {
	case <-firstAttempt:
	case <-time.After(time.Second):
		t.Fatal("first handler attempt did not start")
	}
	seq, err := cursor.Load(context.Background())
	require.NoError(t, err)
	require.Equal(t, uint64(0), seq)
	cancel1()

	secondAttempt := make(chan struct{}, 1)
	secondHandler := &bridgeHandlerDouble{}
	secondBridge := &RexEventBridge{
		Log:       eventLog,
		Partition: "local",
		Cursor:    cursor,
		Gateway: fakeRexEventGateway{
			decision: rexgateway.Decision{Decision: rexgateway.SignalDecisionStart, WorkflowID: "wf-retry", RunID: "wf-retry:run"},
		},
		Handle: func(ctx context.Context, _ rexgateway.Decision, event rexevents.CanonicalEvent) error {
			attempts.Add(1)
			select {
			case secondAttempt <- struct{}{}:
			default:
			}
			return secondHandler.Handle(ctx, event)
		},
	}
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	require.NoError(t, secondBridge.Start(ctx2))

	select {
	case <-secondAttempt:
	case <-time.After(time.Second):
		t.Fatal("second bridge did not replay failed event")
	}
	require.Eventually(t, func() bool {
		seq, err := cursor.Load(context.Background())
		require.NoError(t, err)
		return seq == 1
	}, time.Second, 20*time.Millisecond)
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
		decisions: []rexcontrolplane.AdmissionDecision{{Allowed: false, Reason: "over_capacity"}},
	}
	audit := &rexcontrolplane.AuditLog{}
	clock := newManualClock(time.Date(2026, 4, 2, 18, 0, 0, 0, time.UTC))
	bridge := &RexEventBridge{
		Log:            eventLog,
		Partition:      "local",
		Cursor:         newSQLiteRexEventCursorStore(workflowStore.DB(), "local", "test-admission"),
		Gateway:        fakeRexEventGateway{},
		Now:            clock.Now,
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
		Actor:     core.EventActor{Kind: "service", ID: "test", TenantID: "tenant-a", Scopes: []string{"rex:workload:critical"}},
		Partition: "local",
		Payload: mustJSON(t, map[string]any{
			"task_id":     "task-1",
			"instruction": "run rex task",
		}),
	})

	require.Eventually(t, func() bool {
		return controller.DecideCount() == 1
	}, time.Second, 20*time.Millisecond)
	require.Equal(t, 0, controller.ReleaseCount())
	require.Len(t, controller.decideRequests, 1)
	require.Equal(t, "tenant-a", controller.decideRequests[0].TenantID)
	require.Equal(t, rexcontrolplane.WorkloadCritical, controller.decideRequests[0].Class)
	records := audit.Records()
	require.Len(t, records, 1)
	require.False(t, records[0].Allowed)
	require.Equal(t, "over_capacity", records[0].Reason)
	require.Equal(t, clock.Now(), records[0].Timestamp)

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
	agent := rexpkg.New(ayenitd.WorkspaceEnvironment{
		Model:    stubModel{},
		Registry: capability.NewRegistry(),
		Memory:   composite,
		Config:   &core.Config{Name: "rex-test", Model: "stub", MaxIterations: 1},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	agent.Runtime.Start(ctx)

	controller := &countingAdmissionController{
		decisions: []rexcontrolplane.AdmissionDecision{{Allowed: true, Reason: "capacity_available"}},
	}
	provider := &RexRuntimeProvider{
		Agent:          agent,
		Admission:      controller,
		AdmissionAudit: &rexcontrolplane.AuditLog{},
	}
	trustedCtx := rexctx.WithTrustedExecutionContext(ctx, rexctx.TrustedExecutionContext{
		TenantID:      "tenant-a",
		WorkloadClass: rexcontrolplane.WorkloadImportant,
		SessionID:     "sess-1",
	})

	err = provider.handleEventDecision(trustedCtx, rexgateway.Decision{
		Decision:   rexgateway.SignalDecisionStart,
		WorkflowID: "wf-1",
		RunID:      "wf-1:run",
	}, rexevents.CanonicalEvent{
		ID:            "evt-1",
		Type:          rexevents.TypeTaskRequested,
		Partition:     "local",
		IngressOrigin: rexevents.OriginInternal,
		Payload: map[string]any{
			"task_id":        "task-1",
			"instruction":    "review the code",
			"workspace":      t.TempDir(),
			"edit_permitted": false,
		},
	})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return controller.ReleaseCount() == 1
	}, 2*time.Second, 20*time.Millisecond)
}

func TestHandleEventDecisionReleasesAdmissionWhenQueueIsFull(t *testing.T) {
	t.Parallel()

	agent := &rexpkg.Agent{
		Runtime: rexruntime.New(rexconfig.Config{QueueCapacity: 1}, nil),
	}
	require.True(t, agent.Runtime.Enqueue(rexruntime.WorkItem{WorkflowID: "wf-block", RunID: "wf-block:run"}))

	controller := &countingAdmissionController{
		decisions: []rexcontrolplane.AdmissionDecision{{Allowed: true, Reason: "capacity_available"}},
	}
	provider := &RexRuntimeProvider{
		Agent:          agent,
		Admission:      controller,
		AdmissionAudit: &rexcontrolplane.AuditLog{},
	}

	trustedCtx := rexctx.WithTrustedExecutionContext(context.Background(), rexctx.TrustedExecutionContext{
		TenantID:      "tenant-a",
		WorkloadClass: rexcontrolplane.WorkloadImportant,
		SessionID:     "sess-2",
	})

	err := provider.handleEventDecision(trustedCtx, rexgateway.Decision{
		Decision:   rexgateway.SignalDecisionStart,
		WorkflowID: "wf-queue",
		RunID:      "wf-queue:run",
	}, rexevents.CanonicalEvent{
		ID:            "evt-queue",
		Type:          rexevents.TypeTaskRequested,
		Partition:     "local",
		IngressOrigin: rexevents.OriginInternal,
		Payload: map[string]any{
			"task_id":     "task-queue",
			"instruction": "queue should reject",
		},
	})
	require.EqualError(t, err, "rex runtime queue full")
	require.Equal(t, 1, controller.ReleaseCount())
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
	mu              sync.Mutex
	decisions       []rexcontrolplane.AdmissionDecision
	decideRequests  []rexcontrolplane.AdmissionRequest
	releaseRequests []rexcontrolplane.AdmissionRequest
	decideCalls     atomic.Int32
	releaseCalls    atomic.Int32
}

func (c *countingAdmissionController) Admit(req rexcontrolplane.AdmissionRequest) bool {
	return c.Decide(req).Allowed
}

func (c *countingAdmissionController) Decide(req rexcontrolplane.AdmissionRequest) rexcontrolplane.AdmissionDecision {
	c.mu.Lock()
	c.decideRequests = append(c.decideRequests, req)
	var decision rexcontrolplane.AdmissionDecision
	if len(c.decisions) > 0 {
		decision = c.decisions[0]
		if len(c.decisions) > 1 {
			c.decisions = c.decisions[1:]
		}
	}
	c.mu.Unlock()
	c.decideCalls.Add(1)
	return decision
}

func (c *countingAdmissionController) Release(req rexcontrolplane.AdmissionRequest) {
	c.mu.Lock()
	c.releaseRequests = append(c.releaseRequests, req)
	c.mu.Unlock()
	c.releaseCalls.Add(1)
}

func (c *countingAdmissionController) DecideCount() int {
	return int(c.decideCalls.Load())
}

func (c *countingAdmissionController) ReleaseCount() int {
	return int(c.releaseCalls.Load())
}

type bridgeHandlerDouble struct {
	mu                    sync.Mutex
	failuresRemaining     int
	blockUntilContextDone bool
	calls                 []string
}

func (h *bridgeHandlerDouble) Handle(ctx context.Context, event rexevents.CanonicalEvent) error {
	h.mu.Lock()
	h.calls = append(h.calls, event.ID)
	fail := h.failuresRemaining > 0
	if fail {
		h.failuresRemaining--
	}
	block := h.blockUntilContextDone
	h.mu.Unlock()
	if block {
		<-ctx.Done()
	}
	if fail {
		return fmt.Errorf("transient failure for %s", event.ID)
	}
	return nil
}

type manualClock struct {
	mu  sync.Mutex
	now time.Time
}

func newManualClock(now time.Time) *manualClock {
	return &manualClock{now: now.UTC()}
}

func (c *manualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
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
