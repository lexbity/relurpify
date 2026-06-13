package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
)

const (
	Checkpoint1_checkpoint_test = "checkpoint-1"
	Run1_checkpoint_test = "run-1"
)


func TestSaveAndLoadCheckpointArtifact(t *testing.T) {
	var artifacts []contextports.WorkflowArtifactRecord
	env := contextdata.NewEnvelope("task-1", "session-1")
	ref, err := SaveCheckpointArtifact(context.Background(), env, func(artifact contextports.WorkflowArtifactRecord) error {
		artifacts = append(artifacts, artifact)
		return nil
	}, CheckpointSnapshot{
		CheckpointID: Checkpoint1_checkpoint_test,
		WorkflowID:   "workflow-1",
		RunID:        Run1_checkpoint_test,
		Summary:      "checkpoint summary",
		Metadata:     map[string]any{"kind": "checkpoint"},
		InlineRaw:    `{"ok":true}`,
	})
	require.NoError(t, err)
	require.NotNil(t, ref)
	require.Equal(t, Checkpoint1_checkpoint_test, ref.ArtifactID)

	loaded, err := LoadLatestCheckpointArtifact(context.Background(), func(runID string) ([]contextports.WorkflowArtifactRecord, error) {
		return append([]contextports.WorkflowArtifactRecord(nil), artifacts...), nil
	}, Run1_checkpoint_test)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Equal(t, Checkpoint1_checkpoint_test, loaded.ArtifactID)
	require.Equal(t, "checkpoint summary", loaded.Summary)
}

func TestLoadLatestCheckpointArtifactReturnsNewestMatch(t *testing.T) {
	artifacts := []contextports.WorkflowArtifactRecord{
		{
			ArtifactID: "checkpoint-old",
			RunID:      Run1_checkpoint_test,
			CreatedAt:  time.Date(2024, time.January, 1, 10, 0, 0, 0, time.UTC),
			Summary:    "old",
		},
		{
			ArtifactID: "checkpoint-new",
			RunID:      Run1_checkpoint_test,
			CreatedAt:  time.Date(2024, time.January, 1, 11, 0, 0, 0, time.UTC),
			Summary:    "new",
		},
		{
			ArtifactID: "other-kind",
			RunID:      Run1_checkpoint_test,
			CreatedAt:  time.Date(2024, time.January, 1, 12, 0, 0, 0, time.UTC),
			Summary:    "ignored",
		},
	}

	loaded, err := LoadLatestCheckpointArtifact(context.Background(), func(runID string) ([]contextports.WorkflowArtifactRecord, error) {
		return append([]contextports.WorkflowArtifactRecord(nil), artifacts...), nil
	}, Run1_checkpoint_test)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Equal(t, "other-kind", loaded.ArtifactID)
}
