package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	"github.com/stretchr/testify/require"
)

type retrievalTestEmbedder struct{}

func (retrievalTestEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, 0, len(texts))
	for _, text := range texts {
		out = append(out, []float32{float32(len(text)), 1})
	}
	return out, nil
}

func (retrievalTestEmbedder) ModelID() string { return "runtime-test-v1" }
func (retrievalTestEmbedder) Dims() int       { return 2 }

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

func TestSQLiteRuntimeMemoryStoreDeclarativeSearchPrefersTitleMatches(t *testing.T) {
	store, err := NewSQLiteRuntimeMemoryStore(filepath.Join(t.TempDir(), "runtime_memory.db"))
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	require.NoError(t, store.PutDeclarative(ctx, memory.DeclarativeMemoryRecord{
		RecordID: "fact-title-1",
		Scope:    memory.MemoryScopeProject,
		Kind:     memory.DeclarativeMemoryKindProjectKnowledge,
		Title:    "Structured persistence",
		Summary:  "Primary decision title match.",
		Content:  "General persistence notes.",
		Verified: true,
	}))
	require.NoError(t, store.PutDeclarative(ctx, memory.DeclarativeMemoryRecord{
		RecordID: "fact-title-2",
		Scope:    memory.MemoryScopeProject,
		Kind:     memory.DeclarativeMemoryKindProjectKnowledge,
		Title:    "Persistence notes",
		Summary:  "Structured persistence appears only in summary.",
		Content:  "General persistence notes.",
	}))

	results, err := store.SearchDeclarative(ctx, memory.DeclarativeMemoryQuery{
		Query: "structured persistence",
		Scope: memory.MemoryScopeProject,
		Limit: 5,
	})
	require.NoError(t, err)
	require.Len(t, results, 2)
	require.Equal(t, "fact-title-1", results[0].RecordID)
}

func TestSQLiteRuntimeMemoryStoreProceduralSearchPrefersNameAndCapabilityMatches(t *testing.T) {
	store, err := NewSQLiteRuntimeMemoryStore(filepath.Join(t.TempDir(), "runtime_memory.db"))
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	require.NoError(t, store.PutProcedural(ctx, memory.ProceduralMemoryRecord{
		RoutineID:   "routine-name-1",
		Scope:       memory.MemoryScopeProject,
		Kind:        memory.ProceduralMemoryKindCapabilityComposition,
		Name:        "checkpoint",
		Description: "Exact name match routine.",
		Summary:     "Checkpoint path.",
		CapabilityDependencies: []core.CapabilitySelector{{
			Kind: core.CapabilityKindTool,
			Name: "checkpoint",
		}},
		Verified:   true,
		ReuseCount: 2,
	}))
	require.NoError(t, store.PutProcedural(ctx, memory.ProceduralMemoryRecord{
		RoutineID:   "routine-name-2",
		Scope:       memory.MemoryScopeProject,
		Kind:        memory.ProceduralMemoryKindCapabilityComposition,
		Name:        "verify-and-save",
		Description: "Routine mentioning checkpoint in body.",
		Summary:     "Checkpoint appears in summary only.",
		InlineBody:  "checkpoint verify save",
		CapabilityDependencies: []core.CapabilitySelector{{
			Kind: core.CapabilityKindTool,
			Name: "checkpoint",
		}},
		ReuseCount: 5,
	}))

	results, err := store.SearchProcedural(ctx, memory.ProceduralMemoryQuery{
		Query:          "checkpoint",
		Scope:          memory.MemoryScopeProject,
		CapabilityName: "checkpoint",
		Limit:          5,
	})
	require.NoError(t, err)
	require.Len(t, results, 2)
	require.Equal(t, "routine-name-1", results[0].RoutineID)
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

func TestSQLiteRuntimeMemoryStoreGenericSearchUsesRelevanceAcrossMemoryClasses(t *testing.T) {
	store, err := NewSQLiteRuntimeMemoryStore(filepath.Join(t.TempDir(), "runtime_memory.db"))
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	require.NoError(t, store.PutDeclarative(ctx, memory.DeclarativeMemoryRecord{
		RecordID:  "fact-ranked-1",
		Scope:     memory.MemoryScopeProject,
		Kind:      memory.DeclarativeMemoryKindDecision,
		Title:     "Structured persistence strategy",
		Summary:   "Structured persistence should be preferred for runtime state.",
		Content:   "Choose structured persistence over ad hoc blobs.",
		Verified:  true,
		CreatedAt: parseTimeValue("2026-03-01T00:00:00Z"),
	}))
	require.NoError(t, store.PutProcedural(ctx, memory.ProceduralMemoryRecord{
		RoutineID:   "routine-ranked-1",
		Scope:       memory.MemoryScopeProject,
		Kind:        memory.ProceduralMemoryKindRoutine,
		Name:        "structured persistence routine",
		Description: "Checkpoint runtime state after each step using structured state snapshots.",
		Summary:     "Generic routine that touches persistence but does not focus on it.",
		InlineBody:  "save structured persistence state verify state",
		CreatedAt:   parseTimeValue("2026-03-02T00:00:00Z"),
	}))

	results, err := store.Search(ctx, "structured persistence", memory.MemoryScopeProject)
	require.NoError(t, err)
	require.Len(t, results, 2)
	require.Equal(t, "fact-ranked-1", results[0].Key)
	require.Equal(t, "declarative", results[0].Value["memory_class"])
}

func TestSQLiteRuntimeMemoryStoreIndexesDeclarativeRecordsForRetrieval(t *testing.T) {
	store, err := NewSQLiteRuntimeMemoryStore(filepath.Join(t.TempDir(), "runtime_memory.db"))
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	require.NoError(t, store.PutDeclarative(ctx, memory.DeclarativeMemoryRecord{
		RecordID: "fact-rt-1",
		Scope:    memory.MemoryScopeProject,
		Kind:     memory.DeclarativeMemoryKindProjectKnowledge,
		Title:    "Retrieval mirror",
		Summary:  "Declarative memory should be searchable through retrieval.",
		Content:  "The runtime store mirrors declarative records into retrieval tables.",
		Tags:     []string{"retrieval", "runtime"},
	}))

	service := store.RetrievalService()
	blocks, event, err := service.Retrieve(ctx, retrieval.RetrievalQuery{
		Text:      "mirrors declarative records",
		Scope:     string(memory.MemoryScopeProject),
		MaxTokens: 200,
		Limit:     3,
	})
	require.NoError(t, err)
	require.NotEmpty(t, blocks)
	require.Equal(t, "l3_main", event.CacheTier)

	var rows int
	err = store.DB().QueryRow(`SELECT COUNT(*) FROM retrieval_documents`).Scan(&rows)
	require.NoError(t, err)
	require.Equal(t, 1, rows)

	require.NoError(t, store.Forget(ctx, "fact-rt-1", memory.MemoryScopeProject))
	err = store.DB().QueryRow(`SELECT COUNT(*) FROM retrieval_chunks WHERE tombstoned = 0`).Scan(&rows)
	require.NoError(t, err)
	require.Equal(t, 0, rows)
}

func TestSQLiteRuntimeMemoryStoreCanConfigureDenseRetrieval(t *testing.T) {
	store, err := NewSQLiteRuntimeMemoryStoreWithRetrieval(
		filepath.Join(t.TempDir(), "runtime_memory.db"),
		SQLiteRuntimeRetrievalOptions{Embedder: retrievalTestEmbedder{}},
	)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	require.NoError(t, store.PutDeclarative(ctx, memory.DeclarativeMemoryRecord{
		RecordID: "fact-dense-1",
		Scope:    memory.MemoryScopeProject,
		Kind:     memory.DeclarativeMemoryKindProjectKnowledge,
		Title:    "Dense retrieval mirror",
		Summary:  "Dense retrieval should work for runtime memory.",
		Content:  "zz",
	}))
	require.NoError(t, store.PutDeclarative(ctx, memory.DeclarativeMemoryRecord{
		RecordID: "fact-dense-2",
		Scope:    memory.MemoryScopeProject,
		Kind:     memory.DeclarativeMemoryKindProjectKnowledge,
		Title:    "Distractor",
		Summary:  "Unrelated runtime memory.",
		Content:  "yyyy",
	}))

	blocks, _, err := store.RetrievalService().Retrieve(ctx, retrieval.RetrievalQuery{
		Text:      "qq",
		Scope:     string(memory.MemoryScopeProject),
		MaxTokens: 100,
		Limit:     3,
	})
	require.NoError(t, err)
	require.NotEmpty(t, blocks)

	block := blocks[0].(core.StructuredContentBlock)
	payload := block.Data.(map[string]any)
	require.Contains(t, payload["text"].(string), "zz")
}

func TestSQLiteRuntimeMemoryStoreUpdatesProceduralCapabilityIndex(t *testing.T) {
	store, err := NewSQLiteRuntimeMemoryStore(filepath.Join(t.TempDir(), "runtime_memory.db"))
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	require.NoError(t, store.PutProcedural(ctx, memory.ProceduralMemoryRecord{
		RoutineID:   "routine-update-1",
		Scope:       memory.MemoryScopeProject,
		Kind:        memory.ProceduralMemoryKindCapabilityComposition,
		Name:        "checkpoint-then-summarize",
		Description: "Initial dependency set",
		Summary:     "Uses checkpoint first",
		InlineBody:  "checkpoint summarize",
		CapabilityDependencies: []core.CapabilitySelector{{
			Kind: core.CapabilityKindTool,
			Name: "checkpoint",
		}},
	}))

	results, err := store.SearchProcedural(ctx, memory.ProceduralMemoryQuery{
		Scope:          memory.MemoryScopeProject,
		CapabilityName: "checkpoint",
		Limit:          5,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)

	require.NoError(t, store.PutProcedural(ctx, memory.ProceduralMemoryRecord{
		RoutineID:   "routine-update-1",
		Scope:       memory.MemoryScopeProject,
		Kind:        memory.ProceduralMemoryKindCapabilityComposition,
		Name:        "summarize-only",
		Description: "Updated dependency set",
		Summary:     "Uses summarize only",
		InlineBody:  "summarize",
		CapabilityDependencies: []core.CapabilitySelector{{
			Kind: core.CapabilityKindTool,
			Name: "summarize",
		}},
		Version: 2,
	}))

	results, err = store.SearchProcedural(ctx, memory.ProceduralMemoryQuery{
		Scope:          memory.MemoryScopeProject,
		CapabilityName: "checkpoint",
		Limit:          5,
	})
	require.NoError(t, err)
	require.Empty(t, results)

	results, err = store.SearchProcedural(ctx, memory.ProceduralMemoryQuery{
		Scope:          memory.MemoryScopeProject,
		CapabilityName: "summarize",
		Limit:          5,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "routine-update-1", results[0].RoutineID)
}

func TestSQLiteRuntimeMemoryStoreBackfillsSearchIndexesOnExistingStore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "runtime_memory.db")
	store, err := NewSQLiteRuntimeMemoryStore(dbPath)
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, store.PutDeclarative(ctx, memory.DeclarativeMemoryRecord{
		RecordID:  "legacy-fact-1",
		Scope:     memory.MemoryScopeProject,
		Kind:      memory.DeclarativeMemoryKindDecision,
		Title:     "Legacy SQLite decision",
		Content:   "SQLite was chosen before search projections existed.",
		Summary:   "Legacy decision",
		TaskID:    "task-legacy",
		ProjectID: "proj-legacy",
		Tags:      []string{"legacy", "sqlite"},
		Metadata:  map[string]any{"source": "legacy"},
		Verified:  true,
		CreatedAt: parseTimeValue("2026-03-01T00:00:00Z"),
		UpdatedAt: parseTimeValue("2026-03-01T00:00:00Z"),
	}))
	require.NoError(t, store.PutProcedural(ctx, memory.ProceduralMemoryRecord{
		RoutineID:   "legacy-routine-1",
		Scope:       memory.MemoryScopeProject,
		Kind:        memory.ProceduralMemoryKindCapabilityComposition,
		Name:        "Legacy checkpoint routine",
		Description: "Legacy routine description",
		Summary:     "Legacy checkpoint routine",
		TaskID:      "task-legacy",
		ProjectID:   "proj-legacy",
		InlineBody:  "checkpoint verify",
		CapabilityDependencies: []core.CapabilitySelector{{
			Kind: core.CapabilityKindTool,
			Name: "checkpoint",
		}},
		Verified:   true,
		Version:    1,
		ReuseCount: 3,
		CreatedAt:  parseTimeValue("2026-03-01T00:00:00Z"),
		UpdatedAt:  parseTimeValue("2026-03-01T00:00:00Z"),
	}))
	require.NoError(t, store.Close())

	db, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)
	_, err = db.Exec(`DROP TABLE IF EXISTS declarative_memory_search`)
	require.NoError(t, err)
	_, err = db.Exec(`DROP TABLE IF EXISTS procedural_memory_search`)
	require.NoError(t, err)
	_, err = db.Exec(`DROP TABLE IF EXISTS procedural_memory_capabilities`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	store, err = NewSQLiteRuntimeMemoryStore(dbPath)
	require.NoError(t, err)
	defer store.Close()
	decl, err := store.SearchDeclarative(ctx, memory.DeclarativeMemoryQuery{
		Query:  "sqlite decision",
		Scope:  memory.MemoryScopeProject,
		Limit:  5,
		TaskID: "task-legacy",
	})
	require.NoError(t, err)
	require.Len(t, decl, 1)
	require.Equal(t, "legacy-fact-1", decl[0].RecordID)

	proc, err := store.SearchProcedural(ctx, memory.ProceduralMemoryQuery{
		Query:          "checkpoint",
		Scope:          memory.MemoryScopeProject,
		CapabilityName: "checkpoint",
		Limit:          5,
	})
	require.NoError(t, err)
	require.Len(t, proc, 1)
	require.Equal(t, "legacy-routine-1", proc[0].RoutineID)
}

func TestSQLiteRuntimeMemoryStoreRebuildsSearchProjectionWhenReadyMarkerMissing(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "runtime_memory.db")
	store, err := NewSQLiteRuntimeMemoryStore(dbPath)
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, store.PutDeclarative(ctx, memory.DeclarativeMemoryRecord{
		RecordID:  "fact-ready-1",
		Scope:     memory.MemoryScopeProject,
		Kind:      memory.DeclarativeMemoryKindProjectKnowledge,
		Title:     "Projection readiness",
		Summary:   "Search projection should recover when readiness metadata is lost.",
		Content:   "projection metadata repair path",
		ProjectID: "proj-ready",
	}))

	_, err = store.DB().Exec(`DELETE FROM declarative_memory_search WHERE record_id = ?`, "fact-ready-1")
	require.NoError(t, err)
	_, err = store.DB().Exec(`DELETE FROM runtime_memory_search_meta WHERE key = ?`, declarativeSearchReadyMetaKey)
	require.NoError(t, err)
	require.NoError(t, store.Close())

	store, err = NewSQLiteRuntimeMemoryStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	results, err := store.SearchDeclarative(ctx, memory.DeclarativeMemoryQuery{
		Query: "metadata repair",
		Scope: memory.MemoryScopeProject,
		Limit: 5,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "fact-ready-1", results[0].RecordID)

	var ready string
	err = store.DB().QueryRow(`SELECT value FROM runtime_memory_search_meta WHERE key = ?`, declarativeSearchReadyMetaKey).Scan(&ready)
	require.NoError(t, err)
	require.Equal(t, runtimeMemorySearchSchemaVersion, ready)
}

func TestSQLiteRuntimeMemoryStoreRebuildsProjectionOnSearchSchemaVersionChange(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "runtime_memory.db")
	store, err := NewSQLiteRuntimeMemoryStore(dbPath)
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, store.PutDeclarative(ctx, memory.DeclarativeMemoryRecord{
		RecordID: "fact-schema-1",
		Scope:    memory.MemoryScopeProject,
		Kind:     memory.DeclarativeMemoryKindProjectKnowledge,
		Title:    "Structured persistence schema",
		Summary:  "Search schema upgrades should rebuild projections.",
		Content:  "legacy projection rebuild path",
	}))
	_, err = store.DB().Exec(`UPDATE runtime_memory_search_meta SET value = '1' WHERE key = ?`, declarativeSearchReadyMetaKey)
	require.NoError(t, err)
	require.NoError(t, store.Close())

	store, err = NewSQLiteRuntimeMemoryStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	results, err := store.SearchDeclarative(ctx, memory.DeclarativeMemoryQuery{
		Query: "structured persistence schema",
		Scope: memory.MemoryScopeProject,
		Limit: 5,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "fact-schema-1", results[0].RecordID)

	var titleNorm string
	err = store.DB().QueryRow(`SELECT title_norm FROM declarative_memory_search WHERE record_id = ?`, "fact-schema-1").Scan(&titleNorm)
	require.NoError(t, err)
	require.Equal(t, "structured persistence schema", titleNorm)
}
