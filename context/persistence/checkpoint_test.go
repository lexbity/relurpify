package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
)

func TestSaveAndLoadCheckpointArtifact(t *testing.T) {
	var artifacts []contextports.WorkflowArtifactRecord
	env := contextdata.NewEnvelope("task-1", "session-1")
	ref, err := SaveCheckpointArtifact(context.Background(), env, func(artifact contextports.WorkflowArtifactRecord) error {
		artifacts = append(artifacts, artifact)
		return nil
	}, CheckpointSnapshot{
		CheckpointID: "checkpoint-1",
		WorkflowID:   "workflow-1",
		RunID:        "run-1",
		Summary:      "checkpoint summary",
		Metadata:     map[string]any{"kind": "checkpoint"},
		InlineRaw:    `{"ok":true}`,
	})
	require.NoError(t, err)
	require.NotNil(t, ref)
	require.Equal(t, "checkpoint-1", ref.ArtifactID)

	loaded, err := LoadLatestCheckpointArtifact(context.Background(), func(runID string) ([]contextports.WorkflowArtifactRecord, error) {
		return append([]contextports.WorkflowArtifactRecord(nil), artifacts...), nil
	}, "run-1")
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Equal(t, "checkpoint-1", loaded.ArtifactID)
	require.Equal(t, "checkpoint summary", loaded.Summary)
}

func TestLoadLatestCheckpointArtifactReturnsNewestMatch(t *testing.T) {
	artifacts := []contextports.WorkflowArtifactRecord{
		{
			ArtifactID: "checkpoint-old",
			RunID:      "run-1",
			CreatedAt:  time.Date(2024, time.January, 1, 10, 0, 0, 0, time.UTC),
			Summary:    "old",
		},
		{
			ArtifactID: "checkpoint-new",
			RunID:      "run-1",
			CreatedAt:  time.Date(2024, time.January, 1, 11, 0, 0, 0, time.UTC),
			Summary:    "new",
		},
		{
			ArtifactID: "other-kind",
			RunID:      "run-1",
			CreatedAt:  time.Date(2024, time.January, 1, 12, 0, 0, 0, time.UTC),
			Summary:    "ignored",
		},
	}

	loaded, err := LoadLatestCheckpointArtifact(context.Background(), func(runID string) ([]contextports.WorkflowArtifactRecord, error) {
		return append([]contextports.WorkflowArtifactRecord(nil), artifacts...), nil
	}, "run-1")
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Equal(t, "other-kind", loaded.ArtifactID)
}
