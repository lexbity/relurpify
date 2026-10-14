package memory

import (
	"testing"
	"time"

	relurpctx "codeburg.org/lexbit/relurpify/context"
	"github.com/stretchr/testify/require"
)

func TestScopeReturnsSameTaskForSameID(t *testing.T) {
	s := NewWorkingMemoryStore()
	first := s.Scope("task-a")
	second := s.Scope("task-a")
	require.Same(t, first, second, "Scope must reuse the existing task memory")
	require.Equal(t, "task-a", first.TaskID())
}

func TestScopeCreatesIndependentTasks(t *testing.T) {
	s := NewWorkingMemoryStore()
	a := s.Scope("task-a")
	b := s.Scope("task-b")
	a.Set("k", "va", relurpctx.MemoryClassWorking)
	b.Set("k", "vb", relurpctx.MemoryClassStreamed)

	gotA, ok := a.Get("k")
	require.True(t, ok)
	require.Equal(t, "va", gotA.Value)
	gotB, ok := b.Get("k")
	require.True(t, ok)
	require.Equal(t, "vb", gotB.Value)
}

func TestListTasksReflectsStoreContents(t *testing.T) {
	s := NewWorkingMemoryStore()
	s.Scope("task-a")
	s.Scope("task-b")

	require.ElementsMatch(t, []string{"task-a", "task-b"}, s.ListTasks())

	s.Evict("task-a")
	require.Equal(t, []string{"task-b"}, s.ListTasks())
}

func TestListTasksReturnsEmptySliceWhenStoreIsEmpty(t *testing.T) {
	s := NewWorkingMemoryStore()
	require.Empty(t, s.ListTasks())
}

func TestSetPreservesCreatedAtOnOverwrite(t *testing.T) {
	task := NewWorkingMemoryStore().Scope("task-a")
	task.Set("k", "v1", relurpctx.MemoryClassWorking)
	first, ok := task.Get("k")
	require.True(t, ok)

	time.Sleep(2 * time.Millisecond)
	task.Set("k", "v2", relurpctx.MemoryClassStreamed)
	second, ok := task.Get("k")
	require.True(t, ok)

	require.Equal(t, "v2", second.Value)
	require.Equal(t, relurpctx.MemoryClassStreamed, second.Class)
	require.Equal(t, first.CreatedAt, second.CreatedAt, "overwrite must preserve CreatedAt")
	require.True(t, second.UpdatedAt.After(first.UpdatedAt), "overwrite must bump UpdatedAt")
}

func TestGetMissingKeyReturnsFalse(t *testing.T) {
	task := NewWorkingMemoryStore().Scope("task-a")
	entry, ok := task.Get("missing")
	require.False(t, ok)
	require.Zero(t, entry.Value)
}

func TestSnapshotIsAPointInTimeCopy(t *testing.T) {
	task := NewWorkingMemoryStore().Scope("task-a")
	task.Set("k", "v", relurpctx.MemoryClassWorking)

	snap := task.Snapshot()
	require.Len(t, snap, 1)

	// Mutating the snapshot must not touch the task memory.
	snap["injected"] = MemoryEntry{Value: "bad"}
	task.Delete("k")

	snap2 := task.Snapshot()
	require.Empty(t, snap2)
}

func TestDeleteRemovesOnlyNamedEntry(t *testing.T) {
	task := NewWorkingMemoryStore().Scope("task-a")
	task.Set("a", 1, relurpctx.MemoryClassWorking)
	task.Set("b", 2, relurpctx.MemoryClassWorking)

	task.Delete("a")
	_, ok := task.Get("a")
	require.False(t, ok)
	_, ok = task.Get("b")
	require.True(t, ok)

	task.Delete("a") // deleting a missing key is a no-op
	require.Len(t, task.Keys(), 1)
}

func TestClearRemovesAllEntries(t *testing.T) {
	task := NewWorkingMemoryStore().Scope("task-a")
	task.Set("a", 1, relurpctx.MemoryClassWorking)
	task.Set("b", 2, relurpctx.MemoryClassWorking)

	task.Clear()
	require.Empty(t, task.Keys())
	_, ok := task.Get("a")
	require.False(t, ok)
}

func TestRetrieveReturnsEmptySliceForUnknownTask(t *testing.T) {
	s := NewWorkingMemoryStore()
	results, err := s.Retrieve(t.Context(), MemoryQuery{TaskID: "nope"})
	require.NoError(t, err)
	require.NotNil(t, results, "unknown task must yield an empty, non-nil slice")
	require.Empty(t, results)
}

func TestRetrieveFiltersByPrefixAndClass(t *testing.T) {
	s := NewWorkingMemoryStore()
	task := s.Scope("task-a")
	task.Set("alpha", "a", relurpctx.MemoryClassWorking)
	task.Set("beta", "b", relurpctx.MemoryClassStreamed)
	task.Set("gamma", "g", relurpctx.MemoryClassWorking)

	t.Run("no filters returns everything", func(t *testing.T) {
		results, err := s.Retrieve(t.Context(), MemoryQuery{TaskID: "task-a"})
		require.NoError(t, err)
		require.Len(t, results, 3)
		for _, r := range results {
			require.Equal(t, "task-a", r.TaskID)
		}
	})

	t.Run("prefix filter", func(t *testing.T) {
		results, err := s.Retrieve(t.Context(), MemoryQuery{TaskID: "task-a", KeyPrefix: "al"})
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.Equal(t, "alpha", results[0].Key)
		require.Equal(t, "a", results[0].Entry.Value)
	})

	t.Run("class filter", func(t *testing.T) {
		results, err := s.Retrieve(t.Context(), MemoryQuery{TaskID: "task-a", Class: relurpctx.MemoryClassWorking})
		require.NoError(t, err)
		keys := make([]string, 0, len(results))
		for _, r := range results {
			keys = append(keys, r.Key)
		}
		require.ElementsMatch(t, []string{"alpha", "gamma"}, keys)
	})

	t.Run("prefix and class combined", func(t *testing.T) {
		results, err := s.Retrieve(t.Context(), MemoryQuery{
			TaskID: "task-a", KeyPrefix: "be", Class: relurpctx.MemoryClassStreamed,
		})
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.Equal(t, "beta", results[0].Key)
	})
}

func TestRetrieveStopsAtLimit(t *testing.T) {
	s := NewWorkingMemoryStore()
	task := s.Scope("task-a")
	for i := 0; i < 5; i++ {
		task.Set(keyOf(i), i, relurpctx.MemoryClassWorking)
	}

	results, err := s.Retrieve(t.Context(), MemoryQuery{TaskID: "task-a", Limit: 2})
	require.NoError(t, err)
	require.Len(t, results, 2, "limit must cap the result count")

	// A limit above the entry count returns everything.
	results, err = s.Retrieve(t.Context(), MemoryQuery{TaskID: "task-a", Limit: 50})
	require.NoError(t, err)
	require.Len(t, results, 5)

	// Zero limit means unbounded.
	results, err = s.Retrieve(t.Context(), MemoryQuery{TaskID: "task-a", Limit: 0})
	require.NoError(t, err)
	require.Len(t, results, 5)
}

func TestGetByKey(t *testing.T) {
	s := NewWorkingMemoryStore()
	task := s.Scope("task-a")
	task.Set("k", "v", relurpctx.MemoryClassWorking)

	entry, ok := s.GetByKey("task-a", "k")
	require.True(t, ok)
	require.Equal(t, "v", entry.Value)

	_, ok = s.GetByKey("task-a", "missing")
	require.False(t, ok)

	_, ok = s.GetByKey("unknown-task", "k")
	require.False(t, ok)
}

func TestGetTaskKeysAndSnapshot(t *testing.T) {
	s := NewWorkingMemoryStore()
	task := s.Scope("task-a")
	task.Set("a", 1, relurpctx.MemoryClassWorking)
	task.Set("b", 2, relurpctx.MemoryClassWorking)

	require.ElementsMatch(t, []string{"a", "b"}, s.GetTaskKeys("task-a"))

	snap := s.GetTaskSnapshot("task-a")
	require.Len(t, snap, 2)
	require.Equal(t, 1, snap["a"].Value)
	require.Equal(t, relurpctx.MemoryClassWorking, snap["a"].Class)

	// Unknown tasks yield empty collections, not nil.
	require.NotNil(t, s.GetTaskKeys("unknown"))
	require.NotNil(t, s.GetTaskSnapshot("unknown"))
	require.Empty(t, s.GetTaskKeys("unknown"))
	require.Empty(t, s.GetTaskSnapshot("unknown"))
}
