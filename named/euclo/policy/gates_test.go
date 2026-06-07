package policy

import (
	"context"
	"sync"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/governance/authorization"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
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
	permitted, ok := contextdata.GetTyped[bool](env, policyMutationKey)
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
	contextdata.SetTyped(env, policyDecisionKey, decision)

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result["decision"] != "allow" {
		t.Fatalf("expected allow decision, got %v", result["decision"])
	}
	got, ok := contextdata.GetTyped[*PolicyDecision](env, policyDecisionKey)
	if !ok || got == nil {
		t.Fatal("expected policy decision written to envelope")
	}
}

func TestGateNodeDeny(t *testing.T) {
	node := NewGateNode("gate1", NewEvaluator())
	env := contextdata.NewEnvelope("task-123", "session-456")
	contextdata.SetTyped(env, policyDecisionKey, &PolicyDecision{
		MutationPermitted:    false,
		HITLRequired:         false,
		VerificationRequired: false,
		ReasonCodes:          []string{"edit_not_permitted"},
	})

	if _, err := node.Execute(context.Background(), env); err == nil {
		t.Fatal("expected deny error")
	}
}

func TestGateNodeAskWithBroker(t *testing.T) {
	broker := authorization.NewHITLBroker(250 * time.Millisecond)
	node := NewGateNode("gate1", NewEvaluator()).WithHITLBroker(broker).WithTelemetry(&gateTelemetrySink{})
	env := contextdata.NewEnvelope("task-123", "session-456")
	contextdata.SetTyped(env, policyDecisionKey, &PolicyDecision{
		MutationPermitted:    false,
		HITLRequired:         true,
		VerificationRequired: false,
		ReasonCodes:          []string{"mutating_family"},
	})

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
	if got, ok := contextdata.GetTyped[bool](env, policyHITLTriggeredKey); !ok || got != true {
		t.Fatalf("expected hitl triggered true, got %v (ok=%v)", got, ok)
	}
	if got, ok := contextdata.GetTyped[*interaction.HITLResponse](env, policyHITLResponseKey); !ok || got == nil {
		t.Fatalf("expected hitl response written, got %v (ok=%v)", got, ok)
	}
}

func TestGateNodeAskWithPermissionManagerFallback(t *testing.T) {
	pm := &stubPermissionManager{}
	node := NewGateNode("gate1", NewEvaluator()).WithPermissionManager(pm)
	env := contextdata.NewEnvelope("task-123", "session-456")
	contextdata.SetTyped(env, policyDecisionKey, &PolicyDecision{
		MutationPermitted:    false,
		HITLRequired:         true,
		VerificationRequired: false,
		ReasonCodes:          []string{"mutating_family"},
	})

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
	_, ok := contextdata.GetTyped[bool](env, policyMutationKey)
	if !ok {
		t.Error("Expected mutation_permitted in envelope")
	}

	_, ok = contextdata.GetTyped[bool](env, policyHITLRequiredKey)
	if !ok {
		t.Error("Expected hitl_required in envelope")
	}

	_, ok = contextdata.GetTyped[bool](env, policyVerificationKey)
	if !ok {
		t.Error("Expected verification_required in envelope")
	}

	_, ok = contextdata.GetTyped[[]string](env, policyReasonCodesKey)
	if !ok {
		t.Error("Expected reason_codes in envelope")
	}
}

type gateTelemetrySink struct {
	mu     sync.Mutex
	events []telemetry.Event
}

func (s *gateTelemetrySink) Emit(event telemetry.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

type stubPermissionManager struct {
	called bool
}

func (s *stubPermissionManager) RequireApproval(ctx context.Context, agentID string, desc permissions.PermissionDescriptor, justification string, scope authorization.GrantScope, risk authorization.RiskLevel, duration time.Duration) error {
	s.called = true
	return nil
}
