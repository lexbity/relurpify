package contextdata

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing"
)

// TestApplyBranchMergesDeterministic proves the merge is insensitive to map
// iteration order and to the order of entries within each unit's delta slices.
// The unit Index order is fixed; every other degree of freedom is perturbed.
// Because the merge contains no goroutines in Phase 2, this is the structural
// analogue of the scheduling-shuffle test Phase 3 adds at the graph level.
func TestApplyBranchMergesDeterministic(t *testing.T) {
	rng := rand.New(rand.NewSource(1))

	base := map[string]any{
		"keep": "keep",
		"del":  "base-del",
		"over": "base-over",
	}
	specs := []struct {
		added    []string
		modified []string
		deleted  []string
		values   map[string]any
	}{
		{modified: []string{"over"}, deleted: []string{"del"}, added: []string{"add0"}, values: map[string]any{"over": "u0", "add0": "a0"}},
		{modified: []string{"over", "keep"}, added: []string{"add1"}, values: map[string]any{"over": "u1", "keep": "keep1", "add1": "a1"}},
		{modified: []string{"over"}, deleted: []string{"add0"}, values: map[string]any{"over": "u2"}},
		{modified: []string{"keep"}, added: []string{"add3"}, values: map[string]any{"keep": "keep3", "add3": "a3"}},
	}
	want := map[string]any{"keep": "keep3", "over": "u2", "add1": "a1", "add3": "a3"}
	wantConflicts := []string{"add0", "keep", "over"}

	for iter := 0; iter < 1000; iter++ {
		parent := NewEnvelope("task-1", "session-1")
		for k, v := range base {
			parent.SetWorkingValueWithClass(k, v, MemoryClassTask)
		}

		units := make([]BranchMergeUnit, 0, len(specs))
		for i, spec := range specs {
			env := NewEnvelope("task-1", "session-1")
			for k, v := range spec.values {
				env.SetWorkingValueWithClass(k, v, MemoryClassTask)
			}
			// Junk keys perturb envelope map iteration without appearing in
			// any delta, so they must not influence the merge.
			for j := 0; j < rng.Intn(8); j++ {
				env.SetWorkingValueWithClass(fmt.Sprintf("junk-%d-%d-%d", iter, i, j), rng.Intn(1000), MemoryClassTask)
			}
			units = append(units, BranchMergeUnit{
				Index: i,
				ID:    fmt.Sprintf("b%d", i),
				Delta: BranchDelta{
					WorkingMemoryAdded:    shuffledStrings(rng, spec.added),
					WorkingMemoryModified: shuffledStrings(rng, spec.modified),
					WorkingMemoryDeleted:  shuffledStrings(rng, spec.deleted),
				},
				Env: env,
			})
		}

		stats, err := parent.ApplyBranchMerges(units)
		if err != nil {
			t.Fatalf("iteration %d: unexpected error: %v", iter, err)
		}
		if got := parent.WorkingDataSnapshot(); !reflect.DeepEqual(got, want) {
			t.Fatalf("iteration %d: working data = %#v, want %#v", iter, got, want)
		}
		if !reflect.DeepEqual(stats.Conflicts, wantConflicts) {
			t.Fatalf("iteration %d: conflicts = %v, want %v", iter, stats.Conflicts, wantConflicts)
		}
	}
}

func shuffledStrings(rng *rand.Rand, in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := append([]string(nil), in...)
	rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}
