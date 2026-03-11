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
			{Key: "a", MemoryClass: core.MemoryClassDeclarative, Scope: "project", Summary: "one"},
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
			{Key: "routine.a", MemoryClass: core.MemoryClassProcedural, Scope: "project", Summary: "one"},
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
