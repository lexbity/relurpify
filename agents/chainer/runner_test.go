package chainer

import (
	"testing"

	"codeburg.org/lexbit/relurpify/context/contextdata"
)

func TestFilterStateUsesHandoffPolicy(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	env.SetWorkingValueWithClass("keep", "value-keep", contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("keep.local", "value-prefix", contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("drop", "value-drop", contextdata.MemoryClassTask)

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
