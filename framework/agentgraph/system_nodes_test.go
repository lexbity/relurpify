package agentgraph

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/retrieval"
)

func TestStreamTriggerNodeContract(t *testing.T) {
	node := NewContextStreamNode("stream-node", nil, retrieval.RetrievalQuery{Text: "query"}, 128)
	contract := node.Contract()
	if contract.SideEffectClass != SideEffectContext {
		t.Fatalf("expected SideEffectContext, got %q", contract.SideEffectClass)
	}
	if contract.Idempotency != IdempotencyReplaySafe {
		t.Fatalf("expected replay-safe idempotency, got %q", contract.Idempotency)
	}
	if err := validateNodeContract(node, contract); err != nil {
		t.Fatalf("expected stream node contract to validate, got %v", err)
	}
	resolved := ResolveNodeContract(node)
	if resolved.SideEffectClass != SideEffectContext {
		t.Fatalf("expected resolved side effect context, got %q", resolved.SideEffectClass)
	}
}
