package core

import (
	"sync"
	"testing"
	"time"
)

func TestContextMergeConcurrentCrossMergeCompletes(t *testing.T) {
	left := NewContext()
	left.Set("left", "a")
	left.SetVariable("shared", "left")
	left.AddInteraction("assistant", "left", nil)

	right := NewContext()
	right.Set("right", "b")
	right.SetVariable("shared", "right")
	right.AddInteraction("user", "right", nil)

	start := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		left.Merge(right)
	}()
	go func() {
		defer wg.Done()
		<-start
		right.Merge(left)
	}()
	close(start)

	finished := make(chan struct{})
	go func() {
		wg.Wait()
		close(finished)
	}()

	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("concurrent Merge calls timed out; possible lock-order deadlock")
	}

	if got := left.GetString("right"); got != "b" {
		t.Fatalf("expected left context to receive right state, got %q", got)
	}
	if got := right.GetString("left"); got != "a" {
		t.Fatalf("expected right context to receive left state, got %q", got)
	}
	if len(left.History()) == 0 || len(right.History()) == 0 {
		t.Fatal("expected merged histories to remain available")
	}
}
