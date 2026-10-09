package contextdata

import (
	"sync"
	"testing"
)

// TestNextSequence_AtomicPerKey proves the read-increment-write is one
// critical section (D15): N concurrent callers on one key observe N distinct
// values, and the stored counter reaches exactly N. This closes the previous
// unlocked RMW sequence-assignment defect for frames.
func TestNextSequence_AtomicPerKey(t *testing.T) {
	env := NewEnvelope("task-1", "session-1")
	const workers = 16

	results := make([]uint64, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = env.NextSequence("counter")
		}(i)
	}
	wg.Wait()

	seen := make(map[uint64]int, workers)
	for _, value := range results {
		seen[value]++
	}
	for value, count := range seen {
		if count != 1 {
			t.Fatalf("sequence %d observed %d times: concurrent emitters collided", value, count)
		}
	}
	got := env.WorkingDataSnapshot()["counter"]
	if got != uint64(workers) {
		t.Fatalf("stored counter = %v, want %d", got, workers)
	}

	// Per-key independence: a second key starts at 1 regardless of the first.
	if next := env.NextSequence("other"); next != 1 {
		t.Fatalf("second key next = %d, want 1", next)
	}
}

// TestNextSequence_StartsAtOneAndIsMonotonic locks the post-increment contract
// callers (frame sequence assignment) rely on.
func TestNextSequence_StartsAtOneAndIsMonotonic(t *testing.T) {
	env := NewEnvelope("task-1", "session-1")
	for i := uint64(1); i <= 5; i++ {
		if got := env.NextSequence("seq"); got != i {
			t.Fatalf("NextSequence step %d = %d, want %d", i, got, i)
		}
	}
}

// TestNextSequence_NormalizesLegacyIntCounter proves a pre-existing int-valued
// counter migrates without drift (an envelope persisted by the old unlocked
// RMW keeps counting upward).
func TestNextSequence_NormalizesLegacyIntCounter(t *testing.T) {
	env := NewEnvelope("task-1", "session-1")
	env.SetWorkingValue("seq", 4) // legacy int value (4 frames already emitted)
	if got := env.NextSequence("seq"); got != 5 {
		t.Fatalf("legacy int counter next = %d, want 5", got)
	}
}
