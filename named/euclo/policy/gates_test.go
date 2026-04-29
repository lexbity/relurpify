package policy

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
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
