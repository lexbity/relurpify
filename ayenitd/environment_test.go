package ayenitd_test

import (
	"testing"

	"github.com/lexcodex/relurpify/ayenitd"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/memory"
)

func newTestMemory(t *testing.T) memory.MemoryStore {
	t.Helper()
	m, err := memory.NewHybridMemory(t.TempDir())
	if err != nil {
		t.Fatalf("NewHybridMemory: %v", err)
	}
	return m
}

func TestWithRegistry_ShallowCopy(t *testing.T) {
	r1 := capability.NewRegistry()
	r2 := capability.NewRegistry()

	env := ayenitd.WorkspaceEnvironment{Registry: r1}
	env2 := env.WithRegistry(r2)

	if env2.Registry != r2 {
		t.Error("WithRegistry: returned env should have the new registry")
	}
	if env.Registry != r1 {
		t.Error("WithRegistry: original env should be unchanged")
	}
}

func TestWithMemory_ShallowCopy(t *testing.T) {
	m1 := newTestMemory(t)
	m2 := newTestMemory(t)

	env := ayenitd.WorkspaceEnvironment{Memory: m1}
	env2 := env.WithMemory(m2)

	if env2.Memory != m2 {
		t.Error("WithMemory: returned env should have the new memory store")
	}
	if env.Memory != m1 {
		t.Error("WithMemory: original env should be unchanged")
	}
}

func TestWithRegistry_OtherFieldsPreserved(t *testing.T) {
	r1 := capability.NewRegistry()
	r2 := capability.NewRegistry()
	m := newTestMemory(t)

	env := ayenitd.WorkspaceEnvironment{Registry: r1, Memory: m}
	env2 := env.WithRegistry(r2)

	if env2.Memory != m {
		t.Error("WithRegistry: Memory field should be preserved in copy")
	}
}
