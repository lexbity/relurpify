package session

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCloseStack_CloseReverseOrder(t *testing.T) {
	var order []string
	cs := &CloseStack{}
	cs.Add(func(_ context.Context) error {
		order = append(order, "first")
		return nil
	})
	cs.Add(func(_ context.Context) error {
		order = append(order, "second")
		return nil
	})
	cs.Add(func(_ context.Context) error {
		order = append(order, "third")
		return nil
	})

	if err := cs.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	want := []string{"third", "second", "first"}
	if len(order) != len(want) {
		t.Fatalf("close order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("close order[%d] = %q, want %q", i, order[i], want[i])
		}
	}
}

func TestCloseStack_CloseReverseOrderSingleItem(t *testing.T) {
	called := false
	cs := &CloseStack{}
	cs.Add(func(_ context.Context) error {
		called = true
		return nil
	})

	if err := cs.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Error("close function was not called")
	}
}

func TestCloseStack_AggregatesErrors(t *testing.T) {
	cs := &CloseStack{}
	cs.Add(func(_ context.Context) error {
		return errors.New("error A")
	})
	cs.Add(func(_ context.Context) error {
		return errors.New("error B")
	})

	err := cs.Close(context.Background())
	if err == nil {
		t.Fatal("expected combined error")
	}
	if !errors.Is(err, errors.New("error A")) && !containsSubstring(err.Error(), "error A") {
		t.Errorf("error should contain 'error A', got: %v", err)
	}
	if !errors.Is(err, errors.New("error B")) && !containsSubstring(err.Error(), "error B") {
		t.Errorf("error should contain 'error B', got: %v", err)
	}
}

func TestCloseStack_Idempotent(t *testing.T) {
	var callCount atomic.Int32
	cs := &CloseStack{}
	cs.Add(func(_ context.Context) error {
		callCount.Add(1)
		return nil
	})

	ctx := context.Background()
	if err := cs.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if callCount.Load() != 1 {
		t.Errorf("first Close did not call function (count=%d)", callCount.Load())
	}

	// Second Close must be no-op.
	if err := cs.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if callCount.Load() != 1 {
		t.Errorf("second Close called function again (count=%d)", callCount.Load())
	}
}

func TestCloseStack_NilCloseFn(t *testing.T) {
	cs := &CloseStack{}
	cs.Add(nil)
	cs.Add(func(_ context.Context) error {
		return nil
	})
	cs.Add(nil)

	if err := cs.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCloseStack_NilStack(t *testing.T) {
	var cs *CloseStack
	if err := cs.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCloseStack_EmptyStack(t *testing.T) {
	cs := &CloseStack{}
	if err := cs.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCloseStack_Len(t *testing.T) {
	cs := &CloseStack{}
	if cs.Len() != 0 {
		t.Errorf("empty stack Len = %d, want 0", cs.Len())
	}
	cs.Add(func(_ context.Context) error { return nil })
	if cs.Len() != 1 {
		t.Errorf("after add Len = %d, want 1", cs.Len())
	}
	_ = cs.Close(context.Background())
	if cs.Len() != 0 {
		t.Errorf("after close Len = %d, want 0", cs.Len())
	}
}

func TestCloseStack_NilStackLen(t *testing.T) {
	var cs *CloseStack
	if cs.Len() != 0 {
		t.Errorf("nil stack Len = %d, want 0", cs.Len())
	}
}

func TestCloseStack_FailureAtStageNClosesNMinusOne(t *testing.T) {
	// Simulate: resource 1 opens, resource 2 opens, resource 3 fails.
	// Resources 1 and 2 must be closed in reverse order.
	var closed []string
	cs := &CloseStack{}
	cs.Add(func(_ context.Context) error {
		closed = append(closed, "resource1")
		return nil
	})
	cs.Add(func(_ context.Context) error {
		closed = append(closed, "resource2")
		return nil
	})
	// Resource 3 "failed" — we simulate by closing the stack with what we have.

	if err := cs.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	want := []string{"resource2", "resource1"}
	if len(closed) != len(want) {
		t.Fatalf("closed = %v, want %v", closed, want)
	}
	for i := range want {
		if closed[i] != want[i] {
			t.Fatalf("closed[%d] = %q, want %q", i, closed[i], want[i])
		}
	}
}

func TestCloseStack_SkipNilItems(t *testing.T) {
	var closed []string
	cs := &CloseStack{}
	cs.Add(nil)
	cs.Add(func(_ context.Context) error {
		closed = append(closed, "second")
		return nil
	})
	cs.Add(nil)
	cs.Add(func(_ context.Context) error {
		closed = append(closed, "fourth")
		return nil
	})

	_ = cs.Close(context.Background())

	want := []string{"fourth", "second"}
	if len(closed) != len(want) {
		t.Fatalf("closed = %v, want %v", closed, want)
	}
	for i := range want {
		if closed[i] != want[i] {
			t.Fatalf("closed[%d] = %q, want %q", i, closed[i], want[i])
		}
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && strings.Contains(s, substr)
}
