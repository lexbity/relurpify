package framework_test

import (
	"github.com/lexcodex/relurpify/framework/core"
	"testing"
)

// TestContextSnapshotRestore verifies snapshot and restore round-trips all
// portions of the context (values, variables, history) without data loss.
func TestContextSnapshotRestore(t *testing.T) {
	ctx := core.NewContext()
	ctx.Set("task.id", "123")
	ctx.SetVariable("cursor", 42)
	ctx.SetKnowledge("analysis", "done")
	ctx.AddInteraction("user", "hello", nil)

	snap := ctx.Snapshot()
	ctx.Set("task.id", "456")
	ctx.SetVariable("cursor", 0)

	if err := ctx.Restore(snap); err != nil {
		t.Fatalf("restore failed: %v", err)
	}

	if val, _ := ctx.Get("task.id"); val != "123" {
		t.Fatalf("expected task.id=123, got %v", val)
	}
	if val, _ := ctx.GetVariable("cursor"); val != 42 {
		t.Fatalf("expected cursor=42, got %v", val)
	}
	if len(ctx.History()) != 1 {
		t.Fatalf("expected history size 1, got %d", len(ctx.History()))
	}
}

func TestContextSnapshotDeepCopy(t *testing.T) {
	ctx := core.NewContext()
	nested := map[string]interface{}{"key": "value"}
	ctx.Set("nested", nested)

	snap := ctx.Snapshot()
	nested["key"] = "mutated"

	if err := ctx.Restore(snap); err != nil {
		t.Fatalf("restore failed: %v", err)
	}
	val, _ := ctx.Get("nested")
	got, ok := val.(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested map, got %T", val)
	}
	if got["key"] != "value" {
		t.Fatalf("expected snapshot to preserve nested value, got %v", got["key"])
	}
}

func TestContextCloneGobFailureFallback(t *testing.T) {
	ctx := core.NewContext()
	ctx.Set("safe", "value")
	ctx.Set("fn", func() {})

	clone := ctx.Clone()
	val, _ := clone.Get("safe")
	if val != "value" {
		t.Fatalf("expected clone to keep safe value, got %v", val)
	}
}

func TestContextRegistryHandles(t *testing.T) {
	ctx := core.NewContext()
	handle := ctx.SetHandle("registry.handle", func() {})
	if handle == "" {
		t.Fatalf("expected handle to be set")
	}
	resolved, ok := ctx.GetHandle("registry.handle")
	if !ok || resolved == nil {
		t.Fatalf("expected handle to resolve")
	}

	clone := ctx.Clone()
	resolvedClone, ok := clone.GetHandle("registry.handle")
	if !ok || resolvedClone == nil {
		t.Fatalf("expected clone to resolve handle")
	}
}

func TestContextRegistryScopeCleanup(t *testing.T) {
	ctx := core.NewContext()
	scoped := ctx.SetHandleScoped("registry.scoped", func() {}, "task-1")
	unscoped := ctx.SetHandle("registry.unscoped", func() {})
	if scoped == "" || unscoped == "" {
		t.Fatalf("expected handles to be set")
	}

	ctx.ClearHandleScope("task-1")

	if _, ok := ctx.GetHandle("registry.scoped"); ok {
		t.Fatalf("expected scoped handle to be cleared")
	}
	if _, ok := ctx.GetHandle("registry.unscoped"); !ok {
		t.Fatalf("expected unscoped handle to remain")
	}
}
