package sandbox

import (
	"context"
	"testing"
	"time"
)

// TestContainerHandleTeardownIdempotent verifies that calling Teardown multiple
// times does not panic or hang. Since no real container exists, each call is a
// no-op that fails silently (command not found or container not found).
func TestContainerHandleTeardownIdempotent(t *testing.T) {
	h := NewContainerHandle("test-container-nonexistent", "docker", "nonexistent-binary")

	// First call should not panic.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	h.Teardown(ctx, 1*time.Second)

	// Second call — idempotent.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	h.Teardown(ctx2, 1*time.Second)
}

func TestContainerHandleNilHandle(t *testing.T) {
	var h *ContainerHandle
	ctx := context.Background()
	// Nil handle must not panic.
	h.Teardown(ctx, 1*time.Second)
}

func TestContainerHandleEmptyName(t *testing.T) {
	h := NewContainerHandle("", "docker", "docker")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Empty name must not panic.
	h.Teardown(ctx, 1*time.Second)
}

func TestContainerHandleRunscRuntime(t *testing.T) {
	h := NewContainerHandle("test-runsc-nonexistent", "runsc", "nonexistent-binary")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Should not panic even though runsc doesn't exist.
	h.Teardown(ctx, 1*time.Second)
}

func TestContainerHandleUnknownRuntime(t *testing.T) {
	h := NewContainerHandle("test-unknown", "unknown-runtime", "binary")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Unknown runtime is a no-op.
	h.Teardown(ctx, 1*time.Second)
}
