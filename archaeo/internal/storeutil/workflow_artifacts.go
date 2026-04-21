package storeutil

import (
	"context"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/memory"
)

type workflowArtifactByIDGetter interface {
	WorkflowArtifactByID(ctx context.Context, artifactID string) (*memory.WorkflowArtifactRecord, bool, error)
}

type workflowArtifactByKindLister interface {
	ListWorkflowArtifactsByKind(ctx context.Context, workflowID, runID, kind string) ([]memory.WorkflowArtifactRecord, error)
}

type workflowArtifactByKindAndWorkspaceLister interface {
	ListWorkflowArtifactsByKindAndWorkspace(ctx context.Context, workflowID, runID, kind, workspaceID string) ([]memory.WorkflowArtifactRecord, error)
}

type latestWorkflowArtifactByKindGetter interface {
	LatestWorkflowArtifactByKind(ctx context.Context, workflowID, runID, kind string) (*memory.WorkflowArtifactRecord, bool, error)
}

type latestWorkflowArtifactByKindAndWorkspaceGetter interface {
	LatestWorkflowArtifactByKindAndWorkspace(ctx context.Context, workflowID, runID, kind, workspaceID string) (*memory.WorkflowArtifactRecord, bool, error)
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

func ListWorkflowArtifactsByKindAndWorkspace(ctx context.Context, store memory.WorkflowStateStore, workflowID, runID, kind, workspaceID string) ([]memory.WorkflowArtifactRecord, error) {
	if store == nil || strings.TrimSpace(kind) == "" || strings.TrimSpace(workspaceID) == "" {
		return nil, nil
	}
	if typed, ok := store.(workflowArtifactByKindAndWorkspaceLister); ok {
		return typed.ListWorkflowArtifactsByKindAndWorkspace(ctx, workflowID, runID, kind, workspaceID)
	}
	if strings.TrimSpace(workflowID) != "" {
		artifacts, err := ListWorkflowArtifactsByKind(ctx, store, workflowID, runID, kind)
		if err != nil {
			return nil, err
		}
		return filterWorkflowArtifactsByWorkspace(artifacts, workspaceID), nil
	}
	workflows, err := store.ListWorkflows(ctx, 4096)
	if err != nil {
		return nil, err
	}
	out := make([]memory.WorkflowArtifactRecord, 0)
	for _, workflow := range workflows {
		artifacts, err := ListWorkflowArtifactsByKind(ctx, store, workflow.WorkflowID, runID, kind)
		if err != nil {
			return nil, err
		}
		out = append(out, filterWorkflowArtifactsByWorkspace(artifacts, workspaceID)...)
	}
	return out, nil
}

func LatestWorkflowArtifactByKind(ctx context.Context, store memory.WorkflowStateStore, workflowID, runID, kind string) (*memory.WorkflowArtifactRecord, bool, error) {
	if store == nil || strings.TrimSpace(workflowID) == "" || strings.TrimSpace(kind) == "" {
		return nil, false, nil
	}
	if typed, ok := store.(latestWorkflowArtifactByKindGetter); ok {
		return typed.LatestWorkflowArtifactByKind(ctx, workflowID, runID, kind)
	}
	artifacts, err := ListWorkflowArtifactsByKind(ctx, store, workflowID, runID, kind)
	if err != nil || len(artifacts) == 0 {
		return nil, false, err
	}
	record := artifacts[len(artifacts)-1]
	return &record, true, nil
}

func LatestWorkflowArtifactByKindAndWorkspace(ctx context.Context, store memory.WorkflowStateStore, workflowID, runID, kind, workspaceID string) (*memory.WorkflowArtifactRecord, bool, error) {
	if store == nil || strings.TrimSpace(kind) == "" || strings.TrimSpace(workspaceID) == "" {
		return nil, false, nil
	}
	if typed, ok := store.(latestWorkflowArtifactByKindAndWorkspaceGetter); ok {
		return typed.LatestWorkflowArtifactByKindAndWorkspace(ctx, workflowID, runID, kind, workspaceID)
	}
	artifacts, err := ListWorkflowArtifactsByKindAndWorkspace(ctx, store, workflowID, runID, kind, workspaceID)
	if err != nil || len(artifacts) == 0 {
		return nil, false, err
	}
	record := artifacts[len(artifacts)-1]
	return &record, true, nil
}

func filterWorkflowArtifactsByWorkspace(artifacts []memory.WorkflowArtifactRecord, workspaceID string) []memory.WorkflowArtifactRecord {
	if len(artifacts) == 0 || strings.TrimSpace(workspaceID) == "" {
		return nil
	}
	out := make([]memory.WorkflowArtifactRecord, 0, len(artifacts))
	for _, artifact := range artifacts {
		if artifactWorkspaceID(artifact) == strings.TrimSpace(workspaceID) {
			out = append(out, artifact)
		}
	}
	return out
}

func artifactWorkspaceID(artifact memory.WorkflowArtifactRecord) string {
	if artifact.SummaryMetadata == nil {
		return ""
	}
	if value, ok := artifact.SummaryMetadata["workspace_id"].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}
