package graph_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/stretchr/testify/require"
)

type stubArtifactSink struct {
	artifacts []graph.ArtifactRecord
}

func (s *stubArtifactSink) SaveArtifact(ctx context.Context, artifact graph.ArtifactRecord) error {
	s.artifacts = append(s.artifacts, artifact)
	return nil
}

type stubCheckpointPersister struct {
	checkpoints []*graph.GraphCheckpoint
}

func (s *stubCheckpointPersister) Save(checkpoint *graph.GraphCheckpoint) error {
	s.checkpoints = append(s.checkpoints, checkpoint)
	return nil
}

type stubRetriever struct {
	results []core.MemoryRecordEnvelope
}

func (s stubRetriever) Retrieve(ctx context.Context, query string, limit int) ([]core.MemoryRecordEnvelope, error) {
	if limit > 0 && len(s.results) > limit {
		return append([]core.MemoryRecordEnvelope{}, s.results[:limit]...), nil
	}
	return append([]core.MemoryRecordEnvelope{}, s.results...), nil
}

type stubPublishedRetriever struct {
	publication *graph.MemoryRetrievalPublication
}

func (s stubPublishedRetriever) Retrieve(ctx context.Context, query string, limit int) ([]core.MemoryRecordEnvelope, error) {
	if s.publication == nil {
		return nil, nil
	}
	return append([]core.MemoryRecordEnvelope{}, s.publication.Results...), nil
}

func (s stubPublishedRetriever) RetrievePublication(ctx context.Context, query string, limit int) (*graph.MemoryRetrievalPublication, error) {
	return s.publication, nil
}

type stubHydrator struct {
	values map[string]any
}

func (s stubHydrator) Hydrate(ctx context.Context, refs []string) (map[string]any, error) {
	return s.values, nil
}

func TestSummarizeContextNodeWritesSummaryAndArtifactReference(t *testing.T) {
	sink := &stubArtifactSink{}
	node := graph.NewSummarizeContextNode("summarize", &core.SimpleSummarizer{})
	node.ArtifactSink = sink
	node.StateKeys = []string{"react.last_tool_result"}

	state := core.NewContext()
	state.Set("task.id", "task-1")
	state.Set("react.last_tool_result", map[string]any{"summary": "ok"})
	state.AddInteraction("user", "hello world", nil)

	result, err := node.Execute(context.Background(), state)
	require.NoError(t, err)
	require.True(t, result.Success)

	raw, ok := state.Get("graph.summary_ref")
	require.True(t, ok)
	ref, ok := raw.(core.ArtifactReference)
	require.True(t, ok)
	require.NotEmpty(t, ref.ArtifactID)
	require.Len(t, sink.artifacts, 1)
}

func TestRetrieveDeclarativeMemoryNodeBoundsStructuredResults(t *testing.T) {
	node := graph.NewRetrieveDeclarativeMemoryNode("retrieve", stubRetriever{
		results: []core.MemoryRecordEnvelope{
			{Key: "a", MemoryClass: core.MemoryClassDeclarative, Scope: "project", Summary: "one", Text: "retrieved one", Source: "retrieval", RecordID: "doc:1", Kind: "document"},
			{Key: "b", MemoryClass: core.MemoryClassDeclarative, Scope: "project", Summary: "two"},
			{Key: "c", MemoryClass: core.MemoryClassDeclarative, Scope: "project", Summary: "three"},
		},
	})
	node.Limit = 2
	state := core.NewContext()
	state.Set("task.instruction", "find memory")

	result, err := node.Execute(context.Background(), state)
	require.NoError(t, err)
	require.True(t, result.Success)

	raw, ok := state.Get("graph.declarative_memory")
	require.True(t, ok)
	payload, ok := raw.(map[string]any)
	require.True(t, ok)
	results, ok := payload["results"].([]core.MemoryRecordEnvelope)
	require.True(t, ok)
	require.Len(t, results, 2)

	rawMixed, ok := state.Get("graph.declarative_memory_payload")
	require.True(t, ok)
	mixedPayload, ok := rawMixed.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "find memory", mixedPayload["query"])
	mixedResults, ok := mixedPayload["results"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, mixedResults, 2)
	require.Equal(t, "retrieval", mixedResults[0]["source"])
	require.Equal(t, "doc:1", mixedResults[0]["record_id"])
	rawRefs, ok := state.Get("graph.declarative_memory_refs")
	require.True(t, ok)
	refs, ok := rawRefs.([]core.ContextReference)
	require.True(t, ok)
	require.Len(t, refs, 2)
	require.Equal(t, core.ContextReferenceRetrievalEvidence, refs[0].Kind)
}

func TestCheckpointNodePersistsCheckpointAndStateReference(t *testing.T) {
	persister := &stubCheckpointPersister{}
	node := graph.NewCheckpointNode("checkpoint", "done", persister)
	node.TaskID = "task-1"

	state := core.NewContext()
	result, err := node.Execute(context.Background(), state)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Len(t, persister.checkpoints, 1)
	require.Equal(t, "done", persister.checkpoints[0].NextNodeID)

	raw, ok := state.Get("graph.checkpoint_ref")
	require.True(t, ok)
	_, ok = raw.(core.ArtifactReference)
	require.True(t, ok)
}

func TestCheckpointNodeAllowsNilPersister(t *testing.T) {
	node := graph.NewCheckpointNode("checkpoint", "done", nil)
	state := core.NewContext()

	result, err := node.Execute(context.Background(), state)
	require.NoError(t, err)
	require.True(t, result.Success)

	raw, ok := state.Get("graph.checkpoint_ref")
	require.True(t, ok)
	_, ok = raw.(core.ArtifactReference)
	require.True(t, ok)
}

func TestSummarizeContextNodeAllowsNilArtifactSink(t *testing.T) {
	node := graph.NewSummarizeContextNode("summarize", &core.SimpleSummarizer{})
	node.ArtifactSink = nil
	state := core.NewContext()
	state.Set("task.id", "task-1")
	state.AddInteraction("user", "hello", nil)

	result, err := node.Execute(context.Background(), state)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.NotEmpty(t, result.Data["summary"])
}

func TestSummarizeContextNodeBoundsLargeStructuredPayload(t *testing.T) {
	node := graph.NewSummarizeContextNode("summarize", &core.SimpleSummarizer{})
	node.StateKeys = []string{"react.last_tool_result"}

	state := core.NewContext()
	state.Set("task.id", "task-1")
	files := make([]any, 0, 100)
	for i := 0; i < 100; i++ {
		files = append(files, filepath.Join("/very/long/path", "segment", "file-name-with-extra-text.txt"))
	}
	state.Set("react.last_tool_result", map[string]any{
		"file_list": map[string]any{
			"data": map[string]any{
				"files": files,
			},
		},
	})

	result, err := node.Execute(context.Background(), state)
	require.NoError(t, err)
	require.True(t, result.Success)
	summary, _ := result.Data["summary"].(string)
	require.NotEmpty(t, summary)

	raw, ok := state.Get("graph.summary")
	require.True(t, ok)
	payload, ok := raw.(map[string]any)
	require.True(t, ok)
	text, ok := payload["summary"].(string)
	require.True(t, ok)
	require.Less(t, len(text), 2048)
}

func TestPersistenceWriterNodePersistsDeclarativeSummaryAndAudit(t *testing.T) {
	store, err := db.NewSQLiteRuntimeMemoryStore(filepath.Join(t.TempDir(), "runtime.db"))
	require.NoError(t, err)
	defer store.Close()

	node := graph.NewPersistenceWriterNode("persist", memory.AdaptRuntimeStoreForGraph(store))
	node.TaskID = "task-1"
	node.Declarative = []graph.DeclarativePersistenceRequest{{
		StateKey:     "react.final_output",
		Scope:        string(memory.MemoryScopeProject),
		Kind:         graph.DeclarativeKindProjectKnowledge,
		Title:        "Ship phase 6",
		SummaryField: "summary",
		ContentField: "result",
		Reason:       "task-completion-summary",
	}}

	state := core.NewContext()
	state.Set("react.final_output", map[string]any{
		"summary": "Phase 6 completed successfully.",
		"result":  map[string]any{"files": 3},
	})

	result, err := node.Execute(context.Background(), state)
	require.NoError(t, err)
	require.True(t, result.Success)

	records, err := store.SearchDeclarative(context.Background(), memory.DeclarativeMemoryQuery{
		Query: "Phase 6 completed successfully.",
		Scope: memory.MemoryScopeProject,
		Limit: 5,
	})
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, "Ship phase 6", records[0].Title)

	raw, ok := state.Get("graph.persistence")
	require.True(t, ok)
	payload, ok := raw.(map[string]any)
	require.True(t, ok)
	audits, ok := payload["records"].([]graph.PersistenceAuditRecord)
	require.True(t, ok)
	require.Len(t, audits, 1)
	require.Equal(t, graph.PersistenceActionCreated, audits[0].Action)
	require.Equal(t, "task-completion-summary", audits[0].Reason)
}

func TestPersistenceWriterNodeSkipsProceduralRoutineWithoutVerification(t *testing.T) {
	store, err := db.NewSQLiteRuntimeMemoryStore(filepath.Join(t.TempDir(), "runtime.db"))
	require.NoError(t, err)
	defer store.Close()

	node := graph.NewPersistenceWriterNode("persist", memory.AdaptRuntimeStoreForGraph(store))
	node.TaskID = "task-2"
	node.Procedural = []graph.ProceduralPersistenceRequest{{
		StateKey:        "graph.candidate_routine",
		Scope:           string(memory.MemoryScopeProject),
		Kind:            graph.ProceduralKindRoutine,
		NameField:       "name",
		SummaryField:    "summary",
		InlineBodyField: "inline_body",
		VerifiedField:   "verified",
		Reason:          "candidate-routine",
	}}

	state := core.NewContext()
	state.Set("graph.candidate_routine", map[string]any{
		"name":        "temporary workaround",
		"summary":     "One-off manual workaround",
		"inline_body": "echo workaround",
		"verified":    false,
	})

	result, err := node.Execute(context.Background(), state)
	require.NoError(t, err)
	require.True(t, result.Success)

	records, err := store.SearchProcedural(context.Background(), memory.ProceduralMemoryQuery{
		Query: "workaround",
		Scope: memory.MemoryScopeProject,
		Limit: 5,
	})
	require.NoError(t, err)
	require.Len(t, records, 0)

	raw, ok := state.Get("graph.persistence")
	require.True(t, ok)
	payload, ok := raw.(map[string]any)
	require.True(t, ok)
	audits, ok := payload["records"].([]graph.PersistenceAuditRecord)
	require.True(t, ok)
	require.Len(t, audits, 1)
	require.Equal(t, graph.PersistenceActionSkipped, audits[0].Action)
}

func TestRetrieveProceduralMemoryNodeBoundsStructuredResults(t *testing.T) {
	node := graph.NewRetrieveProceduralMemoryNode("retrieve", stubRetriever{
		results: []core.MemoryRecordEnvelope{
			{Key: "routine.a", MemoryClass: core.MemoryClassProcedural, Scope: "project", Summary: "one", Text: "routine one", Source: "runtime_memory", RecordID: "routine.a", Kind: "routine"},
			{Key: "routine.b", MemoryClass: core.MemoryClassProcedural, Scope: "project", Summary: "two"},
		},
	})
	node.Limit = 1
	state := core.NewContext()
	state.Set("task.instruction", "find routine")

	result, err := node.Execute(context.Background(), state)
	require.NoError(t, err)
	require.True(t, result.Success)

	raw, ok := state.Get("graph.procedural_memory")
	require.True(t, ok)
	payload, ok := raw.(map[string]any)
	require.True(t, ok)
	results, ok := payload["results"].([]core.MemoryRecordEnvelope)
	require.True(t, ok)
	require.Len(t, results, 1)

	rawMixed, ok := state.Get("graph.procedural_memory_payload")
	require.True(t, ok)
	mixedPayload, ok := rawMixed.(map[string]any)
	require.True(t, ok)
	mixedResults, ok := mixedPayload["results"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, mixedResults, 1)
	require.Equal(t, "runtime_memory", mixedResults[0]["source"])
	rawRefs, ok := state.Get("graph.procedural_memory_refs")
	require.True(t, ok)
	refs, ok := rawRefs.([]core.ContextReference)
	require.True(t, ok)
	require.Len(t, refs, 1)
	require.Equal(t, core.ContextReferenceRuntimeMemory, refs[0].Kind)
}

func TestRetrieveDeclarativeMemoryNodePrefersPublishedRetrieverShape(t *testing.T) {
	node := graph.NewRetrieveDeclarativeMemoryNode("retrieve", stubPublishedRetriever{
		publication: &graph.MemoryRetrievalPublication{
			Query: "find memory",
			Results: []core.MemoryRecordEnvelope{{
				Key:         "fact-1",
				MemoryClass: core.MemoryClassDeclarative,
				Scope:       "project",
				Summary:     "published fact",
			}},
			References: []core.MemoryReference{{
				MemoryClass: core.MemoryClassDeclarative,
				Scope:       "project",
				RecordKey:   "fact-1",
				Summary:     "published fact",
			}},
			Payload: map[string]any{
				"query": "find memory",
				"results": []map[string]any{{
					"summary": "published fact",
					"source":  "published",
				}},
			},
			Refs: []core.ContextReference{{
				Kind: core.ContextReferenceRuntimeMemory,
				ID:   "fact-1",
			}},
		},
	})
	state := core.NewContext()
	state.Set("task.instruction", "find memory")

	result, err := node.Execute(context.Background(), state)
	require.NoError(t, err)
	require.True(t, result.Success)

	rawPayload, ok := state.Get("graph.declarative_memory_payload")
	require.True(t, ok)
	payload, ok := rawPayload.(map[string]any)
	require.True(t, ok)
	mixedResults, ok := payload["results"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, mixedResults, 1)
	require.Equal(t, "published", mixedResults[0]["source"])

	rawRefs, ok := state.Get("graph.declarative_memory_refs")
	require.True(t, ok)
	refs, ok := rawRefs.([]core.ContextReference)
	require.True(t, ok)
	require.Len(t, refs, 1)
	require.Equal(t, "fact-1", refs[0].ID)
}

func TestHydrateContextNodeWritesHydratedValues(t *testing.T) {
	node := graph.NewHydrateContextNode("hydrate", stubHydrator{values: map[string]any{"artifact.body": "ready"}})
	state := core.NewContext()

	result, err := node.Execute(context.Background(), state)
	require.NoError(t, err)
	require.True(t, result.Success)
	raw, ok := state.Get("graph.hydrated")
	require.True(t, ok)
	values, ok := raw.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "ready", values["artifact.body"])
}

func TestPersistenceWriterNodeDeduplicatesDeclarativeRecords(t *testing.T) {
	store, err := db.NewSQLiteRuntimeMemoryStore(filepath.Join(t.TempDir(), "runtime.db"))
	require.NoError(t, err)
	defer store.Close()
	require.NoError(t, store.PutDeclarative(context.Background(), memory.DeclarativeMemoryRecord{
		RecordID:   "decl-1",
		Scope:      memory.MemoryScopeProject,
		Kind:       memory.DeclarativeMemoryKindProjectKnowledge,
		Title:      "Ship phase 6",
		Summary:    "Phase 6 completed successfully.",
		WorkflowID: "workflow-1",
		TaskID:     "task-1",
		CreatedAt:  time.Now().UTC(),
	}))

	node := graph.NewPersistenceWriterNode("persist", memory.AdaptRuntimeStoreForGraph(store))
	node.TaskID = "task-1"
	node.Declarative = []graph.DeclarativePersistenceRequest{{
		StateKey:     "react.final_output",
		Scope:        string(memory.MemoryScopeProject),
		Kind:         graph.DeclarativeKindProjectKnowledge,
		Title:        "Ship phase 6",
		SummaryField: "summary",
		ContentField: "result",
		Reason:       "task-completion-summary",
	}}
	state := core.NewContext()
	state.Set("react.final_output", map[string]any{
		"summary": "Phase 6 completed successfully.",
		"result":  map[string]any{"files": 3},
	})

	result, err := node.Execute(context.Background(), state)
	require.NoError(t, err)
	require.True(t, result.Success)
	raw, ok := state.Get("graph.persistence")
	require.True(t, ok)
	payload, ok := raw.(map[string]any)
	require.True(t, ok)
	audits, ok := payload["records"].([]graph.PersistenceAuditRecord)
	require.True(t, ok)
	require.Len(t, audits, 1)
	require.Equal(t, graph.PersistenceActionDeduplicated, audits[0].Action)
}

func TestPersistenceWriterNodePersistsArtifactAudit(t *testing.T) {
	sink := &stubArtifactSink{}
	node := graph.NewPersistenceWriterNode("persist", nil)
	node.ArtifactSink = sink
	node.Artifacts = []graph.ArtifactPersistenceRequest{{
		ArtifactRefStateKey: "graph.summary_ref",
		Reason:              "summary-artifact",
	}}

	state := core.NewContext()
	state.Set("graph.summary_ref", core.ArtifactReference{
		ArtifactID:   "artifact-1",
		Kind:         "summary",
		ContentType:  "text/plain",
		StorageKind:  "inline",
		URI:          "workflow://artifact/artifact-1",
		Summary:      "summary",
		RawSizeBytes: 42,
	})

	result, err := node.Execute(context.Background(), state)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Len(t, sink.artifacts, 1)
}

func TestPersistenceWriterNodeAllowsNilStore(t *testing.T) {
	node := graph.NewPersistenceWriterNode("persist", nil)
	node.TaskID = "task-3"
	node.Declarative = []graph.DeclarativePersistenceRequest{{
		StateKey:     "react.final_output",
		Scope:        string(memory.MemoryScopeProject),
		Kind:         graph.DeclarativeKindProjectKnowledge,
		Title:        "Summary",
		SummaryField: "summary",
		ContentField: "result",
		Reason:       "task-completion-summary",
	}}
	state := core.NewContext()
	state.Set("react.final_output", map[string]any{
		"summary": "completed",
		"result":  map[string]any{"files": 1},
	})

	result, err := node.Execute(context.Background(), state)
	require.NoError(t, err)
	require.True(t, result.Success)
}
