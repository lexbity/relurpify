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
		testKeyKeep: testKeyKeep,
		"del":       "base-del",
		testKeyOver: "base-over",
	}
	specs := []struct {
		added    []string
		modified []string
		deleted  []string
		values   map[string]any
	}{
		{modified: []string{testKeyOver}, deleted: []string{"del"}, added: []string{testKeyAdd0}, values: map[string]any{testKeyOver: "u0", testKeyAdd0: "a0"}},
		{modified: []string{testKeyOver, testKeyKeep}, added: []string{testKeyAdd1}, values: map[string]any{testKeyOver: "u1", testKeyKeep: "keep1", testKeyAdd1: "a1"}},
		{modified: []string{testKeyOver}, deleted: []string{testKeyAdd0}, values: map[string]any{testKeyOver: "u2"}},
		{modified: []string{testKeyKeep}, added: []string{testKeyAdd3}, values: map[string]any{testKeyKeep: "keep3", testKeyAdd3: "a3"}},
	}
	want := map[string]any{testKeyKeep: "keep3", testKeyOver: "u2", testKeyAdd1: "a1", testKeyAdd3: "a3"}
	wantConflicts := []string{testKeyAdd0, testKeyKeep, testKeyOver}

	for iter := 0; iter < 1000; iter++ {
		parent := NewEnvelope(testTaskID, testSessionID)
		for k, v := range base {
			parent.SetWorkingValueWithClass(k, v, MemoryClassTask)
		}

		units := make([]BranchMergeUnit, 0, len(specs))
		for i, spec := range specs {
			env := NewEnvelope(testTaskID, testSessionID)
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
