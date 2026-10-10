package memory

import (
	"testing"

	relurpctx "codeburg.org/lexbit/relurpify/context"
)

// TestTaskMemoryEnforcesEntryCap: a task's entries cap at 1024; the oldest
// entries are evicted past the cap and the eviction counter rises.
func TestTaskMemoryEnforcesEntryCap(t *testing.T) {
	s := NewWorkingMemoryStore()
	task := s.Scope("task-cap")
	for i := 0; i < taskMemoryMaxEntries+10; i++ {
		task.Set(keyOf(i), i, relurpctx.MemoryClassWorking)
	}
	if got := len(task.Keys()); got != taskMemoryMaxEntries {
		t.Fatalf("task entries = %d, want %d", got, taskMemoryMaxEntries)
	}
	if Evictions() < 10 {
		t.Fatalf("eviction counter = %d, want >= 10", Evictions())
	}
	// Oldest keys evicted, newest retained.
	if _, ok := task.Get(keyOf(0)); ok {
		t.Fatal("oldest entry survived the cap eviction")
	}
	if _, ok := task.Get(keyOf(taskMemoryMaxEntries + 9)); !ok {
		t.Fatal("newest entry missing")
	}
}

// TestWorkingMemoryEvictIsIdempotent: Evicting a missing task is a no-op;
// evicting a present task removes it and its keys.
func TestWorkingMemoryEvictIsIdempotent(t *testing.T) {
	s := NewWorkingMemoryStore()
	s.Evict("never-existed") // must not panic

	task := s.Scope("task-1")
	task.Set("k", "v", relurpctx.MemoryClassWorking)
	s.Evict("task-1")
	if _, ok := task.Get("k"); ok {
		t.Fatal("entry survived Evict")
	}
	s.Evict("task-1") // second call is a no-op
}

func keyOf(i int) string {
	digits := []byte{}
	if i == 0 {
		return "key-0"
	}
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return "key-" + string(digits)
}
