package persistence

import (
	"context"
	"fmt"
	"strings"
	"time"

	relurpctx "codeburg.org/lexbit/relurpify/context"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
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
func SaveCheckpointArtifact(ctx context.Context, env *contextdata.Envelope, upsertArtifact func(contextports.WorkflowArtifactRecord) error, snapshot CheckpointSnapshot) (*relurpctx.ArtifactReference, error) {
	if env == nil || upsertArtifact == nil {
		return nil, nil
	}
	if strings.TrimSpace(snapshot.WorkflowID) == "" || strings.TrimSpace(snapshot.RunID) == "" {
		return nil, nil
	}
	if snapshot.Metadata == nil {
		snapshot.Metadata = map[string]any{}
	}
	if snapshot.InlineRaw != "" {
		snapshot.Metadata["inline_raw"] = snapshot.InlineRaw
	}
	artifact := contextports.WorkflowArtifactRecord{
		ArtifactID:  snapshot.CheckpointID,
		WorkflowID:  snapshot.WorkflowID,
		RunID:       snapshot.RunID,
		ContentType: "application/json",
		StorageKind: "inline",
		Summary:     snapshot.Summary,
		Metadata:    snapshot.Metadata,
		CreatedAt:   time.Now().UTC(),
	}
	if err := upsertArtifact(artifact); err != nil {
		return nil, fmt.Errorf("checkpoint: save artifact: %w", err)
	}
	ref := relurpctx.ArtifactReference{
		ArtifactID: artifact.ArtifactID,
		WorkflowID: artifact.WorkflowID,
		RunID:      artifact.RunID,
	}
	return &ref, nil
}

// LoadLatestCheckpointArtifact returns the most recent checkpoint artifact for a run.
func LoadLatestCheckpointArtifact(ctx context.Context, listArtifactsByRun func(runID string) ([]contextports.WorkflowArtifactRecord, error), runID string) (*contextports.WorkflowArtifactRecord, error) {
	if listArtifactsByRun == nil || strings.TrimSpace(runID) == "" {
		return nil, nil
	}
	artifacts, err := listArtifactsByRun(runID)
	if err != nil {
		return nil, err
	}
	var latest *contextports.WorkflowArtifactRecord
	var latestAt time.Time
	for i := range artifacts {
		if latest == nil || artifacts[i].CreatedAt.After(latestAt) {
			latest = &artifacts[i]
			latestAt = artifacts[i].CreatedAt
		}
	}
	return latest, nil
}
