package gateway

import (
	"context"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/named/rex/events"
)

type stubWorkflowReader struct {
	workflow *memory.WorkflowRecord
	run      *memory.WorkflowRunRecord
}

func (s stubWorkflowReader) GetWorkflow(context.Context, string) (*memory.WorkflowRecord, bool, error) {
	if s.workflow == nil {
		return nil, false, nil
	}
	copy := *s.workflow
	return &copy, true, nil
}

func (s stubWorkflowReader) GetRun(context.Context, string) (*memory.WorkflowRunRecord, bool, error) {
	if s.run == nil {
		return nil, false, nil
	}
	copy := *s.run
	return &copy, true, nil
}

func TestDefaultGatewayDecideAndClassifyBranches(t *testing.T) {
	gw := DefaultGateway{}
	if got := gw.Decide(events.CanonicalEvent{Type: "  "}); got != SignalDecisionReject {
		t.Fatalf("expected reject for blank event, got %s", got)
	}
	if got := classifyEvent("workflow.signal.ready"); got != SignalDecisionSignal {
		t.Fatalf("expected signal classification, got %s", got)
	}
	if got := classifyEvent("task.requested"); got != SignalDecisionStart {
		t.Fatalf("expected start classification, got %s", got)
	}
	if got := classifyEvent("something.else"); got != SignalDecisionStart {
		t.Fatalf("expected default start classification, got %s", got)
	}
}

func TestDefaultGatewayResolveAndValidationHelpers(t *testing.T) {
	ctx := context.Background()
	store := stubWorkflowReader{
		workflow: &memory.WorkflowRecord{WorkflowID: "wf-1", Status: memory.WorkflowRunStatusRunning},
		run:      &memory.WorkflowRunRecord{RunID: "wf-1:run", Status: memory.WorkflowRunStatusRunning},
	}
	gw := DefaultGateway{Store: store}

	start, err := gw.Resolve(ctx, events.CanonicalEvent{Type: events.TypeTaskRequested, Payload: map[string]any{"workflow_id": "wf-1"}})
	if err != nil || start.Decision != SignalDecisionSignal {
		t.Fatalf("unexpected start resolution: %+v err=%v", start, err)
	}

	signal, err := gw.Resolve(ctx, events.CanonicalEvent{
		Type:          events.TypeWorkflowSignal,
		IngressOrigin: events.OriginPeer,
		Payload: map[string]any{
			"workflow_id":     "wf-1",
			"expected_signal": "resume",
			"signal":          "resume",
		},
	})
	if err != nil || signal.Decision != SignalDecisionSignal {
		t.Fatalf("unexpected signal resolution: %+v err=%v", signal, err)
	}

	if err := ValidateSignal("resume", "resume"); err != nil {
		t.Fatalf("expected matching signal to validate: %v", err)
	}
	if err := ValidateSignal("resume", "wrong"); err == nil {
		t.Fatal("expected mismatched signal validation to fail")
	}
	if got := firstNonEmpty(" ", "a", "b"); got != "a" {
		t.Fatalf("unexpected firstNonEmpty result: %q", got)
	}
	if got := stringValue(nil); got != "" {
		t.Fatalf("unexpected stringValue result: %q", got)
	}
	identity := gw.IdentityFor(events.CanonicalEvent{Type: events.TypeTaskRequested, ActorID: "actor", Partition: "local", IdempotencyKey: "idem"})
	if !strings.HasPrefix(identity, "rexwf:") || len(identity) != len("rexwf:")+64 {
		t.Fatalf("expected sha256 identity, got %q", identity)
	}
}

func TestDefaultGatewayStartAndSignalGuardBranches(t *testing.T) {
	ctx := context.Background()
	gw := DefaultGateway{}
	if err := gw.ensureStartAllowed(ctx, "wf-1"); err != nil {
		t.Fatalf("nil store should allow starts: %v", err)
	}
	if gw.hasWorkflow(ctx, "wf-1") {
		t.Fatalf("expected hasWorkflow to be false without a store")
	}
	if err := gw.validateSignalEvent(ctx, "", "", events.CanonicalEvent{Type: events.TypeWorkflowSignal, IngressOrigin: events.OriginPeer}); err == nil {
		t.Fatalf("expected workflow identity rejection")
	}
	if err := gw.validateSignalEvent(ctx, "wf-1", "run-1", events.CanonicalEvent{Type: events.TypeCallbackReceived, IngressOrigin: events.OriginExternal, Payload: map[string]any{"expected_callback": "cb", "callback_key": "cb"}}); err == nil {
		t.Fatalf("expected untrusted signal rejection")
	}
	if err := ValidateSignal("", "cb"); err == nil {
		t.Fatalf("expected empty validation failure")
	}
	store := stubWorkflowReader{
		workflow: &memory.WorkflowRecord{WorkflowID: "wf-2", Status: memory.WorkflowRunStatusCompleted},
		run:      &memory.WorkflowRunRecord{RunID: "run-2", Status: memory.WorkflowRunStatusFailed},
	}
	gw = DefaultGateway{Store: store}
	if err := gw.ensureStartAllowed(ctx, "wf-2"); err != nil {
		t.Fatalf("completed workflow should still allow start decision path: %v", err)
	}
	if !gw.hasWorkflow(ctx, "wf-2") {
		t.Fatalf("expected hasWorkflow to detect workflow")
	}
	decision, err := gw.Resolve(ctx, events.CanonicalEvent{Type: events.TypeTaskRequested, IngressOrigin: events.OriginPeer, Payload: map[string]any{"workflow_id": "wf-2"}})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if decision.Decision != SignalDecisionSignal {
		t.Fatalf("existing workflow should resolve as signal: %+v", decision)
	}
}
