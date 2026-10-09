package contextdata

import (
	"errors"
	"reflect"
	"testing"
)

// mergeEnv builds a branch-final envelope carrying values under task-1.
func mergeEnv(values map[string]any) *Envelope {
	env := NewEnvelope(testTaskID, testSessionID)
	for k, v := range values {
		env.SetWorkingValueWithClass(k, v, MemoryClassTask)
	}
	return env
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// TestComputeBranchDeltaIgnoresUnchangedValues pins the precise modification
// detection that makes the merge's deletion propagation sound: a key the branch
// did not actually change must not appear in the delta, or an untouched branch
// would resurrect another branch's deletion.
func TestComputeBranchDeltaIgnoresUnchangedValues(t *testing.T) {
	parent := NewEnvelope(testTaskID, testSessionID)
	parent.SetWorkingValueWithClass("same", "value", MemoryClassTask)
	parent.SetWorkingValueWithClass("changed", "old", MemoryClassTask)
	parent.SetWorkingValueWithClass("deleted", "gone", MemoryClassTask)

	child := parent.Clone()
	child.SetWorkingValueWithClass("changed", "updated", MemoryClassTask)
	child.DeleteWorkingValue("deleted")

	delta := ComputeBranchDelta(parent, child)
	if containsString(delta.WorkingMemoryAdded, "same") || containsString(delta.WorkingMemoryModified, "same") {
		t.Fatalf("unchanged key reported as written: %#v", delta)
	}
	if !containsString(delta.WorkingMemoryModified, "changed") {
		t.Fatalf("changed key not reported as modified: %#v", delta)
	}
	if !containsString(delta.WorkingMemoryDeleted, "deleted") {
		t.Fatalf("deleted key not reported as deleted: %#v", delta)
	}
}

func TestApplyBranchMergesEmpty(t *testing.T) {
	parent := NewEnvelope(testTaskID, testSessionID)
	parent.SetWorkingValueWithClass(testKeyKeep, "v", MemoryClassTask)

	stats, err := parent.ApplyBranchMerges(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(stats, MergeStats{}) {
		t.Fatalf("expected zero stats, got %#v", stats)
	}
	if got := parent.WorkingDataSnapshot(); !reflect.DeepEqual(got, map[string]any{testKeyKeep: "v"}) {
		t.Fatalf("empty merge mutated working data: %#v", got)
	}
}

func TestApplyBranchMergesNilReceiver(t *testing.T) {
	var parent *Envelope
	stats, err := parent.ApplyBranchMerges([]BranchMergeUnit{{Index: 0}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(stats, MergeStats{}) {
		t.Fatalf("expected zero stats for nil receiver, got %#v", stats)
	}
}

func TestApplyBranchMergesAddsAndModifiesInPlace(t *testing.T) {
	parent := NewEnvelope(testTaskID, testSessionID)
	parent.SetWorkingValueWithClass(testKeyBase, 1, MemoryClassTask)
	parent.SetWorkingValueWithClass(testKeyKeep, 2, MemoryClassTask)

	stats, err := parent.ApplyBranchMerges([]BranchMergeUnit{
		{Index: 0, ID: "a", Delta: BranchDelta{WorkingMemoryModified: []string{testKeyBase}}, Env: mergeEnv(map[string]any{testKeyBase: 10})},
		{Index: 1, ID: "b", Delta: BranchDelta{WorkingMemoryAdded: []string{testKeyNew}}, Env: mergeEnv(map[string]any{testKeyNew: 3})},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]any{testKeyBase: 10, testKeyKeep: 2, testKeyNew: 3}
	if got := parent.WorkingDataSnapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("working data = %#v, want %#v", got, want)
	}
	if stats.UnitsApplied != 2 || stats.KeysWritten != 2 || stats.KeysDeleted != 0 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
	if len(stats.Conflicts) != 0 {
		t.Fatalf("expected no conflicts, got %v", stats.Conflicts)
	}
}

func TestApplyBranchMergesHigherIndexWins(t *testing.T) {
	parent := NewEnvelope(testTaskID, testSessionID)
	stats, err := parent.ApplyBranchMerges([]BranchMergeUnit{
		{Index: 0, ID: "a", Delta: BranchDelta{WorkingMemoryAdded: []string{"k"}}, Env: mergeEnv(map[string]any{"k": "u0"})},
		{Index: 1, ID: "b", Delta: BranchDelta{WorkingMemoryAdded: []string{"k"}}, Env: mergeEnv(map[string]any{"k": "u1"})},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := parent.WorkingDataSnapshot()["k"]; got != "u1" {
		t.Fatalf("expected u1 (higher index), got %v", got)
	}
	if !reflect.DeepEqual(stats.Conflicts, []string{"k"}) {
		t.Fatalf("expected conflict [k], got %v", stats.Conflicts)
	}
}

func TestApplyBranchMergesRejectsNonAscending(t *testing.T) {
	parent := NewEnvelope(testTaskID, testSessionID)
	parent.SetWorkingValueWithClass("k", testKeyBase, MemoryClassTask)

	cases := map[string][]BranchMergeUnit{
		"descending": {
			{Index: 1, Delta: BranchDelta{WorkingMemoryAdded: []string{"k"}}, Env: mergeEnv(map[string]any{"k": "b"})},
			{Index: 0, Delta: BranchDelta{WorkingMemoryAdded: []string{"k"}}, Env: mergeEnv(map[string]any{"k": "a"})},
		},
		"duplicate": {
			{Index: 0, Delta: BranchDelta{WorkingMemoryAdded: []string{"k"}}, Env: mergeEnv(map[string]any{"k": "a"})},
			{Index: 0, Delta: BranchDelta{WorkingMemoryAdded: []string{"k"}}, Env: mergeEnv(map[string]any{"k": "b"})},
		},
	}
	for name, units := range cases {
		t.Run(name, func(t *testing.T) {
			stats, err := parent.ApplyBranchMerges(units)
			if !errors.Is(err, ErrBranchOrder) {
				t.Fatalf("expected ErrBranchOrder, got %v", err)
			}
			if !reflect.DeepEqual(stats, MergeStats{}) {
				t.Fatalf("rejected merge must not report stats: %#v", stats)
			}
			if got := parent.WorkingDataSnapshot()["k"]; got != testKeyBase {
				t.Fatalf("rejected merge mutated parent: %v", got)
			}
		})
	}
}

func TestBranchMergeDeletion(t *testing.T) {
	cases := []struct {
		name    string
		base    map[string]any
		units   []BranchMergeUnit
		want    map[string]any
		deleted int
	}{
		{
			name: "write then delete",
			base: map[string]any{"k": testKeyBase},
			units: []BranchMergeUnit{
				{Index: 0, Delta: BranchDelta{WorkingMemoryModified: []string{"k"}}, Env: mergeEnv(map[string]any{"k": "u0"})},
				{Index: 1, Delta: BranchDelta{WorkingMemoryDeleted: []string{"k"}}, Env: mergeEnv(nil)},
			},
			want:    map[string]any{},
			deleted: 1,
		},
		{
			name: "delete then write",
			base: map[string]any{"k": testKeyBase},
			units: []BranchMergeUnit{
				{Index: 0, Delta: BranchDelta{WorkingMemoryDeleted: []string{"k"}}, Env: mergeEnv(nil)},
				{Index: 1, Delta: BranchDelta{WorkingMemoryAdded: []string{"k"}}, Env: mergeEnv(map[string]any{"k": "u1"})},
			},
			want: map[string]any{"k": "u1"},
		},
		{
			name: "add then delete",
			base: map[string]any{},
			units: []BranchMergeUnit{
				{Index: 0, Delta: BranchDelta{WorkingMemoryAdded: []string{"k"}}, Env: mergeEnv(map[string]any{"k": "u0"})},
				{Index: 1, Delta: BranchDelta{WorkingMemoryDeleted: []string{"k"}}, Env: mergeEnv(nil)},
			},
			want: map[string]any{},
		},
		{
			name: "untouched branch does not resurrect a deletion",
			base: map[string]any{"k": testKeyBase, "other": testKeyKeep},
			units: []BranchMergeUnit{
				{Index: 0, Delta: BranchDelta{WorkingMemoryDeleted: []string{"k"}}, Env: mergeEnv(nil)},
				{Index: 1, Delta: BranchDelta{}, Env: mergeEnv(nil)},
			},
			want:    map[string]any{"other": testKeyKeep},
			deleted: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parent := NewEnvelope(testTaskID, testSessionID)
			for k, v := range tc.base {
				parent.SetWorkingValueWithClass(k, v, MemoryClassTask)
			}
			stats, err := parent.ApplyBranchMerges(tc.units)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := parent.WorkingDataSnapshot(); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("working data = %#v, want %#v", got, tc.want)
			}
			if stats.KeysDeleted != tc.deleted {
				t.Fatalf("KeysDeleted = %d, want %d", stats.KeysDeleted, tc.deleted)
			}
		})
	}
}

func TestApplyBranchMergesConflictReporting(t *testing.T) {
	parent := NewEnvelope(testTaskID, testSessionID)
	stats, err := parent.ApplyBranchMerges([]BranchMergeUnit{
		{Index: 0, Delta: BranchDelta{WorkingMemoryAdded: []string{"a", "b"}}, Env: mergeEnv(map[string]any{"a": 1, "b": 1})},
		{Index: 1, Delta: BranchDelta{WorkingMemoryAdded: []string{"b", "c"}}, Env: mergeEnv(map[string]any{"b": 2, "c": 3})},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]any{"a": 1, "b": 2, "c": 3}
	if got := parent.WorkingDataSnapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("working data = %#v, want %#v", got, want)
	}
	if !reflect.DeepEqual(stats.Conflicts, []string{"b"}) {
		t.Fatalf("expected sorted conflicts [b], got %v", stats.Conflicts)
	}
	if stats.KeysWritten != 3 {
		t.Fatalf("expected 3 writes, got %d", stats.KeysWritten)
	}
}

func TestApplyBranchMergesSkipsMissingValue(t *testing.T) {
	parent := NewEnvelope(testTaskID, testSessionID)
	stats, err := parent.ApplyBranchMerges([]BranchMergeUnit{
		{Index: 0, Delta: BranchDelta{WorkingMemoryAdded: []string{"ghost"}}, Env: mergeEnv(nil)},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := parent.WorkingDataSnapshot()["ghost"]; ok {
		t.Fatal("ghost key must not be fabricated")
	}
	if stats.KeysWritten != 0 {
		t.Fatalf("expected 0 writes, got %d", stats.KeysWritten)
	}
	if stats.KeysSkipped != 1 {
		t.Fatalf("expected 1 skipped delta key, got %d", stats.KeysSkipped)
	}
}

func TestApplyBranchMergesUnionsReferences(t *testing.T) {
	parent := NewEnvelope(testTaskID, testSessionID)
	parent.AddStreamedContextReference(ChunkReference{ChunkID: testChunkID1, Rank: 5})

	u0 := NewEnvelope(testTaskID, testSessionID)
	u0.AddStreamedContextReference(ChunkReference{ChunkID: testChunkID1, Rank: 5}) // duplicate
	u0.AddStreamedContextReference(ChunkReference{ChunkID: testChunkID2, Rank: 2})
	u0.AddRetrievalReference(RetrievalReference{QueryID: "q1", QueryText: "dup"})
	u0.AddRetrievalReference(RetrievalReference{QueryID: "q2", QueryText: testKeyNew})
	u0.AddCheckpointReference(CheckpointReference{CheckpointID: testCheckpointID, WorkingMemoryKeys: []string{"a", "b"}})

	u1 := NewEnvelope(testTaskID, testSessionID)
	u1.AddStreamedContextReference(ChunkReference{ChunkID: "chunk-3", Rank: 1})
	u1.AddRetrievalReference(RetrievalReference{QueryID: "q1", QueryText: "dup"})
	u1.AddCheckpointReference(CheckpointReference{CheckpointID: testCheckpointID, WorkingMemoryKeys: []string{"b", "c"}})

	stats, err := parent.ApplyBranchMerges([]BranchMergeUnit{
		{Index: 0, Env: u0},
		{Index: 1, Env: u1},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Streamed: dedup by chunk ID, then sorted by Rank. First occurrence wins.
	gotChunks := make([]ChunkID, 0, 3)
	for _, ref := range parent.References.StreamedContext {
		gotChunks = append(gotChunks, ref.ChunkID)
	}
	wantChunks := []ChunkID{"chunk-3", testChunkID2, testChunkID1}
	if !reflect.DeepEqual(gotChunks, wantChunks) {
		t.Fatalf("streamed chunks = %v, want %v", gotChunks, wantChunks)
	}
	if stats.RefsStreamed != 2 {
		t.Fatalf("RefsStreamed = %d, want 2", stats.RefsStreamed)
	}

	// Retrieval: first position wins, q1 not duplicated.
	gotQueries := []string{}
	for _, ref := range parent.References.Retrieval {
		gotQueries = append(gotQueries, ref.QueryID)
	}
	if !reflect.DeepEqual(gotQueries, []string{"q1", "q2"}) {
		t.Fatalf("retrieval queries = %v, want [q1 q2]", gotQueries)
	}
	if stats.RefsRetrieval != 2 {
		t.Fatalf("RefsRetrieval = %d, want 2", stats.RefsRetrieval)
	}

	// Checkpoints: merged by ID with working-key set union.
	if len(parent.References.Checkpoints) != 1 {
		t.Fatalf("expected 1 checkpoint, got %d", len(parent.References.Checkpoints))
	}
	if got := parent.References.Checkpoints[0].WorkingMemoryKeys; !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("checkpoint keys = %v, want [a b c]", got)
	}
}

func TestApplyBranchMergesDropsReferenceOnDelete(t *testing.T) {
	parent := NewEnvelope(testTaskID, testSessionID)
	parent.SetWorkingValueWithClass("doomed", testKeyBase, MemoryClassTask)

	branch := parent.Clone()
	branch.DeleteWorkingValue("doomed")

	stats, err := parent.ApplyBranchMerges([]BranchMergeUnit{
		{Index: 0, Delta: ComputeBranchDelta(parent, branch), Env: branch},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := parent.WorkingDataSnapshot()["doomed"]; ok {
		t.Fatal("deleted key remains in working data")
	}
	if parent.References.HasWorkingMemoryKey(testTaskID, "doomed") {
		t.Fatal("working-memory reference for a deleted key must be dropped")
	}
	if stats.KeysDeleted != 1 {
		t.Fatalf("KeysDeleted = %d, want 1", stats.KeysDeleted)
	}
}
