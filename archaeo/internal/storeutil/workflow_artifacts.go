package storeutil

import (
	"context"
	"strings"

	"github.com/lexcodex/relurpify/framework/memory"
)

type workflowArtifactByIDGetter interface {
	WorkflowArtifactByID(ctx context.Context, artifactID string) (*memory.WorkflowArtifactRecord, bool, error)
}

type workflowArtifactByKindLister interface {
	ListWorkflowArtifactsByKind(ctx context.Context, workflowID, runID, kind string) ([]memory.WorkflowArtifactRecord, error)
}

func WorkflowArtifactByID(ctx context.Context, store memory.WorkflowStateStore, artifactID string) (*memory.WorkflowArtifactRecord, bool, error) {
	if store == nil || strings.TrimSpace(artifactID) == "" {
		return nil, false, nil
	}
	if typed, ok := store.(workflowArtifactByIDGetter); ok {
		return typed.WorkflowArtifactByID(ctx, artifactID)
	}
	return nil, false, nil
}

func ListWorkflowArtifactsByKind(ctx context.Context, store memory.WorkflowStateStore, workflowID, runID, kind string) ([]memory.WorkflowArtifactRecord, error) {
	if store == nil || strings.TrimSpace(workflowID) == "" || strings.TrimSpace(kind) == "" {
		return nil, nil
	}
	if typed, ok := store.(workflowArtifactByKindLister); ok {
		return typed.ListWorkflowArtifactsByKind(ctx, workflowID, runID, kind)
	}
	artifacts, err := store.ListWorkflowArtifacts(ctx, workflowID, runID)
	if err != nil {
		return nil, err
	}
	out := make([]memory.WorkflowArtifactRecord, 0, len(artifacts))
	for _, artifact := range artifacts {
		if artifact.Kind == kind {
			out = append(out, artifact)
		}
	}
	return out, nil
}
