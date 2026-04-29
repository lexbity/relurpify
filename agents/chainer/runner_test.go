package chainer

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

func TestFilterStateUsesHandoffPolicy(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	env.SetWorkingValue("keep", "value-keep", contextdata.MemoryClassTask)
	env.SetWorkingValue("keep.local", "value-prefix", contextdata.MemoryClassTask)
	env.SetWorkingValue("drop", "value-drop", contextdata.MemoryClassTask)

	filtered := FilterState(env, []string{"keep"})
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered entry, got %d", len(filtered))
	}
	if got := filtered["keep"]; got != "value-keep" {
		t.Fatalf("expected keep to survive filter, got %v", got)
	}
	if _, ok := filtered["drop"]; ok {
		t.Fatal("expected drop key to be filtered out")
	}

	empty := FilterState(env, nil)
	if len(empty) != 0 {
		t.Fatalf("expected empty key list to return no state, got %d entries", len(empty))
	}
}
