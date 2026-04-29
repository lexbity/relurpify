package state

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/named/rex/envelope"
)

func TestComputeIdentityStable(t *testing.T) {
	env := envelope.Envelope{TaskID: "task-1", Source: "nexus", Instruction: "analyze"}
	first := ComputeIdentity(env)
	second := ComputeIdentity(env)
	if first != second {
		t.Fatalf("identity not stable: %+v %+v", first, second)
	}
}

func TestRecoveryBootWithNoWorkflowStore(t *testing.T) {
	memStore := memory.NewWorkingMemoryStore()
	candidates, err := RecoveryBoot(context.Background(), memStore)
	if err != nil {
		t.Fatalf("RecoveryBoot: %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("candidates = %v", candidates)
	}
}
