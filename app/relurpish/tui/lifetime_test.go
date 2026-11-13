package tui

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
)

func TestV2HostCleanupIsSharedAndIdempotent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	lifetime := &hostLifetime{cancel: cancel}
	var released atomic.Int64
	lifetime.add(func() { released.Add(1) })
	var group sync.WaitGroup
	for range 30 {
		group.Go(lifetime.close)
	}
	group.Wait()
	if released.Load() != 1 || ctx.Err() == nil {
		t.Fatal("concurrent shutdown duplicated cleanup or failed to cancel work")
	}
	lifetime.add(func() { released.Add(1) })
	if released.Load() != 2 {
		t.Fatal("late subscription leaked after shutdown")
	}
}

func TestV2HostCleanupContinuesAfterResourcePanic(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	lifetime := &hostLifetime{cancel: cancel}
	var order []int
	lifetime.add(func() { order = append(order, 1) })
	lifetime.add(func() { order = append(order, 2); panic("resource failure") })
	lifetime.add(func() { order = append(order, 3) })
	func() {
		defer func() {
			if recover() == nil {
				t.Error("cleanup failure disappeared")
			}
		}()
		lifetime.close()
	}()
	if !reflect.DeepEqual(order, []int{3, 2, 1}) {
		t.Fatalf("resource failure skipped earlier owners: %v", order)
	}
}

func TestV2HostReplacesSubscriptionsWithoutAccumulatingOwners(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	lifetime := &hostLifetime{cancel: cancel}
	var released atomic.Int64
	for range 1000 {
		lifetime.replace("execution", func() { released.Add(1) })
	}
	if released.Load() != 999 || len(lifetime.resources) != 1 || len(lifetime.callbacks) != 0 {
		t.Fatal("replacing subscriptions leaked ownership")
	}
	lifetime.close()
	lifetime.replace("execution", func() { released.Add(1) })
	if released.Load() != 1001 {
		t.Fatal("shutdown or late subscription release was lost")
	}
}
