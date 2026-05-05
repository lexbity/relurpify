package policy

import (
	"context"
	"sync"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

func TestGateNodeExecute(t *testing.T) {
	evaluator := NewEvaluator()
	node := NewGateNode("gate1", evaluator)

	env := contextdata.NewEnvelope("task-123", "session-456")

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}

	// Check that decision was written to envelope
	permitted, ok := env.GetWorkingValue("euclo.policy.mutation_permitted")
	if !ok {
		t.Error("Expected mutation_permitted in envelope")
	}

	if permitted != true {
		t.Errorf("Expected mutation_permitted true, got %v", permitted)
	}
}

func TestGateNodeAllowWritesDecision(t *testing.T) {
	node := NewGateNode("gate1", NewEvaluator())
	env := contextdata.NewEnvelope("task-123", "session-456")
	decision := &PolicyDecision{
		MutationPermitted:    true,
		HITLRequired:         false,
		VerificationRequired: true,
		ReasonCodes:          []string{"read_only_family"},
	}
	env.SetWorkingValue(policyDecisionKey, decision, contextdata.MemoryClassTask)

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result["decision"] != "allow" {
		t.Fatalf("expected allow decision, got %v", result["decision"])
	}
	got, ok := env.GetWorkingValue(policyDecisionKey)
	if !ok || got == nil {
		t.Fatal("expected policy decision written to envelope")
	}
}

func TestGateNodeDeny(t *testing.T) {
	node := NewGateNode("gate1", NewEvaluator())
	env := contextdata.NewEnvelope("task-123", "session-456")
	env.SetWorkingValue(policyDecisionKey, &PolicyDecision{
		MutationPermitted:    false,
		HITLRequired:         false,
		VerificationRequired: false,
		ReasonCodes:          []string{"edit_not_permitted"},
	}, contextdata.MemoryClassTask)

	if _, err := node.Execute(context.Background(), env); err == nil {
		t.Fatal("expected deny error")
	}
}

func TestGateNodeAskWithBroker(t *testing.T) {
	broker := authorization.NewHITLBroker(250 * time.Millisecond)
	node := NewGateNode("gate1", NewEvaluator()).WithHITLBroker(broker).WithTelemetry(&gateTelemetrySink{})
	env := contextdata.NewEnvelope("task-123", "session-456")
	env.SetWorkingValue(policyDecisionKey, &PolicyDecision{
		MutationPermitted:    false,
		HITLRequired:         true,
		VerificationRequired: false,
		ReasonCodes:          []string{"mutating_family"},
	}, contextdata.MemoryClassTask)

	events, cancel := broker.Subscribe(1)
	defer cancel()

	go func() {
		for event := range events {
			if event.Type == authorization.HITLEventRequested && event.Request != nil {
				_ = broker.Approve(authorization.PermissionDecision{
					RequestID:  event.Request.ID,
					Approved:   true,
					ApprovedBy: "tester",
					Scope:      authorization.GrantScopeOneTime,
				})
				return
			}
		}
	}()

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result["decision"] != "ask_approved" {
		t.Fatalf("expected ask_approved decision, got %v", result["decision"])
	}
	if got, ok := env.GetWorkingValue(policyHITLTriggeredKey); !ok || got != true {
		t.Fatalf("expected hitl triggered true, got %v (ok=%v)", got, ok)
	}
	if got, ok := env.GetWorkingValue(policyHITLResponseKey); !ok || got == nil {
		t.Fatalf("expected hitl response written, got %v (ok=%v)", got, ok)
	}
}

func TestGateNodeAskWithPermissionManagerFallback(t *testing.T) {
	pm := &stubPermissionManager{}
	node := NewGateNode("gate1", NewEvaluator()).WithPermissionManager(pm)
	env := contextdata.NewEnvelope("task-123", "session-456")
	env.SetWorkingValue(policyDecisionKey, &PolicyDecision{
		MutationPermitted:    false,
		HITLRequired:         true,
		VerificationRequired: false,
		ReasonCodes:          []string{"mutating_family"},
	}, contextdata.MemoryClassTask)

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result["decision"] != "ask_approved" {
		t.Fatalf("expected ask_approved decision, got %v", result["decision"])
	}
	if !pm.called {
		t.Fatal("expected permission manager fallback to be used")
	}
}

func TestGateNodeID(t *testing.T) {
	evaluator := NewEvaluator()
	node := NewGateNode("gate1", evaluator)

	if node.ID() != "gate1" {
		t.Errorf("Expected ID gate1, got %s", node.ID())
	}
}

func TestGateNodeType(t *testing.T) {
	evaluator := NewEvaluator()
	node := NewGateNode("gate1", evaluator)

	if node.Type() != "gate" {
		t.Errorf("Expected Type gate, got %s", node.Type())
	}
}

func TestGateNodeDecisionWritten(t *testing.T) {
	evaluator := NewEvaluator()
	node := NewGateNode("gate1", evaluator)

	env := contextdata.NewEnvelope("task-123", "session-456")

	_, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Check all decision fields are written to envelope
	_, ok := env.GetWorkingValue("euclo.policy.mutation_permitted")
	if !ok {
		t.Error("Expected mutation_permitted in envelope")
	}

	_, ok = env.GetWorkingValue("euclo.policy.hitl_required")
	if !ok {
		t.Error("Expected hitl_required in envelope")
	}

	_, ok = env.GetWorkingValue("euclo.policy.verification_required")
	if !ok {
		t.Error("Expected verification_required in envelope")
	}

	_, ok = env.GetWorkingValue("euclo.policy.reason_codes")
	if !ok {
		t.Error("Expected reason_codes in envelope")
	}
}

type gateTelemetrySink struct {
	mu     sync.Mutex
	events []core.Event
}

func (s *gateTelemetrySink) Emit(event core.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

type stubPermissionManager struct {
	called bool
}

func (s *stubPermissionManager) RequireApproval(ctx context.Context, agentID string, desc contracts.PermissionDescriptor, justification string, scope authorization.GrantScope, risk authorization.RiskLevel, duration time.Duration) error {
	s.called = true
	return nil
}
