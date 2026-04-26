package graph_test

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
	"codeburg.org/lexbit/relurpify/framework/memory"
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
