package persistence

import (
	"context"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentlifecycle"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// CheckpointSnapshot describes the minimal state needed to persist and restore a checkpoint artifact.
type CheckpointSnapshot struct {
	CheckpointID string
	WorkflowID   string
	RunID        string
	Kind         string
	Summary      string
	Metadata     map[string]any
	InlineRaw    string
}

// SaveCheckpointArtifact writes a checkpoint artifact and stores its reference back into the envelope.
func SaveCheckpointArtifact(ctx context.Context, env *contextdata.Envelope, repo agentlifecycle.Repository, snapshot CheckpointSnapshot) (*core.ArtifactReference, error) {
	if env == nil || repo == nil {
		return nil, nil
	}
	if strings.TrimSpace(snapshot.WorkflowID) == "" || strings.TrimSpace(snapshot.RunID) == "" {
		return nil, nil
	}
	if strings.TrimSpace(snapshot.Kind) == "" {
		snapshot.Kind = "checkpoint"
	}
	if snapshot.Metadata == nil {
		snapshot.Metadata = map[string]any{}
	}
	artifact := agentlifecycle.WorkflowArtifactRecord{
		ArtifactID:      snapshot.CheckpointID,
		WorkflowID:      snapshot.WorkflowID,
		RunID:           snapshot.RunID,
		Kind:            snapshot.Kind,
		ContentType:     "application/json",
		StorageKind:     agentlifecycle.ArtifactStorageInline,
		SummaryText:     snapshot.Summary,
		SummaryMetadata: snapshot.Metadata,
		InlineRawText:   snapshot.InlineRaw,
		CreatedAt:       time.Now().UTC(),
	}
	if err := repo.UpsertArtifact(ctx, artifact); err != nil {
		return nil, fmt.Errorf("checkpoint: save artifact: %w", err)
	}
	ref := core.ArtifactReference{
		ArtifactID: artifact.ArtifactID,
		WorkflowID: artifact.WorkflowID,
		RunID:      artifact.RunID,
	}
	return &ref, nil
}

// LoadLatestCheckpointArtifact returns the most recent checkpoint artifact for a run.
func LoadLatestCheckpointArtifact(ctx context.Context, repo agentlifecycle.Repository, runID, kind string) (*agentlifecycle.WorkflowArtifactRecord, error) {
	if repo == nil || strings.TrimSpace(runID) == "" {
		return nil, nil
	}
	artifacts, err := repo.ListArtifactsByRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	for i := range artifacts {
		if kind == "" || strings.TrimSpace(artifacts[i].Kind) == strings.TrimSpace(kind) {
			return &artifacts[i], nil
		}
	}
	return nil, nil
}
