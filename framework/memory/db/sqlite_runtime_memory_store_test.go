package db

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/stretchr/testify/require"
)

func TestSQLiteRuntimeMemoryStoreStoresDeclarativeAndProceduralSeparately(t *testing.T) {
	store, err := NewSQLiteRuntimeMemoryStore(filepath.Join(t.TempDir(), "runtime_memory.db"))
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	require.NoError(t, store.PutDeclarative(ctx, memory.DeclarativeMemoryRecord{
		RecordID:    "fact-1",
		Scope:       memory.MemoryScopeProject,
		Kind:        memory.DeclarativeMemoryKindDecision,
		Title:       "Selected SQLite",
		Content:     "SQLite is the default runtime substrate.",
		Summary:     "SQLite selected for durable runtime memory.",
		TaskID:      "task-1",
		ProjectID:   "proj-1",
		ArtifactRef: "artifact://decision/1",
		Tags:        []string{"db", "decision"},
		Metadata:    map[string]any{"source": "phase-5"},
		Verified:    true,
	}))
	require.NoError(t, store.PutProcedural(ctx, memory.ProceduralMemoryRecord{
		RoutineID:   "routine-1",
		Scope:       memory.MemoryScopeProject,
		Kind:        memory.ProceduralMemoryKindCapabilityComposition,
		Name:        "checkpoint-and-summarize",
		Description: "Run summarize, then checkpoint",
		Summary:     "Reusable checkpoint routine",
		TaskID:      "task-1",
		ProjectID:   "proj-1",
		BodyRef:     "routine://checkpoint-and-summarize",
		CapabilityDependencies: []core.CapabilitySelector{{
			Kind: core.CapabilityKindTool,
			Name: "checkpoint",
		}},
		VerificationMetadata: map[string]any{"policy_snapshot": "p1"},
		Verified:             true,
		Version:              2,
		ReuseCount:           7,
	}))

	decl, err := store.SearchDeclarative(ctx, memory.DeclarativeMemoryQuery{
		Query:  "sqlite",
		Scope:  memory.MemoryScopeProject,
		Kinds:  []memory.DeclarativeMemoryKind{memory.DeclarativeMemoryKindDecision},
		TaskID: "task-1",
		Limit:  5,
	})
	require.NoError(t, err)
	require.Len(t, decl, 1)
	require.Equal(t, "fact-1", decl[0].RecordID)
	require.Equal(t, "artifact://decision/1", decl[0].ArtifactRef)
	require.Equal(t, "phase-5", decl[0].Metadata["source"])

	proc, err := store.SearchProcedural(ctx, memory.ProceduralMemoryQuery{
		Query:          "checkpoint",
		Scope:          memory.MemoryScopeProject,
		Kinds:          []memory.ProceduralMemoryKind{memory.ProceduralMemoryKindCapabilityComposition},
		TaskID:         "task-1",
		CapabilityName: "checkpoint",
		Limit:          5,
	})
	require.NoError(t, err)
	require.Len(t, proc, 1)
	require.Equal(t, "routine-1", proc[0].RoutineID)
	require.Equal(t, "checkpoint-and-summarize", proc[0].Name)
	require.Len(t, proc[0].CapabilityDependencies, 1)
}

func TestSQLiteRuntimeMemoryStoreImplementsGenericMemoryCompatibility(t *testing.T) {
	store, err := NewSQLiteRuntimeMemoryStore(filepath.Join(t.TempDir(), "runtime_memory.db"))
	require.NoError(t, err)
	defer store.Close()

	var generic memory.MemoryStore = store
	ctx := context.Background()
	require.NoError(t, generic.Remember(ctx, "mem-1", map[string]interface{}{
		"type":         "decision",
		"summary":      "Prefer narrow structured persistence.",
		"task_id":      "task-2",
		"artifact_ref": "artifact://mem/1",
		"verified":     true,
	}, memory.MemoryScopeProject))

	record, ok, err := generic.Recall(ctx, "mem-1", memory.MemoryScopeProject)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "declarative", record.Value["memory_class"])
	require.Equal(t, "artifact://mem/1", record.Value["artifact_ref"])

	results, err := generic.Search(ctx, "structured persistence", memory.MemoryScopeProject)
	require.NoError(t, err)
	require.NotEmpty(t, results)

	summary, err := generic.Summarize(ctx, memory.MemoryScopeProject)
	require.NoError(t, err)
	require.Contains(t, summary, "mem-1")

	require.NoError(t, generic.Forget(ctx, "mem-1", memory.MemoryScopeProject))
	_, ok, err = generic.Recall(ctx, "mem-1", memory.MemoryScopeProject)
	require.NoError(t, err)
	require.False(t, ok)
}
